package aichat

import "context"

// CreateConversationParams holds the immutable snapshots persisted atomically
// with a new Conversation and its first User Message. The service prepares
// these after resolving the enabled tool, normalizing the selection, and
// expanding the template.
type CreateConversationParams struct {
	IdempotencyKey string
	SelectedText   string
	ToolName       string
	PromptTemplate string
	InitialPrompt  string
}

// AppendUserMessageParams appends a follow-up User Message to an existing
// Conversation. It is idempotent by key so a transport retry cannot duplicate
// the message.
type AppendUserMessageParams struct {
	ConversationID  string
	IdempotencyKey  string
	Content         string
}

// Store is the persistence seam for the AI Chat domain. Implementations keep
// persistence details internal; the service drives all domain decisions.
type Store interface {
	// Tools
	ListTools(ctx context.Context) ([]SelectionTool, error)
	GetEnabledTool(ctx context.Context, id string) (SelectionTool, error)
	CreateTool(ctx context.Context, input ToolInput) (SelectionTool, error)
	UpdateTool(ctx context.Context, id string, input ToolInput) (SelectionTool, error)
	DeleteTool(ctx context.Context, id string) error
	ReorderTools(ctx context.Context, ids []string) ([]SelectionTool, error)

	// Conversations
	CreateConversation(ctx context.Context, params CreateConversationParams) (Conversation, Message, error)
	ListConversations(ctx context.Context, limit int, cursor string) (ConversationPage, error)
	GetConversation(ctx context.Context, id string) (Conversation, error)
	DeleteConversation(ctx context.Context, id string) error

	// Messages
	ListMessages(ctx context.Context, conversationID string) ([]Message, error)
	AppendUserMessage(ctx context.Context, params AppendUserMessageParams) (Message, error)
	// StartGeneration opens a new streaming Assistant Message for the latest
	// User Message in the conversation under the one-active-generation guard.
	// It is idempotent by key: a repeated key returns the existing message with
	// created=false instead of duplicating the message or the Provider call.
	StartGeneration(ctx context.Context, conversationID, idempotencyKey string) (Message, bool, error)
	// PeekGeneration reports whether a generation with the given idempotency key
	// already exists and returns it. It lets BeginStream detect a transport
	// replay before any mode or concurrency check.
	PeekGeneration(ctx context.Context, conversationID, idempotencyKey string) (Message, bool, error)
	// CheckpointGeneration durably persists buffered partial content during
	// streaming. It must not write on every token.
	CheckpointGeneration(ctx context.Context, messageID, content string) error
	// CompleteGeneration synchronously persists the terminal content, status,
	// Provider metadata, token usage, finish reason, and safe error before the
	// terminal stream event is emitted.
	CompleteGeneration(ctx context.Context, messageID string, result GenerationResult) error
}
