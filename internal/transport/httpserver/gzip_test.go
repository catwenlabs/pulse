package httpserver

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func decodeGzipBody(t *testing.T, body io.Reader) string {
	t.Helper()
	reader, err := gzip.NewReader(body)
	if err != nil {
		t.Fatalf("gzip.NewReader() error = %v", err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read gzip body error = %v", err)
	}
	return string(decoded)
}

func TestWithGzipCompressesJSONResponses(t *testing.T) {
	payload := strings.Repeat(`{"stories":[{"representative":{"content_html":"<p>正文</p>"}}]}`, 200)
	handler := withGzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, payload)
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/stories", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	handler.ServeHTTP(recorder, request)
	response := recorder.Result()

	if encoding := response.Header.Get("Content-Encoding"); encoding != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", encoding)
	}
	if vary := response.Header.Get("Vary"); !strings.Contains(vary, "Accept-Encoding") {
		t.Fatalf("Vary = %q, want Accept-Encoding", vary)
	}
	if decoded := decodeGzipBody(t, response.Body); decoded != payload {
		t.Fatalf("decoded body mismatch: got %d bytes, want %d", len(decoded), len(payload))
	}
	if recorder.Body.Len() >= len(payload) {
		t.Fatalf("compressed body (%d bytes) should be smaller than raw (%d bytes)", recorder.Body.Len(), len(payload))
	}
}

func TestWithGzipSkipsClientsWithoutAcceptEncoding(t *testing.T) {
	handler := withGzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/stories", nil))
	response := recorder.Result()

	if encoding := response.Header.Get("Content-Encoding"); encoding != "" {
		t.Fatalf("Content-Encoding = %q, want empty", encoding)
	}
	if body, err := io.ReadAll(response.Body); err != nil || string(body) != `{"ok":true}` {
		t.Fatalf("body = %q, err = %v", body, err)
	}
}

func TestWithGzipHonorsExplicitQZero(t *testing.T) {
	handler := withGzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))

	for _, encoding := range []string{"gzip;q=0", "gzip; q=0.0, deflate"} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/stories", nil)
		request.Header.Set("Accept-Encoding", encoding)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if got := recorder.Result().Header.Get("Content-Encoding"); got != "" {
			t.Fatalf("Accept-Encoding %q: Content-Encoding = %q, want empty", encoding, got)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/stories", nil)
	request.Header.Set("Accept-Encoding", "gzip; q=0.5")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if got := recorder.Result().Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Accept-Encoding gzip;q=0.5: Content-Encoding = %q, want gzip", got)
	}
}

func TestWithGzipPassesEventStreamsThrough(t *testing.T) {
	handler := withGzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		flusher := w.(http.Flusher)
		_, _ = io.WriteString(w, "retry: 3000\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "event: library-change\ndata: {}\n\n")
		flusher.Flush()
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))
	response := recorder.Result()

	if encoding := response.Header.Get("Content-Encoding"); encoding != "" {
		t.Fatalf("Content-Encoding = %q, want empty for event streams", encoding)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}
	if !strings.Contains(string(body), "event: library-change") {
		t.Fatalf("event stream body = %q", body)
	}
}

func TestWithGzipSkipsBodylessStatuses(t *testing.T) {
	handler := withGzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/sources/x/runs", nil))
	response := recorder.Result()

	if encoding := response.Header.Get("Content-Encoding"); encoding != "" {
		t.Fatalf("Content-Encoding = %q, want empty for 204", encoding)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("204 body = %q, want empty", recorder.Body.String())
	}
}

func TestWithGzipDropsContentLengthWhenCompressing(t *testing.T) {
	payload := strings.Repeat("padding-", 512)
	handler := withGzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = io.WriteString(w, payload)
	}))

	recorder := httptest.NewRecorder()
	contentLengthRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	contentLengthRequest.Header.Set("Accept-Encoding", "gzip")
	handler.ServeHTTP(recorder, contentLengthRequest)
	response := recorder.Result()

	if length := response.Header.Get("Content-Length"); length != "" {
		t.Fatalf("Content-Length = %q, want it dropped while compressing", length)
	}
	if decoded := decodeGzipBody(t, response.Body); decoded != payload {
		t.Fatalf("decoded body mismatch: got %d bytes, want %d", len(decoded), len(payload))
	}
}

func TestWithGzipSkipsRangeRequests(t *testing.T) {
	handler := withGzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/stories", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	request.Header.Set("Range", "bytes=0-3")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if got := recorder.Result().Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty for range requests", got)
	}
}

func TestWithGzipSkipsCompressedMedia(t *testing.T) {
	handler := withGzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = io.WriteString(w, "pngbytes")
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/logo.png", nil))
	if got := recorder.Result().Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty for images", got)
	}
}

func TestNewHandlerServesGzipJSON(t *testing.T) {
	handler := NewHandler()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	response := recorder.Result()
	if response.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", response.Header.Get("Content-Encoding"))
	}
	if decoded := decodeGzipBody(t, response.Body); !strings.Contains(decoded, `"status"`) {
		t.Fatalf("decoded healthz body = %q", decoded)
	}
}
