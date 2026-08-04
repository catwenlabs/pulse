package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
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
