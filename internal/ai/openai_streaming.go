package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// StreamResult is the terminal outcome of a streaming chat completion. It
// carries the model the Provider reports, the finish reason, and token usage.
type StreamResult struct {
	Model            string
	FinishReason     string
	PromptTokens     int
	CompletionTokens int
}

// Stream sends a streaming Chat Completions request and calls emit for each
// content delta as it arrives. It returns the terminal metadata on success. On
// HTTP or parse failure it returns a non-nil error; deltas already emitted
// remain with the caller. When ctx is canceled it stops promptly and returns
// the context error.
//
// The non-streaming Generate path used by Story summaries and Digests is
// unchanged; streaming is a separate execution path for interactive chat.
func (adapter *OpenAICompatibleAdapter) Stream(
	ctx context.Context,
	request GenerateRequest,
	emit func(string) error,
) (StreamResult, error) {
	if len(request.Messages) == 0 {
		return StreamResult{}, fmt.Errorf("AI request requires at least one message")
	}
	payload := struct {
		Model          string          `json:"model"`
		Messages       []Message       `json:"messages"`
		MaxTokens      int             `json:"max_tokens,omitempty"`
		Temperature    *float32        `json:"temperature,omitempty"`
		Thinking       *thinkingConfig `json:"thinking,omitempty"`
		Stream         bool            `json:"stream"`
		StreamOptions  *streamOptions  `json:"stream_options,omitempty"`
	}{
		Model:         adapter.model,
		Messages:      request.Messages,
		MaxTokens:     request.MaxTokens,
		Temperature:   request.Temperature,
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
	}
	if adapter.disableThinking {
		payload.Thinking = &thinkingConfig{Type: "disabled"}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return StreamResult{}, fmt.Errorf("encode AI stream request: %w", err)
	}
	slog.Info(
		"AI Provider stream request",
		"provider", adapter.providerName,
		"model", adapter.model,
		"url", adapter.completionURL,
		"message_count", len(request.Messages),
		"max_tokens", request.MaxTokens,
	)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, adapter.completionURL, bytes.NewReader(body))
	if err != nil {
		return StreamResult{}, fmt.Errorf("create AI stream request: %w", err)
	}
	httpRequest.Header = adapter.headers.Clone()
	startedAt := time.Now()
	response, err := adapter.client.Do(httpRequest)
	if err != nil {
		return StreamResult{}, fmt.Errorf("request AI Provider stream: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return StreamResult{}, &providerHTTPError{status: response.StatusCode}
	}
	return readStream(response.Body, emit, adapter.providerName, adapter.model, startedAt)
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

func readStream(body io.Reader, emit func(string) error, providerName, model string, startedAt time.Time) (StreamResult, error) {
	result := StreamResult{Model: model}
	reader := bufio.NewReaderSize(body, 64*1024)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if processed, terminal := processStreamLine(line, emit, &result); terminal {
				slog.Info("AI Provider stream completed",
					"provider", providerName,
					"model", result.Model,
					"duration_ms", time.Since(startedAt).Milliseconds(),
					"finish_reason", result.FinishReason,
					"prompt_tokens", result.PromptTokens,
					"completion_tokens", result.CompletionTokens,
				)
				return result, nil
			} else if processed {
				// continue reading
			}
		}
		if err != nil {
			if err == io.EOF {
				if result.FinishReason == "" {
					return result, fmt.Errorf("AI Provider stream ended without completion")
				}
				return result, nil
			}
			return result, fmt.Errorf("read AI Provider stream: %w", err)
		}
	}
}

// processStreamLine handles one SSE line. It returns processed=true if the line
// was a recognized data event (including [DONE]), and terminal=true when the
// stream is finished ([DONE] or a terminal finish_reason with usage).
func processStreamLine(line []byte, emit func(string) error, result *StreamResult) (processed bool, terminal bool) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return false, false
	}
	if !strings.HasPrefix(trimmed, "data:") {
		return false, false
	}
	data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if data == "[DONE]" {
		return true, true
	}
	var chunk struct {
		Model   string `json:"model"`
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return true, false
	}
	if chunk.Model != "" {
		result.Model = chunk.Model
	}
	if len(chunk.Choices) > 0 {
		choice := chunk.Choices[0]
		if choice.Delta.Content != "" {
			if emitErr := emit(choice.Delta.Content); emitErr != nil {
				return true, true
			}
		}
		if choice.FinishReason != "" {
			result.FinishReason = choice.FinishReason
		}
	}
	if chunk.Usage != nil {
		result.PromptTokens = chunk.Usage.PromptTokens
		result.CompletionTokens = chunk.Usage.CompletionTokens
	}
	return true, false
}
