package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 32 << 20

type Ollama struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllama(baseURL, model string, client *http.Client) (*Ollama, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("embedding base URL must be an absolute HTTP(S) URL")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("embedding model is required")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Ollama{
		baseURL: strings.TrimRight(parsed.String(), "/"),
		model:   model,
		client:  client,
	}, nil
}

func (provider *Ollama) Model() string {
	return provider.model
}

func (provider *Ollama) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}{Model: provider.model, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("encode embedding request: %w", err)
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		provider.baseURL+"/api/embed",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := provider.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request embedding: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("request embedding: Ollama returned status %d", response.StatusCode)
	}
	var result struct {
		Model      string      `json:"model"`
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}
	if len(result.Embeddings) != len(texts) {
		return nil, fmt.Errorf(
			"decode embedding response: got %d vectors for %d inputs",
			len(result.Embeddings),
			len(texts),
		)
	}
	dimensions := 0
	for _, vector := range result.Embeddings {
		if dimensions == 0 {
			dimensions = len(vector)
		}
		if len(vector) == 0 || len(vector) != dimensions {
			return nil, fmt.Errorf("decode embedding response: inconsistent vector dimensions")
		}
	}
	return result.Embeddings, nil
}
