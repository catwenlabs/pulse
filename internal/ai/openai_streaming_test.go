package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func newStreamingAdapter(t *testing.T, transport roundTripFunc) *OpenAICompatibleAdapter {
	t.Helper()
	adapter, err := NewOpenAICompatible(OpenAICompatibleConfig{
		ProviderName: "fake",
		BaseURL:      "https://example.com/v1",
		Model:        "fake-model",
		Client:       &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatible() error = %v", err)
	}
	return adapter
}

func TestStreamEmitsDeltasAndReportsUsage(t *testing.T) {
	body := strings.Join([]string{
		`data: {"model":"fake-model","choices":[{"delta":{"content":"Hello"}}]}`,
		``,
		`data: {"model":"fake-model","choices":[{"delta":{"content":" world"}}]}`,
		``,
		`data: {"model":"fake-model","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":2}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	adapter := newStreamingAdapter(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}))

	var collected strings.Builder
	result, err := adapter.Stream(context.Background(), GenerateRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(delta string) error {
		collected.WriteString(delta)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if collected.String() != "Hello world" {
		t.Errorf("collected = %q", collected.String())
	}
	if result.FinishReason != "stop" || result.PromptTokens != 7 || result.CompletionTokens != 2 {
		t.Errorf("result = %+v", result)
	}
}

func TestStreamSurfacesProviderHTTPError(t *testing.T) {
	adapter := newStreamingAdapter(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader("rate limited")),
			Header:     make(http.Header),
		}, nil
	}))
	_, err := adapter.Stream(context.Background(), GenerateRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(string) error { return nil })
	if err == nil {
		t.Fatal("Stream() expected error for 429")
	}
}

func TestStreamStopsWhenEmitReportsClientGone(t *testing.T) {
	body := strings.Join([]string{
		`data: {"model":"fake-model","choices":[{"delta":{"content":"one"}}]}`,
		``,
		`data: {"model":"fake-model","choices":[{"delta":{"content":"two"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	adapter := newStreamingAdapter(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}))
	count := 0
	_, err := adapter.Stream(context.Background(), GenerateRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(string) error {
		count++
		if count == 1 {
			return errors.New("client gone")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if count != 1 {
		t.Errorf("emit called %d times, want 1 (should stop after client gone)", count)
	}
}
