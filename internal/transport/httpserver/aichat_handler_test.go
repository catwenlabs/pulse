package httpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/catwenlabs/pulse/internal/aichat"
)

type chatFakeBackend struct {
	fakeBackend
	listTools          func(context.Context) ([]aichat.SelectionTool, error)
	createTool         func(context.Context, aichat.ToolInput) (aichat.SelectionTool, error)
	updateTool         func(context.Context, string, aichat.ToolInput) (aichat.SelectionTool, error)
	deleteTool         func(context.Context, string) error
	reorderTools       func(context.Context, []string) ([]aichat.SelectionTool, error)
	createConversation func(context.Context, aichat.CreateConversationInput, string) (aichat.Conversation, aichat.Message, error)
	listConversations  func(context.Context, int, string) (aichat.ConversationPage, error)
	getConversation    func(context.Context, string) (aichat.Conversation, []aichat.Message, error)
	deleteConversation func(context.Context, string) error
	sendFollowUp       func(context.Context, string, aichat.FollowUpInput, string) (aichat.Message, error)
	beginStream        func(context.Context, string, string, aichat.StreamMode) (*aichat.StreamSession, error)
	driveStream        func(context.Context, *aichat.StreamSession, func(aichat.ChatStreamEvent) error) error
	stop               func(context.Context, string) error
}

func (f chatFakeBackend) ListChatTools(ctx context.Context) ([]aichat.SelectionTool, error) {
	return f.listTools(ctx)
}
func (f chatFakeBackend) CreateChatTool(ctx context.Context, in aichat.ToolInput) (aichat.SelectionTool, error) {
	return f.createTool(ctx, in)
}
func (f chatFakeBackend) UpdateChatTool(ctx context.Context, id string, in aichat.ToolInput) (aichat.SelectionTool, error) {
	return f.updateTool(ctx, id, in)
}
func (f chatFakeBackend) DeleteChatTool(ctx context.Context, id string) error { return f.deleteTool(ctx, id) }
func (f chatFakeBackend) ReorderChatTools(ctx context.Context, ids []string) ([]aichat.SelectionTool, error) {
	return f.reorderTools(ctx, ids)
}
func (f chatFakeBackend) CreateConversation(ctx context.Context, in aichat.CreateConversationInput, key string) (aichat.Conversation, aichat.Message, error) {
	return f.createConversation(ctx, in, key)
}
func (f chatFakeBackend) ListConversations(ctx context.Context, limit int, cursor string) (aichat.ConversationPage, error) {
	return f.listConversations(ctx, limit, cursor)
}
func (f chatFakeBackend) GetConversation(ctx context.Context, id string) (aichat.Conversation, []aichat.Message, error) {
	return f.getConversation(ctx, id)
}
func (f chatFakeBackend) DeleteConversation(ctx context.Context, id string) error { return f.deleteConversation(ctx, id) }
func (f chatFakeBackend) SendFollowUp(ctx context.Context, id string, in aichat.FollowUpInput, key string) (aichat.Message, error) {
	return f.sendFollowUp(ctx, id, in, key)
}
func (f chatFakeBackend) BeginStream(ctx context.Context, id, key string, mode aichat.StreamMode) (*aichat.StreamSession, error) {
	return f.beginStream(ctx, id, key, mode)
}
func (f chatFakeBackend) DriveStream(ctx context.Context, s *aichat.StreamSession, sink func(aichat.ChatStreamEvent) error) error {
	return f.driveStream(ctx, s, sink)
}
func (f chatFakeBackend) StopGeneration(ctx context.Context, id string) error { return f.stop(ctx, id) }

func chatHandler(fake chatFakeBackend) http.Handler {
	return newHandler(fake, nil, nil)
}

