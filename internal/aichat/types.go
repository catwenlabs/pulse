// Package aichat implements Pulse's independent AI Conversation domain: a
// global set of selection tools and durable Conversations rooted in a text
// selection. The module is intentionally independent of Source, Entry, Story,
// Acquisition, Rule, View, Folder, and Effect. It reuses the shared
// OpenAI-compatible Provider configuration through the StreamingProvider seam
// but owns its own persistence, memory assembly, and interactive streaming.
package aichat

import (
	"errors"
	"fmt"
	"time"
)

const (
	// SelectionPlaceholder is the only supported prompt-template placeholder.
	SelectionPlaceholder = "{{selection}}"

	// MaxToolNameLength bounds a tool's display name after trimming.
	MaxToolNameLength = 40
	// MaxPromptTemplateLength bounds a tool's prompt template length.
	MaxPromptTemplateLength = 4000
	// MaxSelectionCharacters bounds the selected text supplied by the client.
	MaxSelectionCharacters = 10000
)

// MessageRole identifies the author of a Message.
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
)

// GenerationStatus describes the lifecycle of an Assistant Message. User
// messages do not carry a status.
type GenerationStatus string

const (
	StatusStreaming GenerationStatus = "streaming"
	StatusCompleted GenerationStatus = "completed"
	StatusCancelled GenerationStatus = "cancelled"
	StatusFailed    GenerationStatus = "failed"
)

// IsTerminal reports whether the status is a final, non-streaming state.
func (s GenerationStatus) IsTerminal() bool {
	return s == StatusCompleted || s == StatusCancelled || s == StatusFailed
}

// SelectionTool is one user-configurable selection action.
type SelectionTool struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	PromptTemplate string    `json:"prompt_template"`
	Enabled        bool      `json:"enabled"`
	Position       int       `json:"position"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAtAt    time.Time `json:"updated_at"`
}

// Conversation is an independent AI chat thread rooted in a selection. It
// stores immutable snapshots of the tool name, prompt template, and selected
// text. It never references Source, Entry, Story, page origin, or tool ID.
type Conversation struct {
	ID             string    `json:"id"`
	SelectedText   string    `json:"selected_text"`
	ToolName       string    `json:"tool_name"`
	PromptTemplate string    `json:"prompt_template"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Excerpt returns a bounded, single-line excerpt of the selected text suitable
// for history labels. It does not allocate when the text already fits.
func (c Conversation) Excerpt(max int) string {
	if max <= 0 {
		max = 80
	}
	text := normalizeWhitespace(c.SelectedText)
	if len([]rune(text)) <= max {
		return text
	}
	runes := []rune(text)
	return string(runes[:max]) + "…"
}

// Message is a single entry in a Conversation.
type Message struct {
	ID               string           `json:"id"`
	ConversationID   string           `json:"conversation_id"`
	Role             MessageRole      `json:"role"`
	Content          string           `json:"content"`
	Status           GenerationStatus `json:"status,omitempty"`
	Provider         string           `json:"provider,omitempty"`
	Model            string           `json:"model,omitempty"`
	PromptTokens     int              `json:"prompt_tokens,omitempty"`
	CompletionTokens int              `json:"completion_tokens,omitempty"`
	FinishReason     string           `json:"finish_reason,omitempty"`
	Error            string           `json:"error,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

// ToolInput captures the editable fields of a selection tool.
type ToolInput struct {
	Name           string `json:"name"`
	PromptTemplate string `json:"prompt_template"`
	Enabled        bool   `json:"enabled"`
}

// CreateConversationInput is the client request that starts a Conversation.
// The client supplies a tool ID; the server resolves the enabled tool,
// validates the selection, expands the template, and persists snapshots.
type CreateConversationInput struct {
	ToolID    string `json:"tool_id"`
	Selection string `json:"selection"`
}

// FollowUpInput is a follow-up user message in an existing Conversation.
type FollowUpInput struct {
	Content string `json:"content"`
}

// ConversationPage is one cursor-paginated slice of history ordered by
// UpdatedAt descending with a stable ID tie-breaker.
type ConversationPage struct {
	Items      []Conversation `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"`
	HasMore    bool           `json:"has_more"`
}

// GenerationResult is the terminal outcome of an Assistant generation,
// persisted synchronously before the terminal stream event is emitted.
type GenerationResult struct {
	Status           GenerationStatus
	Content          string
	Provider         string
	Model            string
	PromptTokens     int
	CompletionTokens int
	FinishReason     string
	Error            string
}

// Sentinel domain errors. These are matched with errors.Is at service and HTTP
// boundaries, so they must remain pointer-free values.
var (
	ErrUnavailable          = errors.New("AI chat is unavailable")
	ErrToolNotFound         = errors.New("selection tool not found")
	ErrConversationNotFound = errors.New("AI conversation not found")
	ErrMessageNotFound      = errors.New("AI message not found")
	ErrActiveGeneration     = errors.New("conversation already has an active generation")
	ErrNoPendingMessage     = errors.New("conversation has no message awaiting a reply")
	ErrNotRetryable         = errors.New("message is not retryable")
	ErrSelectionRequired    = errors.New("selection is required")
	ErrInvalidCursor        = errors.New("invalid conversation cursor")
)

// ValidationError is a typed, field-scoped validation failure for tool and
// conversation inputs. Field is the JSON name of the offending parameter.
type ValidationError struct {
	Field   string
	Message string
}

func (err *ValidationError) Error() string {
	if err.Message == "" {
		return "validation error"
	}
	return err.Message
}

// DuplicateToolError indicates a case-insensitive tool name collision.
type DuplicateToolError struct {
	Name string
}

func (err *DuplicateToolError) Error() string {
	return fmt.Sprintf("a selection tool named %q already exists", err.Name)
}

// SelectionSizeError indicates the client supplied a selection larger than the
// configured maximum. The selection is rejected rather than truncated.
type SelectionSizeError struct {
	Count int
	Limit int
}

func (err *SelectionSizeError) Error() string {
	return fmt.Sprintf("selection is %d characters; limit is %d", err.Count, err.Limit)
}

// MemoryBudgetError indicates the assembled Provider input could not fit even
// the fixed System Prompt and the immutable initial selection within the
// configured Provider input limit.
type MemoryBudgetError struct {
	RequiredTokens int
	LimitTokens    int
}

func (err *MemoryBudgetError) Error() string {
	return fmt.Sprintf(
		"selection and system prompt require approximately %d tokens; Provider input limit is %d",
		err.RequiredTokens, err.LimitTokens,
	)
}
