package aichat

import "context"

// Chat is the public contract of the AI Chat service. The concrete *Service
// implements it; HTTP and other callers depend on this interface so behavior
// can be faked at the seam.
type Chat interface {
	// Tools
	ListTools(ctx context.Context) ([]SelectionTool, error)
	CreateTool(ctx context.Context, input ToolInput) (SelectionTool, error)
	UpdateTool(ctx context.Context, id string, input ToolInput) (SelectionTool, error)
	DeleteTool(ctx context.Context, id string) error
	ReorderTools(ctx context.Context, ids []string) ([]SelectionTool, error)

	// Conversations
	CreateConversation(ctx context.Context, input CreateConversationInput, idempotencyKey string) (Conversation, Message, error)
	ListConversations(ctx context.Context, limit int, cursor string) (ConversationPage, error)
	GetConversation(ctx context.Context, id string) (Conversation, []Message, error)
	DeleteConversation(ctx context.Context, id string) error
	SendFollowUp(ctx context.Context, conversationID string, input FollowUpInput, idempotencyKey string) (Message, error)

	// Generation (streaming)
	BeginStream(ctx context.Context, conversationID, idempotencyKey string, mode StreamMode) (*StreamSession, error)
	DriveStream(ctx context.Context, session *StreamSession, sink func(ChatStreamEvent) error) error
	Stop(ctx context.Context, conversationID string) error
}
