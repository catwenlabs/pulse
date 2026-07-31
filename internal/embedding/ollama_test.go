package embedding

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestOllamaEmbedsBatch(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/api/embed" {
			t.Errorf("path = %q", request.URL.Path)
		}
		var body struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "qwen3-embedding" || len(body.Input) != 2 {
			t.Errorf("body = %+v", body)
		}
		return jsonResponse(`{"model":"qwen3-embedding","embeddings":[[1,0],[0,1]]}`), nil
	})}

	provider, err := NewOllama("http://ollama:11434/", "qwen3-embedding", client)
	if err != nil {
		t.Fatalf("NewOllama() error = %v", err)
	}
	got, err := provider.Embed(context.Background(), []string{"one", "two"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(got) != 2 || len(got[0]) != 2 {
		t.Fatalf("Embed() = %#v", got)
	}
}

func TestOllamaRejectsInvalidBaseURL(t *testing.T) {
	if _, err := NewOllama("file:///tmp/model", "qwen3-embedding", nil); err == nil {
		t.Fatal("NewOllama() error = nil")
	}
}

func TestOllamaRejectsUnexpectedDimensionsWithinBatch(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"model":"qwen3-embedding","embeddings":[[1,0],[0,1,2]]}`), nil
	})}

	provider, err := NewOllama("http://ollama:11434", "qwen3-embedding", client)
	if err != nil {
		t.Fatalf("NewOllama() error = %v", err)
	}
	if _, err := provider.Embed(context.Background(), []string{"one", "two"}); err == nil {
		t.Fatal("Embed() error = nil")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