func TestChatToolCRUDAndReorder(t *testing.T) {
	tools := []aichat.SelectionTool{{ID: "t1", Name: "AI 解读", PromptTemplate: "{{selection}}", Enabled: true, Position: 0}}
	var saved []string
	fake := chatFakeBackend{
		fakeBackend:    fakeBackend{},
		listTools:      func(context.Context) ([]aichat.SelectionTool, error) { return tools, nil },
		createTool:     func(_ context.Context, in aichat.ToolInput) (aichat.SelectionTool, error) { return aichat.SelectionTool{ID: "t2", Name: in.Name, PromptTemplate: in.PromptTemplate, Enabled: in.Enabled}, nil },
		updateTool:     func(_ context.Context, id string, in aichat.ToolInput) (aichat.SelectionTool, error) { return aichat.SelectionTool{ID: id, Name: in.Name, PromptTemplate: in.PromptTemplate}, nil },
		deleteTool:     func(context.Context, string) error { return nil },
		reorderTools:   func(_ context.Context, ids []string) ([]aichat.SelectionTool, error) { saved = ids; return tools, nil },
	}
	handler := chatHandler(fake)

	listResp := httptest.NewRecorder()
	handler.ServeHTTP(listResp, httptest.NewRequest(http.MethodGet, "/api/v1/ai/tools", nil))
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status = %d", listResp.Code)
	}

	createResp := httptest.NewRecorder()
	handler.ServeHTTP(createResp, httptest.NewRequest(http.MethodPost, "/api/v1/ai/tools", body(t, aichat.ToolInput{Name: "AI 翻译", PromptTemplate: "{{selection}}"})))
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", createResp.Code, createResp.Body.String())
	}

	reorderResp := httptest.NewRecorder()
	handler.ServeHTTP(reorderResp, httptest.NewRequest(http.MethodPut, "/api/v1/ai/tools/order", body(t, map[string]any{"tool_ids": []string{"t1", "t2"}})))
	if reorderResp.Code != http.StatusOK {
		t.Fatalf("reorder status = %d", reorderResp.Code)
	}
	if len(saved) != 2 || saved[0] != "t1" || saved[1] != "t2" {
		t.Errorf("reorder saved = %v", saved)
	}
}

