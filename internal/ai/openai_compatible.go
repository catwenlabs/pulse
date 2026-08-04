package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/catwenlabs/pulse/internal/platform/httpclient"
)

const maxProviderResponseBytes = 8 << 20

type OpenAICompatibleConfig struct {
	ProviderName string
	BaseURL      string
	APIKey       string
	Model        string
	Headers      map[string]string
	Client       *http.Client
	Timeout      time.Duration
}

type OpenAICompatibleAdapter struct {
	providerName  string
	completionURL string
	model         string
	headers       http.Header
	client        *http.Client
}

func NewOpenAICompatible(config OpenAICompatibleConfig) (*OpenAICompatibleAdapter, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("AI Base URL must be an absolute HTTP(S) URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("AI Base URL must not contain query or fragment")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("AI Base URL must not contain userinfo")
	}
	model := strings.TrimSpace(config.Model)
	if model == "" {
		return nil, fmt.Errorf("AI model is required")
	}
	providerName := strings.TrimSpace(config.ProviderName)
	if providerName == "" {
		providerName = "openai-compatible"
	}
	completionPath := parsed.Path
	if !strings.HasSuffix(strings.TrimRight(completionPath, "/"), "/chat/completions") {
		completionPath = path.Join(completionPath, "chat/completions")
	}
	parsed.Path = completionPath
	parsed.RawPath = ""
	client := config.Client
	if client == nil {
		timeout := config.Timeout
		if timeout <= 0 {
			timeout = 2 * time.Minute
		}
		client, err = httpclient.NewForAI(baseURL, timeout)
		if err != nil {
			return nil, fmt.Errorf("configure AI HTTP client: %w", err)
		}
	}
	headers := make(http.Header, len(config.Headers)+2)
	for name, value := range config.Headers {
		headers.Set(name, value)
	}
	if headers.Get("Content-Type") == "" {
		headers.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(config.APIKey) != "" && headers.Get("Authorization") == "" {
		headers.Set("Authorization", "Bearer "+strings.TrimSpace(config.APIKey))
	}
	return &OpenAICompatibleAdapter{
		providerName:  providerName,
		completionURL: parsed.String(),
		model:         model,
		headers:       headers,
		client:        client,
	}, nil
}

func (adapter *OpenAICompatibleAdapter) Metadata() ProviderMetadata {
	return ProviderMetadata{Name: adapter.providerName, Model: adapter.model}
}

func (adapter *OpenAICompatibleAdapter) Generate(ctx context.Context, request GenerateRequest) (GenerateResponse, error) {
	response, err := adapter.generate(ctx, request, request.JSONMode)
	var providerError *providerHTTPError
	if request.JSONMode && errors.As(err, &providerError) && shouldFallbackJSONMode(providerError.status) {
		// Some compatible endpoints reject response_format. The prompt and the
		// application parser still enforce structured output on the fallback.
		return adapter.generate(ctx, request, false)
	}
	return response, err
}

func (adapter *OpenAICompatibleAdapter) generate(ctx context.Context, request GenerateRequest, nativeJSON bool) (GenerateResponse, error) {
	if len(request.Messages) == 0 {
		return GenerateResponse{}, fmt.Errorf("AI request requires at least one message")
	}
	payload := struct {
		Model          string          `json:"model"`
		Messages       []Message       `json:"messages"`
		MaxTokens      int             `json:"max_tokens,omitempty"`
		Temperature    *float32        `json:"temperature,omitempty"`
		ResponseFormat *responseFormat `json:"response_format,omitempty"`
		Stream         bool            `json:"stream"`
	}{
		Model:       adapter.model,
		Messages:    request.Messages,
		MaxTokens:   request.MaxTokens,
		Temperature: request.Temperature,
		Stream:      false,
	}
	if nativeJSON {
		payload.ResponseFormat = &responseFormat{Type: "json_object"}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("encode AI request: %w", err)
	}
	slog.Info(
		"AI Provider request",
		"provider", adapter.providerName,
		"model", adapter.model,
		"url", adapter.completionURL,
		"json_mode", nativeJSON,
		"message_count", len(request.Messages),
		"max_tokens", request.MaxTokens,
		"request_body_bytes", len(body),
		"request_body", string(body),
	)
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		adapter.completionURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("create AI request: %w", err)
	}
	httpRequest.Header = adapter.headers.Clone()
	startedAt := time.Now()
	response, err := adapter.client.Do(httpRequest)
	if err != nil {
		slog.Error(
			"AI Provider request failed",
			"provider", adapter.providerName,
			"model", adapter.model,
			"url", adapter.completionURL,
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"error", err,
		)
		return GenerateResponse{}, fmt.Errorf("request AI Provider: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		slog.Warn(
			"AI Provider response",
			"provider", adapter.providerName,
			"model", adapter.model,
			"url", adapter.completionURL,
			"status", response.StatusCode,
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"response_content_type", response.Header.Get("Content-Type"),
		)
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return GenerateResponse{}, &providerHTTPError{status: response.StatusCode}
	}
	var decoded struct {
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxProviderResponseBytes)).Decode(&decoded); err != nil {
		slog.Error(
			"AI Provider response decode failed",
			"provider", adapter.providerName,
			"model", adapter.model,
			"url", adapter.completionURL,
			"status", response.StatusCode,
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"error", err,
		)
		return GenerateResponse{}, fmt.Errorf("decode AI Provider response: %w", err)
	}
	choiceCount := len(decoded.Choices)
	finishReason := ""
	contentPresent := false
	contentBytes := 0
	if choiceCount > 0 {
		finishReason = decoded.Choices[0].FinishReason
		contentPresent = strings.TrimSpace(decoded.Choices[0].Message.Content) != ""
		contentBytes = len(decoded.Choices[0].Message.Content)
	}
	responseAttrs := []any{
		"provider", adapter.providerName,
		"model", adapter.model,
		"url", adapter.completionURL,
		"status", response.StatusCode,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"response_model", decoded.Model,
		"choices", choiceCount,
		"finish_reason", finishReason,
		"content_present", contentPresent,
		"content_bytes", contentBytes,
		"prompt_tokens", decoded.Usage.PromptTokens,
		"completion_tokens", decoded.Usage.CompletionTokens,
	}
	if !contentPresent {
		slog.Warn("AI Provider response has no message content", responseAttrs...)
		return GenerateResponse{}, fmt.Errorf("AI Provider response has no message content")
	}
	slog.Info("AI Provider response", responseAttrs...)
	return GenerateResponse{
		Content:          decoded.Choices[0].Message.Content,
		Model:            decoded.Model,
		PromptTokens:     decoded.Usage.PromptTokens,
		CompletionTokens: decoded.Usage.CompletionTokens,
	}, nil
}

type providerHTTPError struct {
	status int
}

func (err *providerHTTPError) Error() string {
	return fmt.Sprintf("AI Provider returned HTTP %d", err.status)
}

func (err *providerHTTPError) Retryable() bool {
	return err.status == http.StatusRequestTimeout || err.status == http.StatusTooManyRequests || err.status >= 500
}

func shouldFallbackJSONMode(status int) bool {
	switch status {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusMethodNotAllowed,
		http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity, http.StatusNotImplemented:
		return true
	default:
		return false
	}
}

type responseFormat struct {
	Type string `json:"type"`
}
