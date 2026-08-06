package aichat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	// defaultMaxConcurrent bounds how many interactive generations may call the
	// Provider at once, reusing the spirit of the background ai_jobs limit.
	defaultMaxConcurrent = 4
	// checkpointInterval is the minimum buffered size growth before another
	// partial-content checkpoint is persisted during streaming.
	checkpointInterval = 1024
)

// ServiceOptions configures the Chat service.
type ServiceOptions struct {
	// MaxConcurrent bounds concurrent interactive Provider calls.
	MaxConcurrent int
	// Memory controls short-term memory assembly.
	Memory MemoryOptions
	// Publish, when set, emits a lightweight realtime invalidation topic after
	// tool, conversation, or terminal message changes. Streaming token payloads
	// are never published; they stay on the request stream.
	Publish func(topic string)
}

// Realtime invalidation topics published through the process-local event hub.
const (
	TopicTools         = "ai-chat-tools"
	TopicConversations = "ai-chat-conversations"
)

// StreamMode selects whether BeginStream generates a fresh reply or retries the
// last failed/cancelled one.
type StreamMode int

const (
	// StreamModeGenerate generates the assistant reply for the latest User
	// Message that has no assistant reply yet.
	StreamModeGenerate StreamMode = iota
	// StreamModeRetry generates a new attempt for the last failed or cancelled
	// Assistant Message without rewriting completed history.
	StreamModeRetry
)

// StreamSession is an open assistant generation. BeginStream creates it;
// DriveStream streams its content. Close releases any held concurrency slot if
// DriveStream has not already done so.
type StreamSession struct {
	conversationID string
	message        Message
	memory         []ProviderMessage
	replay         bool
	release        func()
	handle         *generationHandle
}

// Close releases the concurrency slot if DriveStream did not. release is
// idempotent, so calling Close multiple times (or after DriveStream) is safe.
func (s *StreamSession) Close() {
	if s.release != nil {
		s.release()
	}
}

type generationHandle struct {
	cancel  context.CancelFunc
	stopped atomic.Bool
}

// Service is the concrete Chat implementation. It drives all domain decisions;
// the Store and StreamingProvider seams keep persistence and network internal.
type Service struct {
	store    Store
	provider StreamingProvider
	sem      chan struct{}
	memory   MemoryOptions
	publish  func(string)

	mu     sync.Mutex
	active map[string]*generationHandle
}

// NewService constructs a Chat service. A nil provider leaves tool and history
// management usable but makes generation and conversation creation return
// ErrUnavailable.
func NewService(store Store, provider StreamingProvider, options ServiceOptions) *Service {
	maxConcurrent := options.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = defaultMaxConcurrent
	}
	return &Service{
		store:    store,
		provider: provider,
		sem:      make(chan struct{}, maxConcurrent),
		memory:   options.Memory.withDefaults(),
		publish:  options.Publish,
		active:   make(map[string]*generationHandle),
	}
}

func (s *Service) notify(topic string) {
	if s.publish != nil {
		s.publish(topic)
	}
}

// ListTools returns all selection tools in display order.
func (s *Service) ListTools(ctx context.Context) ([]SelectionTool, error) {
	return s.store.ListTools(ctx)
}

// CreateTool validates and persists a new selection tool.
func (s *Service) CreateTool(ctx context.Context, input ToolInput) (SelectionTool, error) {
	if err := ValidateToolInput(input); err != nil {
		return SelectionTool{}, err
	}
	tool, err := s.store.CreateTool(ctx, normalizeToolInput(input))
	if err != nil {
		return SelectionTool{}, err
	}
	s.notify(TopicTools)
	return tool, nil
}

// UpdateTool validates and persists edits to an existing tool. Editing a tool
// does not alter existing Conversation snapshots.
func (s *Service) UpdateTool(ctx context.Context, id string, input ToolInput) (SelectionTool, error) {
	if strings.TrimSpace(id) == "" {
		return SelectionTool{}, &ValidationError{Field: "id", Message: "tool id is required"}
	}
	if err := ValidateToolInput(input); err != nil {
		return SelectionTool{}, err
	}
	tool, err := s.store.UpdateTool(ctx, id, normalizeToolInput(input))
	if err != nil {
		return SelectionTool{}, err
	}
	s.notify(TopicTools)
	return tool, nil
}

// DeleteTool removes a tool. Conversation history is unaffected.
func (s *Service) DeleteTool(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return &ValidationError{Field: "id", Message: "tool id is required"}
	}
	if err := s.store.DeleteTool(ctx, id); err != nil {
		return err
	}
	s.notify(TopicTools)
	return nil
}