func TestCreateConversationReturnsConversationAndUserMessage(t *testing.T) {
	var capturedKey string
	fake := chatFakeBackend{
		fakeBackend: fakeBackend{},
		createConversation: func(_ context.Context, in aichat.CreateConversationInput, key string) (aichat.Conversation, aichat.Message, error) {
			capturedKey = key
			return aichat.Conversation{ID: "c1", SelectedText: in.Selection, ToolName: "AI 解读"},
				aichat.Message{ID: "m1", Role: aichat.RoleUser, Content: "解读：E=mc^2"}, nil
		},
	}
	handler := chatHandler(fake)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations", body(t, aichat.CreateConversationInput{ToolID: "t1", Selection: "E=mc^2"}))
	req.Header.Set("Idempotency-Key", "client-key-1")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	if capturedKey != "client-key-1" {
		t.Errorf("idempotency key forwarded = %q", capturedKey)
	}
	var result struct {
		Conversation aichat.Conversation `json:"conversation"`
		UserMessage  aichat.Message      `json:"user_message"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Conversation.ID != "c1" || result.UserMessage.ID != "m1" {
		t.Errorf("result = %+v", result)
	}
}

func TestChatToolDuplicateMapsToConflict(t *testing.T) {
	fake := chatFakeBackend{
		fakeBackend: fakeBackend{},
		createTool:  func(context.Context, aichat.ToolInput) (aichat.SelectionTool, error) { return aichat.SelectionTool{}, &aichat.DuplicateToolError{Name: "AI 解读"} },
	}
	handler := chatHandler(fake)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/v1/ai/tools", body(t, aichat.ToolInput{Name: "AI 解读", PromptTemplate: "{{selection}}"})))
	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.Code)
	}
}

func TestGenerateStreamEmitsSSEEvents(t *testing.T) {
	fake := chatFakeBackend{
		fakeBackend: fakeBackend{},
		beginStream: func(_ context.Context, _, _ string, _ aichat.StreamMode) (*aichat.StreamSession, error) {
			return &aichat.StreamSession{}, nil
		},
		driveStream: func(_ context.Context, _ *aichat.StreamSession, sink func(aichat.ChatStreamEvent) error) error {
			_ = sink(aichat.ChatStreamEvent{Kind: aichat.StreamEventMetadata, ConversationID: "c1", MessageID: "m2"})
			_ = sink(aichat.ChatStreamEvent{Kind: aichat.StreamEventDelta, Delta: "Hello "})
			_ = sink(aichat.ChatStreamEvent{Kind: aichat.StreamEventDelta, Delta: "world"})
			_ = sink(aichat.ChatStreamEvent{Kind: aichat.StreamEventCompleted, Content: "Hello world", Status: aichat.StatusCompleted, PromptTokens: 5, CompletionTokens: 2})
			return nil
		},
	}
	handler := chatHandler(fake)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations/c1/generate", nil)
	req.Header.Set("Idempotency-Key", "gen-1")
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	if ct := resp.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q", ct)
	}
	events := parseSSE(resp.Body.String())
	if len(events) < 3 {
		t.Fatalf("events = %v", events)
	}
	if events[0].event != "metadata" {
		t.Errorf("first event = %q", events[0].event)
	}
	var deltas strings.Builder
	sawCompleted := false
	for _, ev := range events {
		if ev.event == "delta" {
			var payload aichat.ChatStreamEvent
			_ = json.Unmarshal([]byte(ev.data), &payload)
			deltas.WriteString(payload.Delta)
		}
		if ev.event == "completed" {
			sawCompleted = true
		}
	}
	if deltas.String() != "Hello world" {
		t.Errorf("deltas = %q", deltas.String())
	}
	if !sawCompleted {
		t.Errorf("no completed event")
	}
}

func TestGenerateStreamActiveGenerationReturnsConflict(t *testing.T) {
	fake := chatFakeBackend{
		fakeBackend: fakeBackend{},
		beginStream: func(context.Context, string, string, aichat.StreamMode) (*aichat.StreamSession, error) {
			return nil, aichat.ErrActiveGeneration
		},
	}
	handler := chatHandler(fake)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations/c1/generate", nil))
	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "ai_generation_active") {
		t.Errorf("body = %s", resp.Body.String())
	}
}

func TestStopGenerationReturnsNoContent(t *testing.T) {
	called := false
	fake := chatFakeBackend{
		fakeBackend: fakeBackend{},
		stop:        func(context.Context, string) error { called = true; return nil },
	}
	handler := chatHandler(fake)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations/c1/stop", nil))
	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d", resp.Code)
	}
	if !called {
		t.Errorf("Stop not called")
	}
}

func TestRetryUsesRetryMode(t *testing.T) {
	var observedMode aichat.StreamMode
	fake := chatFakeBackend{
		fakeBackend: fakeBackend{},
		beginStream: func(_ context.Context, _, _ string, mode aichat.StreamMode) (*aichat.StreamSession, error) {
			observedMode = mode
			return &aichat.StreamSession{}, nil
		},
		driveStream: func(_ context.Context, _ *aichat.StreamSession, sink func(aichat.ChatStreamEvent) error) error {
			_ = sink(aichat.ChatStreamEvent{Kind: aichat.StreamEventMetadata})
			_ = sink(aichat.ChatStreamEvent{Kind: aichat.StreamEventFailed, Status: aichat.StatusFailed, Error: "boom"})
			return nil
		},
	}
	handler := chatHandler(fake)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/v1/ai/conversations/c1/retry", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	if observedMode != aichat.StreamModeRetry {
		t.Errorf("mode = %v, want retry", observedMode)
	}
}

func TestChatEndpointUnavailableWithoutCapability(t *testing.T) {
	// A bare fakeBackend does not implement aiChatBackend, so routes do not
	// register and requests 404 rather than 500.
	handler := newHandler(fakeBackend{}, nil, nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/v1/ai/tools", nil))
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.Code)
	}
}

type sseEvent struct {
	event string
	data  string
}

func parseSSE(body string) []sseEvent {
	var events []sseEvent
	scanner := bufio.NewScanner(bytes.NewReader([]byte(body)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var current sseEvent
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if current.event != "" {
				events = append(events, current)
			}
			current = sseEvent{}
			continue
		}
		switch {
		case strings.HasPrefix(line, "event: "):
			current.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			current.data = strings.TrimPrefix(line, "data: ")
		}
	}
	if current.event != "" {
		events = append(events, current)
	}
	return events
}

func body(t *testing.T, value any) io.Reader {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewReader(data)
}
