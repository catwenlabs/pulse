package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/catwenlabs/pulse/internal/aichat"
)

// aiChatBackend is the optional AI Chat capability, discovered by route
// registration via type assertion just like aiBackend.
type aiChatBackend interface {
	ListChatTools(context.Context) ([]aichat.SelectionTool, error)
	CreateChatTool(context.Context, aichat.ToolInput) (aichat.SelectionTool, error)
	UpdateChatTool(context.Context, string, aichat.ToolInput) (aichat.SelectionTool, error)
	DeleteChatTool(context.Context, string) error
	ReorderChatTools(context.Context, []string) ([]aichat.SelectionTool, error)
	CreateConversation(context.Context, aichat.CreateConversationInput, string) (aichat.Conversation, aichat.Message, error)
	ListConversations(context.Context, int, string) (aichat.ConversationPage, error)
	GetConversation(context.Context, string) (aichat.Conversation, []aichat.Message, error)
	DeleteConversation(context.Context, string) error
	SendFollowUp(context.Context, string, aichat.FollowUpInput, string) (aichat.Message, error)
	BeginStream(context.Context, string, string, aichat.StreamMode) (*aichat.StreamSession, error)
	DriveStream(context.Context, *aichat.StreamSession, func(aichat.ChatStreamEvent) error) error
	StopGeneration(context.Context, string) error
}

// registerAIChatRoutes wires the AI Chat endpoints when the backend supports
// them. When the capability is absent (AI disabled), no chat routes register
// and requests return 404.
func registerAIChatRoutes(mux *http.ServeMux, backend Backend) {
	chat, ok := backend.(aiChatBackend)
	if !ok {
		return
	}
	mux.HandleFunc("GET /api/v1/ai/tools", listChatTools(chat))
	mux.HandleFunc("POST /api/v1/ai/tools", createChatTool(chat))
	mux.HandleFunc("PUT /api/v1/ai/tools/order", reorderChatTools(chat))
	mux.HandleFunc("GET /api/v1/ai/tools/{id}", getChatTool(chat))
	mux.HandleFunc("PUT /api/v1/ai/tools/{id}", updateChatTool(chat))
	mux.HandleFunc("DELETE /api/v1/ai/tools/{id}", deleteChatTool(chat))
	mux.HandleFunc("GET /api/v1/ai/conversations", listConversations(chat))
	mux.HandleFunc("POST /api/v1/ai/conversations", createConversation(chat))
	mux.HandleFunc("GET /api/v1/ai/conversations/{id}", getConversation(chat))
	mux.HandleFunc("DELETE /api/v1/ai/conversations/{id}", deleteConversation(chat))
	mux.HandleFunc("POST /api/v1/ai/conversations/{id}/messages", sendFollowUp(chat))
	mux.HandleFunc("POST /api/v1/ai/conversations/{id}/generate", generateStream(chat))
	mux.HandleFunc("POST /api/v1/ai/conversations/{id}/retry", retryStream(chat))
	mux.HandleFunc("POST /api/v1/ai/conversations/{id}/stop", stopGeneration(chat))
}

func listChatTools(chat aiChatBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		tools, err := chat.ListChatTools(request.Context())
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, nonNilSlice(tools))
	}
}

func getChatTool(chat aiChatBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		tools, err := chat.ListChatTools(request.Context())
		if err != nil {
			writeDomainError(w, err)
			return
		}
		for _, tool := range tools {
			if tool.ID == request.PathValue("id") {
				writeJSON(w, http.StatusOK, tool)
				return
			}
		}
		writeProblem(w, http.StatusNotFound, "ai_tool_not_found", aichat.ErrToolNotFound.Error(), "")
	}
}

func createChatTool(chat aiChatBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		var body aichat.ToolInput
		if err := decodeJSONBody(w, request, &body); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		tool, err := chat.CreateChatTool(request.Context(), body)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, tool)
	}
}

func updateChatTool(chat aiChatBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		var body aichat.ToolInput
		if err := decodeJSONBody(w, request, &body); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		tool, err := chat.UpdateChatTool(request.Context(), request.PathValue("id"), body)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tool)
	}
}

