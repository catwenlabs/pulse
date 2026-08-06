package aichat

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// fakeStore is an in-memory Store for service-level tests. It models the
// persistence invariants the service relies on: idempotency, the one-active
// generation guard, immutable snapshots, and cursor ordering.
type fakeStore struct {
	mu sync.Mutex

	tools           map[string]SelectionTool
	toolOrder       []string
	toolNames       map[string]string // lower(name) -> id

	conversations        map[string]Conversation
	conversationCreated  map[string]string // idempotency key -> conversation id
	userMessageByKey     map[string]Message
	generationByKey      map[string]string // idempotency key -> assistant message id
	messages             map[string][]Message
	nextID               int
	now                  func() time.Time
	createConversationFn func(context.Context, CreateConversationParams) (Conversation, Message, error)
	errOnStart           error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		tools:           make(map[string]SelectionTool),
		toolNames:       make(map[string]string),
		conversations:   make(map[string]Conversation),
		conversationCreated: make(map[string]string),
		userMessageByKey:    make(map[string]Message),
		generationByKey:     make(map[string]string),
		messages:        make(map[string][]Message),
		now:             func() time.Time { return time.Unix(1700000000, 0).UTC() },
	}
}

func (s *fakeStore) id(prefix string) string {
	s.nextID++
	return prefix + "-" + itoa(s.nextID)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func (s *fakeStore) ListTools(_ context.Context) ([]SelectionTool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tools := make([]SelectionTool, 0, len(s.tools))
	for _, id := range s.toolOrder {
		if tool, ok := s.tools[id]; ok {
			tools = append(tools, tool)
		}
	}
	return tools, nil
}

func (s *fakeStore) GetEnabledTool(_ context.Context, id string) (SelectionTool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tool, ok := s.tools[id]
	if !ok || !tool.Enabled {
		return SelectionTool{}, ErrToolNotFound
	}
	return tool, nil
}

func (s *fakeStore) CreateTool(_ context.Context, input ToolInput) (SelectionTool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lower := strings.ToLower(input.Name)
	if _, exists := s.toolNames[lower]; exists {
		return SelectionTool{}, &DuplicateToolError{Name: input.Name}
	}
	id := s.id("tool")
	now := s.now()
	tool := SelectionTool{
		ID: id, Name: input.Name, PromptTemplate: input.PromptTemplate,
		Enabled: input.Enabled, Position: len(s.toolOrder),
		CreatedAt: now, UpdatedAtAt: now,
	}
	s.tools[id] = tool
	s.toolOrder = append(s.toolOrder, id)
	s.toolNames[lower] = id
	return tool, nil
}

func (s *fakeStore) UpdateTool(_ context.Context, id string, input ToolInput) (SelectionTool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tool, ok := s.tools[id]
	if !ok {
		return SelectionTool{}, ErrToolNotFound
	}
	lower := strings.ToLower(input.Name)
	if existing, exists := s.toolNames[lower]; exists && existing != id {
		return SelectionTool{}, &DuplicateToolError{Name: input.Name}
	}
	delete(s.toolNames, strings.ToLower(tool.Name))
	tool.Name = input.Name
	tool.PromptTemplate = input.PromptTemplate
	tool.Enabled = input.Enabled
	tool.UpdatedAtAt = s.now()
	s.tools[id] = tool
	s.toolNames[lower] = id
	return tool, nil
}

func (s *fakeStore) DeleteTool(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tool, ok := s.tools[id]
	if !ok {
		return ErrToolNotFound
	}
	delete(s.tools, id)
	delete(s.toolNames, strings.ToLower(tool.Name))
	s.toolOrder = removeString(s.toolOrder, id)
	return nil
}

func (s *fakeStore) ReorderTools(_ context.Context, ids []string) ([]SelectionTool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for pos, id := range ids {
		if tool, ok := s.tools[id]; ok {
			tool.Position = pos
			s.tools[id] = tool
		}
	}
	ordered := make([]string, 0, len(s.tools))
	for _, id := range ids {
		if _, ok := s.tools[id]; ok {
			ordered = append(ordered, id)
		}
	}
	for _, id := range s.toolOrder {
		if _, ok := s.tools[id]; ok && !containsString(ordered, id) {
			ordered = append(ordered, id)
		}
	}
	s.toolOrder = ordered
	return s.ListTools(context.Background())
}

func (s *fakeStore) CreateConversation(ctx context.Context, params CreateConversationParams) (Conversation, Message, error) {
	if s.createConversationFn != nil {
		return s.createConversationFn(ctx, params)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if params.IdempotencyKey != "" {
		if id, ok := s.conversationCreated[params.IdempotencyKey]; ok {
			conv := s.conversations[id]
			msgs := s.messages[id]
			return conv, msgs[0], nil
		}
	}
	id := s.id("conv")
	now := s.now()
	conv := Conversation{
		ID: id, SelectedText: params.SelectedText, ToolName: params.ToolName,
		PromptTemplate: params.PromptTemplate, CreatedAt: now, UpdatedAt: now,
	}
	userMsg := Message{
		ID: s.id("msg"), ConversationID: id, Role: RoleUser,
		Content: params.InitialPrompt, CreatedAt: now, UpdatedAt: now,
	}
	s.conversations[id] = conv
	s.messages[id] = []Message{userMsg}
	if params.IdempotencyKey != "" {
		s.conversationCreated[params.IdempotencyKey] = id
	}
	return conv, userMsg, nil
}

func (s *fakeStore) ListConversations(_ context.Context, limit int, cursor string) (ConversationPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]Conversation, 0, len(s.conversations))
	for _, conv := range s.conversations {
		items = append(items, conv)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	start := 0
	if cursor != "" {
		for i, conv := range items {
			if conv.ID == cursor {
				start = i + 1
				break
			}
		}
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := ConversationPage{Items: items[start:end]}
	if end < len(items) {
		page.NextCursor = items[end-1].ID
		page.HasMore = true
	}
	return page, nil
}

func (s *fakeStore) GetConversation(_ context.Context, id string) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conv, ok := s.conversations[id]
	if !ok {
		return Conversation{}, ErrConversationNotFound
	}
	return conv, nil
}

func (s *fakeStore) DeleteConversation(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.conversations[id]; !ok {
		return ErrConversationNotFound
	}
	delete(s.conversations, id)
	delete(s.messages, id)
	return nil
}

func (s *fakeStore) ListMessages(_ context.Context, conversationID string) ([]Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Message, len(s.messages[conversationID]))
	copy(out, s.messages[conversationID])
	return out, nil
}

