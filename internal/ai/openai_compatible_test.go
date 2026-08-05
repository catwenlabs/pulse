package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func captureAILogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	previous := slog.Default()
	buffer := new(bytes.Buffer)
	slog.SetDefault(slog.New(slog.NewTextHandler(buffer, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})
	return buffer
}

func TestOpenAICompatibleAdapterLogsRequestAndResponseMetadata(t *testing.T) {
	logs := captureAILogs(t)
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"model":"deepseek-v4-flash","choices":[{"finish_reason":"stop","message":{"content":"{\"overview\":\"ok\"}"}}]}`)),
			Header:     make(http.Header),
		}, nil
	})}
	adapter, err := NewOpenAICompatible(OpenAICompatibleConfig{
		ProviderName: "deepseek",
		BaseURL:      "https://example.com/v1",
		APIKey:       "secret-api-key",
		Model:        "deepseek-v4-flash",
		Client:       client,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatible() error = %v", err)
	}
	if _, err := adapter.Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: "user", Content: "private prompt"}},
	}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	output := logs.String()
	for _, want := range []string{
		"AI Provider request",
		"provider=deepseek",
		"model=deepseek-v4-flash",
		"message_count=1",
		"AI Provider response",
		"finish_reason=stop",
		"content_present=true",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("logs do not contain %q: %s", want, output)
		}
	}
	if !strings.Contains(output, "request_body=") || !strings.Contains(output, "private prompt") {
		t.Errorf("logs do not contain the request body: %s", output)
	}
	for _, secret := range []string{"secret-api-key", "overview"} {
		if strings.Contains(output, secret) {
			t.Errorf("logs contain sensitive value %q: %s", secret, output)
		}
	}
}

func TestOpenAICompatibleAdapterLogsFinishReasonWhenContentIsEmpty(t *testing.T) {
	logs := captureAILogs(t)
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"model":"deepseek-v4-flash","choices":[{"finish_reason":"content_filter","message":{"content":""}}]}`)),
			Header:     make(http.Header),
		}, nil
	})}
	adapter, err := NewOpenAICompatible(OpenAICompatibleConfig{
		ProviderName: "deepseek",
		BaseURL:      "https://example.com/v1",
		Model:        "deepseek-v4-flash",
		Client:       client,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatible() error = %v", err)
	}
	if _, err := adapter.Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	}); err == nil || !strings.Contains(err.Error(), "no message content") {
		t.Fatalf("Generate() error = %v", err)
	}

	output := logs.String()
	for _, want := range []string{
		"AI Provider response has no message content",
		"finish_reason=content_filter",
		"content_present=false",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("logs do not contain %q: %s", want, output)
		}
	}
}

func TestOpenAICompatibleAdapterUsesConfiguredEndpointAndJSONMode(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://example.com/v1/chat/completions" {
			t.Errorf("URL = %s", request.URL)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q", got)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		var payload struct {
			Model          string `json:"model"`
			ResponseFormat struct {
				Type string `json:"type"`
			} `json:"response_format"`
			Stream bool `json:"stream"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "qwen-plus" || payload.ResponseFormat.Type != "json_object" || payload.Stream {
			t.Errorf("request payload = %+v", payload)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"model":"qwen-plus","choices":[{"message":{"content":"{\"overview\":\"ok\"}"}}],"usage":{"prompt_tokens":3,"completion_tokens":4}}`)),
			Header:     make(http.Header),
		}, nil
	})}
	adapter, err := NewOpenAICompatible(OpenAICompatibleConfig{
		BaseURL: "https://example.com/v1/",
		APIKey:  "secret",
		Model:   "qwen-plus",
		Client:  client,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatible() error = %v", err)
	}
	response, err := adapter.Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
		JSONMode: true,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if response.Content != `{"overview":"ok"}` || response.PromptTokens != 3 || response.CompletionTokens != 4 {
		t.Errorf("response = %+v", response)
	}
	if metadata := adapter.Metadata(); metadata.Name != "openai-compatible" || metadata.Model != "qwen-plus" {
		t.Errorf("metadata = %+v", metadata)
	}
}