// ReorderTools atomically rewrites the display order of the given tools.
func (s *Service) ReorderTools(ctx context.Context, ids []string) ([]SelectionTool, error) {
	if len(ids) == 0 {
		return s.store.ListTools(ctx)
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return nil, &ValidationError{Field: "tool_ids", Message: "tool ids must not be empty"}
		}
		if _, ok := seen[id]; ok {
			return nil, &ValidationError{Field: "tool_ids", Message: "tool ids must be unique"}
		}
		seen[id] = struct{}{}
	}
	if _, err := s.store.ReorderTools(ctx, ids); err != nil {
		return nil, err
	}
	s.notify(TopicTools)
	return s.store.ListTools(ctx)
}

// CreateConversation resolves the enabled tool server-side, validates the
// selection, expands the template, and atomically persists the Conversation
// and its first User Message. No Source, Entry, Story, page-origin, or tool
// reference is stored.
func (s *Service) CreateConversation(
	ctx context.Context,
	input CreateConversationInput,
	idempotencyKey string,
) (Conversation, Message, error) {
	if s.provider == nil {
		return Conversation{}, Message{}, ErrUnavailable
	}
	if strings.TrimSpace(input.ToolID) == "" {
		return Conversation{}, Message{}, &ValidationError{Field: "tool_id", Message: "tool_id is required"}
	}
	selection, err := NormalizeSelection(input.Selection)
	if err != nil {
		return Conversation{}, Message{}, err
	}
	tool, err := s.store.GetEnabledTool(ctx, input.ToolID)
	if err != nil {
		return Conversation{}, Message{}, err
	}
	initialPrompt, err := ExpandTemplate(tool.PromptTemplate, selection)
	if err != nil {
		return Conversation{}, Message{}, err
	}
	conversation, userMessage, err := s.store.CreateConversation(ctx, CreateConversationParams{
		IdempotencyKey: boundedIdempotencyKey(idempotencyKey),
		SelectedText:   selection,
		ToolName:       tool.Name,
		PromptTemplate: tool.PromptTemplate,
		InitialPrompt:  initialPrompt,
	})
	if err != nil {
		return Conversation{}, Message{}, err
	}
	s.notify(TopicConversations)
	return conversation, userMessage, nil
}

// ListConversations returns a cursor-paginated history page ordered by recent
// activity.
func (s *Service) ListConversations(ctx context.Context, limit int, cursor string) (ConversationPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.store.ListConversations(ctx, limit, cursor)
}

// GetConversation returns a conversation and its full message history.
func (s *Service) GetConversation(ctx context.Context, id string) (Conversation, []Message, error) {
	conversation, err := s.store.GetConversation(ctx, id)
	if err != nil {
		return Conversation{}, nil, err
	}
	messages, err := s.store.ListMessages(ctx, id)
	if err != nil {
		return Conversation{}, nil, err
	}
	return conversation, messages, nil
}

// DeleteConversation deletes a conversation and cascades to all of its messages.
func (s *Service) DeleteConversation(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return &ValidationError{Field: "id", Message: "conversation id is required"}
	}
	if err := s.store.DeleteConversation(ctx, id); err != nil {
		return err
	}
	s.notify(TopicConversations)
	return nil
}

// SendFollowUp appends a follow-up User Message. It does not invoke the
// Provider; the client opens BeginStream to generate the reply.
func (s *Service) SendFollowUp(
	ctx context.Context,
	conversationID string,
	input FollowUpInput,
	idempotencyKey string,
) (Message, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return Message{}, &ValidationError{Field: "content", Message: "message content is required"}
	}
	if len([]rune(content)) > MaxSelectionCharacters {
		return Message{}, &SelectionSizeError{Count: len([]rune(content)), Limit: MaxSelectionCharacters}
	}
	if _, err := s.store.GetConversation(ctx, conversationID); err != nil {
		return Message{}, err
	}
	// Keep messages ordered: a follow-up cannot be appended while an Assistant
	// generation is still streaming, otherwise it would dangle unanswered when
	// the subsequent generate call hits the one-active-generation guard.
	messages, err := s.store.ListMessages(ctx, conversationID)
	if err != nil {
		return Message{}, err
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleAssistant {
			if messages[i].Status == StatusStreaming {
				return Message{}, ErrActiveGeneration
			}
			break
		}
	}
	message, err := s.store.AppendUserMessage(ctx, AppendUserMessageParams{
		ConversationID: conversationID,
		IdempotencyKey: boundedIdempotencyKey(idempotencyKey),
		Content:        content,
	})
	if err != nil {
		return Message{}, err
	}
	s.notify(TopicConversations)
	return message, nil
}

