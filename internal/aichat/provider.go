package aichat

import "context"

// ProviderMessage is one role/content pair sent to the Provider. It mirrors
// ai.Message but keeps the aichat module's types self-contained.
type ProviderMessage struct {
	Role    MessageRole
	Content string
}

// StreamRequest is the assembled short-term memory sent to the Provider.
type StreamRequest struct {
	Messages []ProviderMessage
}

// ProviderMetadata names the Provider and model that produced a result.
type ProviderMetadata struct {
	Name  string
	Model string
}

// StreamResult is the terminal outcome of a successful Provider stream. The
// provider reports the model it used (which may differ from the configured
// model name), the finish reason, and token usage.
type StreamResult struct {
	Model            string
	FinishReason     string
	PromptTokens     int
	CompletionTokens int
}

// StreamingProvider is the interactive chat seam. It reuses the global
// OpenAI-compatible Provider configuration but streams content as it arrives,
// preserving the existing non-streaming generation behavior used by Story
// summaries and Digests.
//
// Stream calls emit for each content delta and returns a StreamResult on
// success. On Provider failure (before or after partial output) it returns a
// non-nil error; deltas already emitted remain buffered by the caller. When the
// context is canceled the provider stops promptly and returns the context
// error.
type StreamingProvider interface {
	Metadata() ProviderMetadata
	Stream(ctx context.Context, request StreamRequest, emit func(delta string) error) (StreamResult, error)
}

// ChatStreamEvent is one event serialized onto the HTTP fetch stream. The
// metadata event always comes first; terminal events (Completed, Cancelled,
// Failed) come last and carry persisted status and safe usage metadata.
type ChatStreamEvent struct {
	Kind             StreamEventKind `json:"kind"`
	ConversationID   string          `json:"conversation_id,omitempty"`
	MessageID        string          `json:"message_id,omitempty"`
	Delta            string          `json:"delta,omitempty"`
	Content          string          `json:"content,omitempty"`
	Status           GenerationStatus `json:"status,omitempty"`
	Provider         string          `json:"provider,omitempty"`
	Model            string          `json:"model,omitempty"`
	PromptTokens     int             `json:"prompt_tokens,omitempty"`
	CompletionTokens int             `json:"completion_tokens,omitempty"`
	FinishReason     string          `json:"finish_reason,omitempty"`
	Error            string          `json:"error,omitempty"`
}

// StreamEventKind identifies the shape of a ChatStreamEvent.
type StreamEventKind string

const (
	StreamEventMetadata  StreamEventKind = "metadata"
	StreamEventDelta     StreamEventKind = "delta"
	StreamEventCompleted StreamEventKind = "completed"
	StreamEventCancelled StreamEventKind = "cancelled"
	StreamEventFailed    StreamEventKind = "failed"
)
