package aichat

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeProvider is a controllable streaming provider. It can emit deterministic
// deltas, block until released, return token metadata, fail before output,
// fail after partial output, and observe the final ordered message request —
// all without making network calls.
type fakeProvider struct {
	mu             sync.Mutex
	metadata       ProviderMetadata
	deltas         []string
	result         StreamResult
	err            error
	failBeforeOut  bool
	block          chan struct{}
	blocked        chan struct{} // closed when the provider reaches the blocking point
	lastRequest    StreamRequest
	observedDeltas []string
}

func (p *fakeProvider) Metadata() ProviderMetadata { return p.metadata }

func (p *fakeProvider) Stream(ctx context.Context, request StreamRequest, emit func(string) error) (StreamResult, error) {
	p.mu.Lock()
	p.lastRequest = request
	p.mu.Unlock()

	if p.err != nil && p.failBeforeOut {
		return StreamResult{}, p.err
	}
	for _, delta := range p.deltas {
		if err := emit(delta); err != nil {
			return StreamResult{}, err
		}
		p.mu.Lock()
		p.observedDeltas = append(p.observedDeltas, delta)
		p.mu.Unlock()
	}
	if p.block != nil {
		if p.blocked != nil {
			close(p.blocked)
		}
		select {
		case <-p.block:
		case <-ctx.Done():
			return StreamResult{}, ctx.Err()
		}
	}
	if ctx.Err() != nil {
		return StreamResult{}, ctx.Err()
	}
	if p.err != nil {
		return StreamResult{}, p.err
	}
	return p.result, nil
}

func (p *fakeProvider) lastMessages() []ProviderMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastRequest.Messages
}

func seedTool(t *testing.T, store *fakeStore, name, template string, enabled bool) SelectionTool {
	t.Helper()
	tool, err := store.CreateTool(context.Background(), ToolInput{Name: name, PromptTemplate: template, Enabled: enabled})
	if err != nil {
		t.Fatalf("seed tool: %v", err)
	}
	return tool
}

func newService(t *testing.T, provider *fakeProvider) (*Service, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	if provider == nil {
		provider = &fakeProvider{metadata: ProviderMetadata{Name: "fake", Model: "fake-model"}}
	}
	service := NewService(store, provider, ServiceOptions{Memory: MemoryOptions{ProviderInputLimit: 0}})
	return service, store
}

func createConversationForSelection(t *testing.T, service *Service, toolID, selection string) (Conversation, Message) {
	t.Helper()
	conv, userMsg, err := service.CreateConversation(context.Background(), CreateConversationInput{ToolID: toolID, Selection: selection}, "create-key")
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	return conv, userMsg
}

func collectStream(t *testing.T, service *Service, ctx context.Context, convID, key string, mode StreamMode) []ChatStreamEvent {
	t.Helper()
	session, err := service.BeginStream(ctx, convID, key, mode)
	if err != nil {
		t.Fatalf("BeginStream() error = %v", err)
	}
	var events []ChatStreamEvent
	_ = service.DriveStream(ctx, session, func(event ChatStreamEvent) error {
		events = append(events, event)
		return nil
	})
	return events
}

func TestCreateConversationResolvesToolAndPersistsImmutableSnapshots(t *testing.T) {
	service, store := newService(t, nil)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)

	conv, userMsg, err := service.CreateConversation(context.Background(), CreateConversationInput{
		ToolID: tool.ID, Selection: "  E=mc^2  ",
	}, "key-1")
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	if conv.ToolName != "AI 解读" || conv.PromptTemplate != "请解释：{{selection}}" {
		t.Errorf("snapshots = %+v", conv)
	}
	if conv.SelectedText != "E=mc^2" {
		t.Errorf("SelectedText = %q, want trimmed", conv.SelectedText)
	}
	if userMsg.Content != "请解释：E=mc^2" {
		t.Errorf("initial prompt not expanded: %q", userMsg.Content)
	}
	// The conversation must not store tool, source, entry, or story references.
	if conv.ID == "" || strings.Contains(conv.ID, tool.ID) {
		t.Errorf("conversation id leaked tool reference: %q", conv.ID)
	}
}