// BeginStream opens (or replays) an assistant generation for the conversation.
// It enforces provider availability, the mode precondition, the one-active
// generation guard, and the global concurrency bound. The returned session must
// be driven by DriveStream (or closed if the caller aborts early).
func (s *Service) BeginStream(
	ctx context.Context,
	conversationID string,
	idempotencyKey string,
	mode StreamMode,
) (*StreamSession, error) {
	if s.provider == nil {
		return nil, ErrUnavailable
	}
	if _, err := s.store.GetConversation(ctx, conversationID); err != nil {
		return nil, err
	}
	key := boundedIdempotencyKey(idempotencyKey)
	// A transport retry with the same key replays the existing generation
	// without re-validating mode or re-calling the Provider.
	if key != "" {
		if existing, ok, err := s.store.PeekGeneration(ctx, conversationID, key); err != nil {
			return nil, err
		} else if ok {
			if existing.Status == StatusStreaming {
				return nil, ErrActiveGeneration
			}
			return &StreamSession{conversationID: conversationID, message: existing, replay: true}, nil
		}
	}

	messages, err := s.store.ListMessages(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	currentIndex, err := resolveCurrentMessage(messages, mode)
	if err != nil {
		return nil, err
	}
	memory, err := AssembleMemory(messages, currentIndex, s.memory)
	if err != nil {
		return nil, err
	}

	if err := s.acquire(ctx); err != nil {
		return nil, err
	}
	released := false
	release := func() {
		if !released {
			released = true
			<-s.sem
		}
	}

	message, created, err := s.store.StartGeneration(ctx, conversationID, key)
	if err != nil {
		release()
		return nil, err
	}
	session := &StreamSession{
		conversationID: conversationID,
		message:        message,
		memory:         memory,
		release:        release,
	}
	if !created {
		// Lost a race against another request with the same key: replay it.
		release()
		if message.Status.IsTerminal() {
			session.replay = true
			return session, nil
		}
		return nil, ErrActiveGeneration
	}
	return session, nil
}

// DriveStream streams the assistant generation, persisting checkpoints and the
// terminal result. It releases the concurrency slot and unregisters the
// generation when done.
func (s *Service) DriveStream(
	ctx context.Context,
	session *StreamSession,
	sink func(ChatStreamEvent) error,
) error {
	defer session.Close()

	emit := func(event ChatStreamEvent) bool {
		event.ConversationID = session.conversationID
		event.MessageID = session.message.ID
		return sink(event) == nil
	}
	if !emit(ChatStreamEvent{Kind: StreamEventMetadata, Status: StatusStreaming}) {
		return nil
	}
	if session.replay {
		emit(terminalEvent(session.message))
		return nil
	}

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	handle := &generationHandle{cancel: cancel}
	s.register(session.conversationID, handle)
	defer s.unregister(session.conversationID, handle)

	var builder strings.Builder
	lastCheckpoint := 0
	providerEmit := func(delta string) error {
		builder.WriteString(delta)
		if !emit(ChatStreamEvent{Kind: StreamEventDelta, Delta: delta}) {
			cancel()
			return fmt.Errorf("client disconnected")
		}
		if builder.Len()-lastCheckpoint >= checkpointInterval {
			lastCheckpoint = builder.Len()
			_ = s.store.CheckpointGeneration(ctx, session.message.ID, builder.String())
		}
		return nil
	}

	result, err := s.provider.Stream(streamCtx, StreamRequest{Messages: session.memory}, providerEmit)
	content := builder.String()
	if err == nil {
		terminal := GenerationResult{
			Status:           StatusCompleted,
			Content:          content,
			Provider:         s.provider.Metadata().Name,
			Model:            result.Model,
			PromptTokens:     result.PromptTokens,
			CompletionTokens: result.CompletionTokens,
			FinishReason:     result.FinishReason,
		}
		if persistErr := s.store.CompleteGeneration(ctx, session.message.ID, terminal); persistErr != nil {
			err = persistErr
		} else {
			emit(terminalEventFromResult(session.message.ID, terminal))
			s.notify(TopicConversations)
			return nil
		}
	}
	// Failure path: preserve partial content, distinguish cancelled vs failed.
	status := StatusFailed
	reason := sanitizeError(err)
	if handle.stopped.Load() {
		status = StatusCancelled
		reason = ""
	}
	terminal := GenerationResult{
		Status:  status,
		Content: content,
		Error:   reason,
	}
	_ = s.store.CompleteGeneration(ctx, session.message.ID, terminal)
	emit(terminalEventFromResult(session.message.ID, terminal))
	s.notify(TopicConversations)
	return nil
}

// Stop cancels the active generation for a conversation, if any. It is
// idempotent: stopping a conversation with no active generation is a no-op.
func (s *Service) Stop(ctx context.Context, conversationID string) error {
	if strings.TrimSpace(conversationID) == "" {
		return &ValidationError{Field: "id", Message: "conversation id is required"}
	}
	s.mu.Lock()
	handle, ok := s.active[conversationID]
	s.mu.Unlock()
	if !ok || handle == nil {
		return nil
	}
	handle.stopped.Store(true)
	handle.cancel()
	return nil
}

func (s *Service) acquire(ctx context.Context) error {
	select {
	case s.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) register(conversationID string, handle *generationHandle) {
	s.mu.Lock()
	s.active[conversationID] = handle
	s.mu.Unlock()
}

func (s *Service) unregister(conversationID string, handle *generationHandle) {
	s.mu.Lock()
	if current, ok := s.active[conversationID]; ok && current == handle {
		delete(s.active, conversationID)
	}
	s.mu.Unlock()
}

// resolveCurrentMessage validates the mode precondition and returns the index
// of the User Message being answered. Active-generation replays must be diverted
// by the caller before this runs, so a streaming Assistant Message here means a
// different request holds the active generation.
func resolveCurrentMessage(messages []Message, mode StreamMode) (int, error) {
	if len(messages) == 0 {
		return 0, ErrNoPendingMessage
	}
	switch mode {
	case StreamModeGenerate:
		lastUser := -1
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == RoleUser {
				lastUser = i
				break
			}
		}
		if lastUser < 0 {
			return 0, ErrNoPendingMessage
		}
		if lastUser == len(messages)-1 {
			return lastUser, nil
		}
		next := messages[lastUser+1]
		if next.Status == StatusStreaming {
			return 0, ErrActiveGeneration
		}
		return 0, ErrNoPendingMessage
	case StreamModeRetry:
		last := messages[len(messages)-1]
		if last.Role != RoleAssistant || !last.Status.IsTerminal() || last.Status == StatusCompleted {
			return 0, ErrNotRetryable
		}
		// Find the user message preceding the failed/cancelled assistant reply.
		for i := len(messages) - 2; i >= 0; i-- {
			if messages[i].Role == RoleUser {
				return i, nil
			}
		}
		return 0, ErrNotRetryable
	}
	return 0, ErrNoPendingMessage
}

func normalizeToolInput(input ToolInput) ToolInput {
	return ToolInput{
		Name:           NormalizeToolName(input.Name),
		PromptTemplate: input.PromptTemplate,
		Enabled:        input.Enabled,
	}
}

// boundedIdempotencyKey trims and bounds an idempotency key so it can be stored
// safely. An empty key is preserved (the Store treats it as "no idempotency").
func boundedIdempotencyKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) > 128 {
		return key[:128]
	}
	return key
}