func deleteChatTool(chat aiChatBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if err := chat.DeleteChatTool(request.Context(), request.PathValue("id")); err != nil {
			writeDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func reorderChatTools(chat aiChatBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		var body struct {
			ToolIDs []string `json:"tool_ids"`
		}
		if err := decodeJSONBody(w, request, &body); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		tools, err := chat.ReorderChatTools(request.Context(), body.ToolIDs)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, nonNilSlice(tools))
	}
}

func listConversations(chat aiChatBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		limit := 50
		if value := strings.TrimSpace(request.URL.Query().Get("limit")); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 1 || parsed > 100 {
				writeProblem(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 100", "limit")
				return
			}
			limit = parsed
		}
		page, err := chat.ListConversations(request.Context(), limit, request.URL.Query().Get("cursor"))
		if err != nil {
			writeDomainError(w, err)
			return
		}
		page.Items = nonNilSlice(page.Items)
		writeJSON(w, http.StatusOK, page)
	}
}

func createConversation(chat aiChatBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		var body aichat.CreateConversationInput
		if err := decodeJSONBody(w, request, &body); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		conversation, userMessage, err := chat.CreateConversation(request.Context(), body, request.Header.Get("Idempotency-Key"))
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, struct {
			aichat.Conversation  `json:"conversation"`
			UserMessage          aichat.Message `json:"user_message"`
		}{Conversation: conversation, UserMessage: userMessage})
	}
}

func getConversation(chat aiChatBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		conversation, messages, err := chat.GetConversation(request.Context(), request.PathValue("id"))
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			aichat.Conversation `json:"conversation"`
			Messages            []aichat.Message `json:"messages"`
		}{Conversation: conversation, Messages: nonNilSlice(messages)})
	}
}

func deleteConversation(chat aiChatBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if err := chat.DeleteConversation(request.Context(), request.PathValue("id")); err != nil {
			writeDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func sendFollowUp(chat aiChatBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		var body aichat.FollowUpInput
		if err := decodeJSONBody(w, request, &body); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		message, err := chat.SendFollowUp(request.Context(), request.PathValue("id"), body, request.Header.Get("Idempotency-Key"))
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, message)
	}
}

func stopGeneration(chat aiChatBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if err := chat.StopGeneration(request.Context(), request.PathValue("id")); err != nil {
			writeDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func generateStream(chat aiChatBackend) http.HandlerFunc {
	return driveChatStream(chat, aichat.StreamModeGenerate)
}

func retryStream(chat aiChatBackend) http.HandlerFunc {
	return driveChatStream(chat, aichat.StreamModeRetry)
}

// driveChatStream opens a generation and streams metadata, deltas, and the
// terminal event as SSE. BeginStream runs before any bytes are written, so a
// rejection (active generation, validation, missing conversation) is returned
// as a normal JSON error rather than a broken stream.
func driveChatStream(chat aiChatBackend, mode aichat.StreamMode) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		session, err := chat.BeginStream(request.Context(), request.PathValue("id"), request.Header.Get("Idempotency-Key"), mode)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		defer session.Close()
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeProblem(w, http.StatusInternalServerError, "streaming_unsupported", "streaming is not supported", "")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		sink := func(event aichat.ChatStreamEvent) error {
			return writeChatStreamEvent(w, flusher, event)
		}
		// DriveStream runs until the generation terminates; the request context
		// cancels on client disconnect and is propagated to the Provider.
		_ = chat.DriveStream(request.Context(), session, sink)
	}
}

// writeChatStreamEvent serializes one event as an SSE message and flushes it.
// It returns a non-nil error when the write fails so the service can stop the
// Provider and record the partial failed result.
func writeChatStreamEvent(w http.ResponseWriter, flusher http.Flusher, event aichat.ChatStreamEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	var builder strings.Builder
	builder.WriteString("event: ")
	builder.WriteString(string(event.Kind))
	builder.WriteByte('\n')
	builder.WriteString("data: ")
	builder.Write(payload)
	builder.WriteString("\n\n")
	if _, err := w.Write([]byte(builder.String())); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