func TestCreateConversationRejectsInvalidInputs(t *testing.T) {
	service, store := newService(t, nil)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	disabled := seedTool(t, store, "disabled", "x{{selection}}", false)

	cases := []struct {
		name    string
		input   CreateConversationInput
		wantErr error
	}{
		{"empty tool id", CreateConversationInput{Selection: "x"}, nil},
		{"disabled tool", CreateConversationInput{ToolID: disabled.ID, Selection: "x"}, ErrToolNotFound},
		{"deleted tool", CreateConversationInput{ToolID: "missing", Selection: "x"}, ErrToolNotFound},
		{"empty selection", CreateConversationInput{ToolID: tool.ID, Selection: "   "}, ErrSelectionRequired},
		{"oversized selection", CreateConversationInput{ToolID: tool.ID, Selection: strings.Repeat("字", MaxSelectionCharacters+1)}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := service.CreateConversation(context.Background(), tc.input, "")
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			switch tc.name {
			case "empty tool id":
				var ve *ValidationError
				if !errors.As(err, &ve) || ve.Field != "tool_id" {
					t.Fatalf("error = %v, want tool_id ValidationError", err)
				}
			case "oversized selection":
				var size *SelectionSizeError
				if !errors.As(err, &size) {
					t.Fatalf("error = %v, want SelectionSizeError", err)
				}
			default:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
			}
		})
	}
}

func TestCreateConversationIsIdempotentByKey(t *testing.T) {
	service, store := newService(t, nil)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)

	conv1, msg1, err := service.CreateConversation(context.Background(), CreateConversationInput{ToolID: tool.ID, Selection: "text"}, "same-key")
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	conv2, msg2, err := service.CreateConversation(context.Background(), CreateConversationInput{ToolID: tool.ID, Selection: "text"}, "same-key")
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if conv1.ID != conv2.ID || msg1.ID != msg2.ID {
		t.Errorf("idempotent retry created duplicates: %+v vs %+v", conv1, conv2)
	}
	if len(store.conversations) != 1 {
		t.Errorf("expected 1 conversation, got %d", len(store.conversations))
	}
}