// sanitizeError strips credentials, headers, and auth material from a Provider
// error before it is persisted or returned.
func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "generation interrupted"
	}
	return truncateMessage(err.Error(), 500)
}

func truncateMessage(value string, limit int) string {
	if len([]rune(value)) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func terminalEvent(message Message) ChatStreamEvent {
	return ChatStreamEvent{
		Kind:             terminalKind(message.Status),
		Content:          message.Content,
		Status:           message.Status,
		Provider:         message.Provider,
		Model:            message.Model,
		PromptTokens:     message.PromptTokens,
		CompletionTokens: message.CompletionTokens,
		FinishReason:     message.FinishReason,
		Error:            message.Error,
	}
}

func terminalEventFromResult(messageID string, result GenerationResult) ChatStreamEvent {
	return ChatStreamEvent{
		Kind:             terminalKind(result.Status),
		Content:          result.Content,
		Status:           result.Status,
		Provider:         result.Provider,
		Model:            result.Model,
		PromptTokens:     result.PromptTokens,
		CompletionTokens: result.CompletionTokens,
		FinishReason:     result.FinishReason,
		Error:            result.Error,
	}
}

func terminalKind(status GenerationStatus) StreamEventKind {
	switch status {
	case StatusCompleted:
		return StreamEventCompleted
	case StatusCancelled:
		return StreamEventCancelled
	default:
		return StreamEventFailed
	}
}
