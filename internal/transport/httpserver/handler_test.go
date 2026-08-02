package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/catwenlabs/pulse/internal/events"
)

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	NewHandler().ServeHTTP(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
}

func TestWebAppServesIndexAssetsAndSPAFallback(t *testing.T) {
	web := fstest.MapFS{
		"index.html":        &fstest.MapFile{Data: []byte("<main>Pulse</main>")},
		"assets/app.js":     &fstest.MapFile{Data: []byte("console.log('pulse')")},
		"assets/ignored.js": &fstest.MapFile{Data: []byte("ignored")},
	}
	handler := NewHandlerWithWeb(nil, web)

	tests := []struct {
		path        string
		wantStatus  int
		wantContent string
	}{
		{path: "/", wantStatus: http.StatusOK, wantContent: "Pulse"},
		{path: "/assets/app.js", wantStatus: http.StatusOK, wantContent: "console.log"},
		{path: "/sources/source-1", wantStatus: http.StatusOK, wantContent: "Pulse"},
		{path: "/api/v1/unknown", wantStatus: http.StatusNotFound},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.wantStatus {
			t.Errorf("%s status = %d, want %d", test.path, response.Code, test.wantStatus)
		}
		if test.wantContent != "" && !strings.Contains(response.Body.String(), test.wantContent) {
			t.Errorf("%s body = %q, want %q", test.path, response.Body.String(), test.wantContent)
		}
		if !strings.HasPrefix(test.path, "/api/") {
			if got := response.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'none'" {
				t.Errorf("%s Content-Security-Policy = %q", test.path, got)
			}
			if got := response.Header().Get("X-Frame-Options"); got != "DENY" {
				t.Errorf("%s X-Frame-Options = %q", test.path, got)
			}
		}
	}

	if _, err := fs.Stat(web, "assets/ignored.js"); err != nil {
		t.Fatalf("test filesystem: %v", err)
	}
}

func TestUnknownRoute(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	response := httptest.NewRecorder()

	NewHandler().ServeHTTP(response, req)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestLibraryEventsStreamsCommittedChangeSignals(t *testing.T) {
	hub := events.NewLibraryChangeHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(ctx)
	response := newTestStreamWriter()
	done := make(chan struct{})
	go func() {
		NewHandlerWithWebAndEvents(completeFakeBackend(), nil, hub).ServeHTTP(response, request)
		close(done)
	}()
	awaitFlush(t, response)
	if response.statusCode != http.StatusOK {
		t.Fatalf("status = %d", response.statusCode)
	}
	if got := response.Header().Get("Content-Type"); got != "text/event-stream; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := response.bodyString(); got != "retry: 3000\n\n" {
		t.Fatalf("initial event = %q", got)
	}

	hub.PublishSource("source-1")
	awaitFlush(t, response)
	want := "retry: 3000\n\nevent: library-change\nid: 1\ndata: {\"source_id\":\"source-1\"}\n\n"
	if got := response.bodyString(); got != want {
		t.Fatalf("event = %q, want %q", got, want)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("event stream did not stop after cancellation")
	}
}

func TestLibraryEventsSendHeartbeatsAndStopOnCancellation(t *testing.T) {
	hub := events.NewLibraryChangeHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(ctx)
	response := newTestStreamWriter()
	done := make(chan struct{})
	go func() {
		streamLibraryChangesWithHeartbeat(hub, 5*time.Millisecond).ServeHTTP(response, request)
		close(done)
	}()
	awaitFlush(t, response)
	awaitFlush(t, response)
	if got := response.bodyString(); !strings.Contains(got, ": heartbeat\n\n") {
		t.Fatalf("stream = %q, want heartbeat", got)
	}
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not close promptly")
	}
}

type testStreamWriter struct {
	mu         sync.Mutex
	header     http.Header
	body       bytes.Buffer
	statusCode int
	flushed    chan struct{}
}

func newTestStreamWriter() *testStreamWriter {
	return &testStreamWriter{header: make(http.Header), flushed: make(chan struct{}, 8)}
}

func (writer *testStreamWriter) Header() http.Header {
	return writer.header
}

func (writer *testStreamWriter) WriteHeader(statusCode int) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.statusCode == 0 {
		writer.statusCode = statusCode
	}
}

func (writer *testStreamWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.statusCode == 0 {
		writer.statusCode = http.StatusOK
	}
	return writer.body.Write(data)
}

func (writer *testStreamWriter) Flush() {
	select {
	case writer.flushed <- struct{}{}:
	default:
	}
}

func (writer *testStreamWriter) bodyString() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.body.String()
}

func awaitFlush(t *testing.T, writer *testStreamWriter) {
	t.Helper()
	select {
	case <-writer.flushed:
	case <-time.After(time.Second):
		t.Fatal("stream did not flush")
	}
}