func TestOpenAICompatibleAdapterDisablesThinkingWhenConfigured(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		var payload struct {
			Thinking *struct {
				Type string `json:"type"`
			} `json:"thinking"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Thinking == nil || payload.Thinking.Type != "disabled" {
			t.Fatalf("thinking control = %+v, want disabled", payload.Thinking)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"model":"deepseek-v4-flash","choices":[{"message":{"content":"{\"overview\":\"ok\"}"}}]}`)),
			Header:     make(http.Header),
		}, nil
	})}
	adapter, err := NewOpenAICompatible(OpenAICompatibleConfig{
		ProviderName:    "openai-compatible",
		BaseURL:         "https://api.deepseek.com",
		Model:           "deepseek-v4-flash",
		DisableThinking: true,
		Client:          client,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatible() error = %v", err)
	}
	if _, err := adapter.Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: "user", Content: "summarize"}},
		JSONMode: true,
	}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
}

func TestOpenAICompatibleAdapterRejectsInvalidConfiguration(t *testing.T) {
	for _, config := range []OpenAICompatibleConfig{
		{BaseURL: "file:///tmp/provider", Model: "model"},
		{BaseURL: "https://example.com", Model: ""},
		{BaseURL: "https://example.com?token=secret", Model: "model"},
		{BaseURL: "https://user:secret@example.com/v1", Model: "model"},
	} {
		if _, err := NewOpenAICompatible(config); err == nil {
			t.Errorf("NewOpenAICompatible(%+v) error = nil", config)
		}
	}
}

func TestOpenAICompatibleAdapterFallsBackWhenJSONModeIsUnsupported(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		var payload struct {
			ResponseFormat map[string]string `json:"response_format"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if requests == 1 {
			if payload.ResponseFormat["type"] != "json_object" {
				t.Fatalf("first request response_format = %#v", payload.ResponseFormat)
			}
			return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader("unsupported response_format")), Header: make(http.Header)}, nil
		}
		if len(payload.ResponseFormat) != 0 {
			t.Fatalf("fallback request response_format = %#v", payload.ResponseFormat)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"model":"model","choices":[{"message":{"content":"{\"overview\":\"ok\"}"}}]}`)),
			Header:     make(http.Header),
		}, nil
	})}
	adapter, err := NewOpenAICompatible(OpenAICompatibleConfig{BaseURL: "https://example.com/v1", Model: "model", Client: client})
	if err != nil {
		t.Fatalf("NewOpenAICompatible() error = %v", err)
	}
	response, err := adapter.Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
		JSONMode: true,
	})
	if err != nil || response.Content == "" || requests != 2 {
		t.Fatalf("Generate() = response=%+v err=%v requests=%d", response, err, requests)
	}
}

func TestOpenAICompatibleAdapterFallsBackForAlternateUnsupportedJSONStatuses(t *testing.T) {
	for _, status := range []int{http.StatusUnprocessableEntity, http.StatusNotImplemented} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			requests := 0
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				requests++
				if requests == 1 {
					return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("unsupported response_format")), Header: make(http.Header)}, nil
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"model":"model","choices":[{"message":{"content":"{\"overview\":\"ok\"}"}}]}`)),
					Header:     make(http.Header),
				}, nil
			})}
			adapter, err := NewOpenAICompatible(OpenAICompatibleConfig{BaseURL: "https://example.com/v1", Model: "model", Client: client})
			if err != nil {
				t.Fatalf("NewOpenAICompatible() error = %v", err)
			}
			response, err := adapter.Generate(context.Background(), GenerateRequest{
				Messages: []Message{{Role: "user", Content: "hello"}},
				JSONMode: true,
			})
			if err != nil || response.Content == "" || requests != 2 {
				t.Fatalf("Generate() = response=%+v err=%v requests=%d", response, err, requests)
			}
		})
	}
}