func TestGenerateStreamsCompletionAndPersistsResult(t *testing.T) {
	provider := &fakeProvider{
		metadata: ProviderMetadata{Name: "fake", Model: "fake-model"},
		deltas:   []string{"Hello ", "world"},
		result:   StreamResult{Model: "fake-model", FinishReason: "stop", PromptTokens: 10, CompletionTokens: 2},
	}
	service, store := newService(t, provider)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	conv, _ := createConversationForSelection(t, service, tool.ID, "E=mc^2")

	events := collectStream(t, service, context.Background(), conv.ID, "gen-1", StreamModeGenerate)

	if len(events) < 3 {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Kind != StreamEventMetadata {
		t.Errorf("first event = %q, want metadata", events[0].Kind)
	}
	var joined strings.Builder
	terminalSeen := false
	for _, event := range events {
		if event.Kind == StreamEventDelta {
			joined.WriteString(event.Delta)
		}
		if event.Kind == StreamEventCompleted {
			terminalSeen = true
			if event.PromptTokens != 10 || event.CompletionTokens != 2 {
				t.Errorf("terminal usage = %+v", event)
			}
		}
	}
	if joined.String() != "Hello world" {
		t.Errorf("streamed content = %q", joined.String())
	}
	if !terminalSeen {
		t.Errorf("no completed terminal event")
	}
	msgs := store.messages_(conv.ID)
	last := msgs[len(msgs)-1]
	if last.Status != StatusCompleted || last.Content != "Hello world" {
		t.Errorf("persisted assistant = %+v", last)
	}
}

func TestGenerateSendsSystemPromptAndInitialSelection(t *testing.T) {
	provider := &fakeProvider{metadata: ProviderMetadata{Name: "fake", Model: "m"}, deltas: []string{"ok"}}
	service, store := newService(t, provider)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	conv, _ := createConversationForSelection(t, service, tool.ID, "公式内容")

	collectStream(t, service, context.Background(), conv.ID, "gen", StreamModeGenerate)

	messages := provider.lastMessages()
	if len(messages) < 2 {
		t.Fatalf("provider messages = %+v", messages)
	}
	if messages[0].Content != SystemPrompt {
		t.Errorf("first provider message is not the System Prompt")
	}
	if !strings.Contains(messages[1].Content, "公式内容") {
		t.Errorf("initial selection missing from provider input: %q", messages[1].Content)
	}
}

func TestExplicitStopProducesCancelledWithPartialPreserved(t *testing.T) {
	provider := &fakeProvider{
		metadata: ProviderMetadata{Name: "fake", Model: "m"},
		deltas:   []string{"partial"},
		block:    make(chan struct{}),
		blocked:  make(chan struct{}),
	}
	service, store := newService(t, provider)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	conv, _ := createConversationForSelection(t, service, tool.ID, "text")

	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session, err := service.BeginStream(streamCtx, conv.ID, "gen", StreamModeGenerate)
	if err != nil {
		t.Fatalf("BeginStream() error = %v", err)
	}
	done := make(chan []ChatStreamEvent, 1)
	go func() {
		var events []ChatStreamEvent
		_ = service.DriveStream(streamCtx, session, func(e ChatStreamEvent) error {
			events = append(events, e)
			return nil
		})
		done <- events
	}()

	<-provider.blocked // ensure the provider is mid-stream before stopping
	if err := service.Stop(context.Background(), conv.ID); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	close(provider.block)

	select {
	case events := <-done:
		terminal := terminalEventOf(events)
		if terminal.Kind != StreamEventCancelled {
			t.Errorf("terminal event = %+v, want cancelled", terminal)
		}
		msgs := store.messages_(conv.ID)
		last := msgs[len(msgs)-1]
		if last.Status != StatusCancelled {
			t.Errorf("persisted status = %q, want cancelled", last.Status)
		}
		if last.Content != "partial" {
			t.Errorf("partial content not preserved: %q", last.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DriveStream did not terminate after Stop")
	}
}

func TestProviderFailureAfterPartialProducesFailedWithPartialPreserved(t *testing.T) {
	provider := &fakeProvider{
		metadata: ProviderMetadata{Name: "fake", Model: "m"},
		deltas:   []string{"partial-"},
		err:      errors.New("upstream returned 503"),
	}
	service, store := newService(t, provider)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	conv, _ := createConversationForSelection(t, service, tool.ID, "text")

	events := collectStream(t, service, context.Background(), conv.ID, "gen", StreamModeGenerate)
	terminal := terminalEventOf(events)
	if terminal.Kind != StreamEventFailed {
		t.Errorf("terminal event = %+v, want failed", terminal)
	}
	if !strings.Contains(terminal.Error, "503") {
		t.Errorf("error message = %q, want sanitized upstream detail", terminal.Error)
	}
	msgs := store.messages_(conv.ID)
	last := msgs[len(msgs)-1]
	if last.Status != StatusFailed || last.Content != "partial-" {
		t.Errorf("persisted failed partial = %+v", last)
	}
}

func TestProviderFailureBeforeOutputProducesFailedEmptyContent(t *testing.T) {
	provider := &fakeProvider{
		metadata:      ProviderMetadata{Name: "fake", Model: "m"},
		err:           errors.New("upstream 500"),
		failBeforeOut: true,
	}
	service, store := newService(t, provider)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	conv, _ := createConversationForSelection(t, service, tool.ID, "text")

	events := collectStream(t, service, context.Background(), conv.ID, "gen", StreamModeGenerate)
	if terminalEventOf(events).Kind != StreamEventFailed {
		t.Errorf("expected failed terminal")
	}
	msgs := store.messages_(conv.ID)
	last := msgs[len(msgs)-1]
	if last.Status != StatusFailed || last.Content != "" {
		t.Errorf("persisted = %+v, want failed empty content", last)
	}
}

func TestClientDisconnectProducesFailedWithPartialPreserved(t *testing.T) {
	provider := &fakeProvider{
		metadata: ProviderMetadata{Name: "fake", Model: "m"},
		deltas:   []string{"partial"},
		block:    make(chan struct{}),
		blocked:  make(chan struct{}),
	}
	service, store := newService(t, provider)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	conv, _ := createConversationForSelection(t, service, tool.ID, "text")

	streamCtx, cancel := context.WithCancel(context.Background())
	session, err := service.BeginStream(streamCtx, conv.ID, "gen", StreamModeGenerate)
	if err != nil {
		t.Fatalf("BeginStream() error = %v", err)
	}
	done := make(chan []ChatStreamEvent, 1)
	go func() {
		var events []ChatStreamEvent
		_ = service.DriveStream(streamCtx, session, func(e ChatStreamEvent) error {
			events = append(events, e)
			return nil
		})
		done <- events
	}()

	<-provider.blocked
	cancel() // genuine disconnect: cancel the request context, not Stop
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("DriveStream did not terminate after disconnect")
	}

	msgs := store.messages_(conv.ID)
	last := msgs[len(msgs)-1]
	if last.Status != StatusFailed {
		t.Errorf("disconnect status = %q, want failed", last.Status)
	}
	if last.Content != "partial" {
		t.Errorf("partial content not preserved on disconnect: %q", last.Content)
	}
}

func TestOneActiveGenerationEnforced(t *testing.T) {
	provider := &fakeProvider{
		metadata: ProviderMetadata{Name: "fake", Model: "m"},
		deltas:   []string{"ok"},
		block:    make(chan struct{}),
		blocked:  make(chan struct{}),
	}
	service, store := newService(t, provider)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	conv, _ := createConversationForSelection(t, service, tool.ID, "text")

	firstSession, err := service.BeginStream(context.Background(), conv.ID, "gen-1", StreamModeGenerate)
	if err != nil {
		t.Fatalf("first BeginStream() error = %v", err)
	}
	defer firstSession.Close()

	if _, err := service.BeginStream(context.Background(), conv.ID, "gen-2", StreamModeGenerate); !errors.Is(err, ErrActiveGeneration) {
		t.Fatalf("second BeginStream() error = %v, want ErrActiveGeneration", err)
	}
	close(provider.block)
}

func TestRepeatedIdempotencyKeyDoesNotDuplicateProviderCall(t *testing.T) {
	provider := &fakeProvider{
		metadata: ProviderMetadata{Name: "fake", Model: "m"},
		deltas:   []string{"ok"},
	}
	service, store := newService(t, provider)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	conv, _ := createConversationForSelection(t, service, tool.ID, "text")

	collectStream(t, service, context.Background(), conv.ID, "same-gen", StreamModeGenerate)

	// Transport retry with the same key must not start a second provider call.
	session, err := service.BeginStream(context.Background(), conv.ID, "same-gen", StreamModeGenerate)
	if err != nil {
		t.Fatalf("retry BeginStream() error = %v", err)
	}
	var replayed []ChatStreamEvent
	_ = service.DriveStream(context.Background(), session, func(e ChatStreamEvent) error {
		replayed = append(replayed, e)
		return nil
	})
	if terminalEventOf(replayed).Kind != StreamEventCompleted {
		t.Errorf("replay terminal = %+v", terminalEventOf(replayed))
	}
	// Only one assistant message should exist.
	count := 0
	for _, m := range store.messages_(conv.ID) {
		if m.Role == RoleAssistant {
			count++
		}
	}
	if count != 1 {
		t.Errorf("assistant messages = %d, want 1", count)
	}
}

func TestRetryFailedResponseAndRejectCompleted(t *testing.T) {
	provider := &fakeProvider{
		metadata: ProviderMetadata{Name: "fake", Model: "m"},
		err:      errors.New("boom"),
	}
	service, store := newService(t, provider)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	conv, _ := createConversationForSelection(t, service, tool.ID, "text")

	// First generation fails.
	collectStream(t, service, context.Background(), conv.ID, "gen-1", StreamModeGenerate)
	last := store.messages_(conv.ID)[len(store.messages_(conv.ID))-1]
	if last.Status != StatusFailed {
		t.Fatalf("setup: expected failed, got %s", last.Status)
	}

	// Retry succeeds with a fresh provider.
	provider2 := &fakeProvider{metadata: ProviderMetadata{Name: "fake", Model: "m"}, deltas: []string{"fixed"}}
	service.provider = provider2
	events := collectStream(t, service, context.Background(), conv.ID, "retry-1", StreamModeRetry)
	if terminalEventOf(events).Kind != StreamEventCompleted {
		t.Errorf("retry terminal = %+v", terminalEventOf(events))
	}
	// Completed history must remain immutable: retry cannot regenerate it.
	if _, err := service.BeginStream(context.Background(), conv.ID, "retry-2", StreamModeRetry); !errors.Is(err, ErrNotRetryable) {
		t.Errorf("retry of completed = %v, want ErrNotRetryable", err)
	}
}

func TestRetryRequiresAFailedOrCancelledReply(t *testing.T) {
	service, store := newService(t, nil)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	conv, _ := createConversationForSelection(t, service, tool.ID, "text")
	if _, err := service.BeginStream(context.Background(), conv.ID, "r", StreamModeRetry); !errors.Is(err, ErrNotRetryable) {
		t.Errorf("retry with no assistant reply = %v, want ErrNotRetryable", err)
	}
}

func TestGlobalConcurrencyBounded(t *testing.T) {
	provider := &fakeProvider{
		metadata: ProviderMetadata{Name: "fake", Model: "m"},
		deltas:   []string{"ok"},
		block:    make(chan struct{}),
	}
	store := newFakeStore()
	service := NewService(store, provider, ServiceOptions{MaxConcurrent: 1})
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	conv1, _ := createConversationForSelection(t, service, tool.ID, "a")
	conv2, _ := createConversationForSelection(t, service, tool.ID, "b")

	firstSession, err := service.BeginStream(context.Background(), conv1.ID, "g1", StreamModeGenerate)
	if err != nil {
		t.Fatalf("first BeginStream() error = %v", err)
	}
	defer firstSession.Close()

	// The second generation must block on the concurrency slot; a bounded
	// context surfaces that as a context error rather than hanging.
	bounded, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := service.BeginStream(bounded, conv2.ID, "g2", StreamModeGenerate); err == nil {
		t.Fatalf("expected concurrency limit error, got nil")
	}
	close(provider.block)
}

func TestStopIsIdempotentWhenNothingActive(t *testing.T) {
	service, store := newService(t, nil)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	conv, _ := createConversationForSelection(t, service, tool.ID, "text")
	if err := service.Stop(context.Background(), conv.ID); err != nil {
		t.Fatalf("Stop() with nothing active = %v, want nil", err)
	}
}

func TestSendFollowUpAppendsUserMessage(t *testing.T) {
	provider := &fakeProvider{metadata: ProviderMetadata{Name: "fake", Model: "m"}, deltas: []string{"a"}}
	service, store := newService(t, provider)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	conv, _ := createConversationForSelection(t, service, tool.ID, "text")

	// Complete the first reply so a follow-up is the new pending message.
	collectStream(t, service, context.Background(), conv.ID, "g1", StreamModeGenerate)

	followUp, err := service.SendFollowUp(context.Background(), conv.ID, FollowUpInput{Content: "再说详细点"}, "fu-1")
	if err != nil {
		t.Fatalf("SendFollowUp() error = %v", err)
	}
	if followUp.Content != "再说详细点" {
		t.Errorf("followUp = %+v", followUp)
	}
	if _, err := service.BeginStream(context.Background(), conv.ID, "g2", StreamModeGenerate); err != nil {
		t.Fatalf("generate for follow-up = %v", err)
	}
}

func TestSendFollowUpRejectedWhileGenerationStreaming(t *testing.T) {
	provider := &fakeProvider{metadata: ProviderMetadata{Name: "fake", Model: "m"}, deltas: []string{"a"}}
	service, store := newService(t, provider)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	conv, _ := createConversationForSelection(t, service, tool.ID, "text")

	// Simulate an in-flight Assistant generation without driving the provider.
	if _, _, err := store.StartGeneration(context.Background(), conv.ID, "inflight"); err != nil {
		t.Fatalf("StartGeneration() error = %v", err)
	}

	_, err := service.SendFollowUp(context.Background(), conv.ID, FollowUpInput{Content: "追问"}, "fu-blocked")
	if !errors.Is(err, ErrActiveGeneration) {
		t.Fatalf("SendFollowUp() error = %v, want ErrActiveGeneration", err)
	}
}

func TestFollowUpMemoryIncludesRecentCompletedTurns(t *testing.T) {
	provider := &fakeProvider{metadata: ProviderMetadata{Name: "fake", Model: "m"}, deltas: []string{"first-answer"}}
	service, store := newService(t, provider)
	tool := seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	conv, _ := createConversationForSelection(t, service, tool.ID, "initial-selection")

	collectStream(t, service, context.Background(), conv.ID, "g1", StreamModeGenerate)
	if _, err := service.SendFollowUp(context.Background(), conv.ID, FollowUpInput{Content: "follow-up-q"}, "fu"); err != nil {
		t.Fatalf("SendFollowUp: %v", err)
	}

	// Replace provider to observe the second request.
	observer := &fakeProvider{metadata: ProviderMetadata{Name: "fake", Model: "m"}, deltas: []string{"second-answer"}}
	service.provider = observer
	collectStream(t, service, context.Background(), conv.ID, "g2", StreamModeGenerate)

	messages := observer.lastMessages()
	joined := joinProviderMessages(messages)
	if !strings.Contains(joined, "initial-selection") {
		t.Errorf("initial selection dropped from follow-up memory: %q", joined)
	}
	if !strings.Contains(joined, "first-answer") {
		t.Errorf("completed prior answer dropped from memory: %q", joined)
	}
	if !strings.Contains(joined, "follow-up-q") {
		t.Errorf("current question missing from memory: %q", joined)
	}
}

func TestServiceUnavailableWithoutProvider(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, nil, ServiceOptions{})
	seedTool(t, store, "AI 解读", "请解释：{{selection}}", true)
	if _, _, err := service.CreateConversation(context.Background(), CreateConversationInput{ToolID: "x", Selection: "y"}, ""); !errors.Is(err, ErrUnavailable) {
		t.Errorf("CreateConversation without provider = %v, want ErrUnavailable", err)
	}
}

func terminalEventOf(events []ChatStreamEvent) ChatStreamEvent {
	for _, event := range events {
		switch event.Kind {
		case StreamEventCompleted, StreamEventCancelled, StreamEventFailed:
			return event
		}
	}
	return ChatStreamEvent{}
}

func joinProviderMessages(messages []ProviderMessage) string {
	var builder strings.Builder
	for _, m := range messages {
		builder.WriteString(m.Content)
		builder.WriteString("|")
	}
	return builder.String()
}

// ensure time import is retained even if future edits remove its only use.
var _ = time.Now