func (s *fakeStore) AppendUserMessage(_ context.Context, params AppendUserMessageParams) (Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if params.IdempotencyKey != "" {
		if existing, ok := s.userMessageByKey[params.IdempotencyKey]; ok {
			return existing, nil
		}
	}
	if _, ok := s.conversations[params.ConversationID]; !ok {
		return Message{}, ErrConversationNotFound
	}
	now := s.now()
	msg := Message{
		ID: s.id("msg"), ConversationID: params.ConversationID, Role: RoleUser,
		Content: params.Content, CreatedAt: now, UpdatedAt: now,
	}
	s.messages[params.ConversationID] = append(s.messages[params.ConversationID], msg)
	if params.IdempotencyKey != "" {
		s.userMessageByKey[params.IdempotencyKey] = msg
	}
	s.touchConversation(params.ConversationID)
	return msg, nil
}

func (s *fakeStore) StartGeneration(_ context.Context, conversationID, idempotencyKey string) (Message, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errOnStart != nil {
		return Message{}, false, s.errOnStart
	}
	if idempotencyKey != "" {
		if messageID, ok := s.generationByKey[idempotencyKey]; ok {
			if msg, found := s.findMessageByID(conversationID, messageID); found {
				return msg, false, nil
			}
		}
	}
	for _, msg := range s.messages[conversationID] {
		if msg.Role == RoleAssistant && msg.Status == StatusStreaming {
			return Message{}, false, ErrActiveGeneration
		}
	}
	now := s.now()
	msg := Message{
		ID: s.id("msg"), ConversationID: conversationID, Role: RoleAssistant,
		Content: "", Status: StatusStreaming, CreatedAt: now, UpdatedAt: now,
	}
	s.messages[conversationID] = append(s.messages[conversationID], msg)
	if idempotencyKey != "" {
		s.generationByKey[idempotencyKey] = msg.ID
	}
	return msg, true, nil
}

func (s *fakeStore) PeekGeneration(_ context.Context, conversationID, idempotencyKey string) (Message, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if idempotencyKey == "" {
		return Message{}, false, nil
	}
	messageID, ok := s.generationByKey[idempotencyKey]
	if !ok {
		return Message{}, false, nil
	}
	msg, found := s.findMessageByID(conversationID, messageID)
	return msg, found, nil
}

// findMessageByID returns the current state of a message by scanning the
// conversation, so idempotency lookups reflect checkpoints and completion.
func (s *fakeStore) findMessageByID(conversationID, messageID string) (Message, bool) {
	for _, msg := range s.messages[conversationID] {
		if msg.ID == messageID {
			return msg, true
		}
	}
	return Message{}, false
}

func (s *fakeStore) CheckpointGeneration(_ context.Context, messageID, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mutateMessage(messageID, func(m *Message) {
		m.Content = content
		m.Status = StatusStreaming
		m.UpdatedAt = s.now()
	})
	return nil
}

func (s *fakeStore) CompleteGeneration(_ context.Context, messageID string, result GenerationResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mutateMessage(messageID, func(m *Message) {
		m.Content = result.Content
		m.Status = result.Status
		m.Provider = result.Provider
		m.Model = result.Model
		m.PromptTokens = result.PromptTokens
		m.CompletionTokens = result.CompletionTokens
		m.FinishReason = result.FinishReason
		m.Error = result.Error
		m.UpdatedAt = s.now()
	})
	return nil
}

func (s *fakeStore) mutateMessage(messageID string, fn func(*Message)) {
	for convID, msgs := range s.messages {
		for i := range msgs {
			if msgs[i].ID == messageID {
				fn(&s.messages[convID][i])
				s.touchConversation(convID)
				return
			}
		}
	}
}

func (s *fakeStore) touchConversation(id string) {
	if conv, ok := s.conversations[id]; ok {
		conv.UpdatedAt = conv.UpdatedAt.Add(time.Second)
		s.conversations[id] = conv
	}
}

func (s *fakeStore) conversation(id string) Conversation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conversations[id]
}

func (s *fakeStore) messages_(id string) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Message, len(s.messages[id]))
	copy(out, s.messages[id])
	return out
}

func removeString(items []string, value string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item != value {
			out = append(out, item)
		}
	}
	return out
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
