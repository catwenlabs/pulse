package jsonapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/source"
)

const defaultMaxBytes int64 = 4 << 20

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Driver struct {
	client HTTPClient
}

type config struct {
	ItemsPath  string            `json:"items_path"`
	Fields     map[string]string `json:"fields"`
	Headers    map[string]string `json:"headers,omitempty"`
	Pagination pagination        `json:"pagination,omitempty"`
}

type pagination struct {
	Mode        string `json:"mode,omitempty"`
	PageParam   string `json:"page_param,omitempty"`
	Start       int    `json:"start,omitempty"`
	NextPath    string `json:"next_path,omitempty"`
	CursorPath  string `json:"cursor_path,omitempty"`
	CursorParam string `json:"cursor_param,omitempty"`
}

func New(client HTTPClient) *Driver {
	return &Driver{client: client}
}

func (driver *Driver) Kind() source.Kind {
	return source.KindJSONAPI
}

func (driver *Driver) Validate(_ context.Context, spec source.Spec) (source.ValidatedSpec, error) {
	if spec.Kind != source.KindJSONAPI {
		return source.ValidatedSpec{}, &source.ValidationError{
			Field: "kind", Message: "JSON API driver requires json-api kind",
		}
	}
	validated, err := spec.Validate()
	if err != nil {
		return source.ValidatedSpec{}, err
	}
	if _, err := decodeConfig(validated.Config); err != nil {
		return source.ValidatedSpec{}, &source.ValidationError{Field: "config", Message: err.Error()}
	}
	return validated, nil
}

func (driver *Driver) Acquire(
	ctx context.Context,
	request ingestion.AcquireRequest,
) (ingestion.AcquisitionBatch, error) {
	cfg, err := decodeConfig(request.Source.Config)
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("decode JSON API config: %w", err)
	}
	if request.Limits.MaxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, request.Limits.MaxDuration)
		defer cancel()
	}
	maxBytes := request.Limits.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	maxEntries := request.Limits.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 1000
	}
	maxPages := request.Limits.MaxPages
	if maxPages <= 0 {
		maxPages = 20
	}

	current, err := url.Parse(request.Source.Locator)
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("parse JSON API URL: %w", err)
	}
	pageNumber := cfg.Pagination.Start
	if pageNumber <= 0 {
		pageNumber = 1
	}
	if cfg.Pagination.Mode == "page" {
		setQuery(current, cfg.Pagination.PageParam, strconv.Itoa(pageNumber))
	}

	var candidates []ingestion.Candidate
	var totalBytes int64
	details := make(map[string]string)
	for pageIndex := 0; pageIndex < maxPages; pageIndex++ {
		document, size, err := driver.fetch(ctx, current, cfg.Headers, maxBytes-totalBytes)
		if err != nil {
			return ingestion.AcquisitionBatch{}, err
		}
		totalBytes += size
		itemsValue, ok := lookup(document, cfg.ItemsPath)
		if !ok {
			return ingestion.AcquisitionBatch{}, fmt.Errorf("map JSON API: items_path %q not found", cfg.ItemsPath)
		}
		items, ok := itemsValue.([]any)
		if !ok {
			return ingestion.AcquisitionBatch{}, fmt.Errorf("map JSON API: items_path %q is not an array", cfg.ItemsPath)
		}
		for _, value := range items {
			item, ok := value.(map[string]any)
			if !ok {
				return ingestion.AcquisitionBatch{}, fmt.Errorf("map JSON API: array item is not an object")
			}
			candidates = append(candidates, mapCandidate(item, cfg.Fields))
			if len(candidates) == maxEntries {
				details["truncated"] = "entries"
				return batch(candidates, details), nil
			}
		}

		next, more, err := nextURL(current, document, len(items), pageNumber, cfg.Pagination)
		if err != nil {
			return ingestion.AcquisitionBatch{}, err
		}
		if !more {
			return batch(candidates, details), nil
		}
		current = next
		pageNumber++
	}
	details["truncated"] = "pages"
	return batch(candidates, details), nil
}

func (driver *Driver) fetch(
	ctx context.Context,
	target *url.URL,
	headers map[string]string,
	remaining int64,
) (map[string]any, int64, error) {
	if remaining <= 0 {
		return nil, 0, fmt.Errorf("read JSON API: response exceeds byte limit")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create JSON API request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := driver.client.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch JSON API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, 0, fmt.Errorf("fetch JSON API: HTTP status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, remaining+1))
	if err != nil {
		return nil, 0, fmt.Errorf("read JSON API: %w", err)
	}
	if int64(len(body)) > remaining {
		return nil, 0, fmt.Errorf("read JSON API: response exceeds byte limit")
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, 0, fmt.Errorf("parse JSON API: %w", err)
	}
	return document, int64(len(body)), nil
}

func decodeConfig(raw json.RawMessage) (config, error) {
	var cfg config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return config{}, err
	}
	cfg.ItemsPath = strings.TrimSpace(cfg.ItemsPath)
	if cfg.ItemsPath == "" {
		return config{}, fmt.Errorf("items_path is required")
	}
	if len(cfg.Fields) == 0 {
		return config{}, fmt.Errorf("fields must include at least one mapping")
	}
	if cfg.Pagination.Mode == "" {
		cfg.Pagination.Mode = "none"
	}
	switch cfg.Pagination.Mode {
	case "none":
	case "page":
		if cfg.Pagination.PageParam == "" {
			cfg.Pagination.PageParam = "page"
		}
	case "next":
		if cfg.Pagination.NextPath == "" {
			return config{}, fmt.Errorf("pagination.next_path is required")
		}
	case "cursor":
		if cfg.Pagination.CursorPath == "" {
			return config{}, fmt.Errorf("pagination.cursor_path is required")
		}
		if cfg.Pagination.CursorParam == "" {
			cfg.Pagination.CursorParam = "cursor"
		}
	default:
		return config{}, fmt.Errorf("pagination.mode %q is not supported", cfg.Pagination.Mode)
	}
	return cfg, nil
}

func lookup(value any, dotPath string) (any, bool) {
	current := value
	for _, part := range strings.Split(dotPath, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func mapCandidate(item map[string]any, fields map[string]string) ingestion.Candidate {
	value := func(field string) string {
		path := fields[field]
		if path == "" {
			return ""
		}
		got, ok := lookup(item, path)
		if !ok || got == nil {
			return ""
		}
		if text, ok := got.(string); ok {
			return strings.TrimSpace(text)
		}
		return fmt.Sprint(got)
	}
	var publishedAt *time.Time
	if raw := value("published_at"); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			publishedAt = &parsed
		}
	}
	return ingestion.Candidate{
		ExternalID:  value("id"),
		URL:         value("url"),
		Title:       value("title"),
		Author:      value("author"),
		Summary:     value("summary"),
		ContentHTML: value("content_html"),
		PublishedAt: publishedAt,
		RawMeta:     item,
	}
}

func nextURL(
	current *url.URL,
	document map[string]any,
	itemCount int,
	pageNumber int,
	cfg pagination,
) (*url.URL, bool, error) {
	switch cfg.Mode {
	case "none":
		return nil, false, nil
	case "page":
		if itemCount == 0 {
			return nil, false, nil
		}
		next := cloneURL(current)
		setQuery(next, cfg.PageParam, strconv.Itoa(pageNumber+1))
		return next, true, nil
	case "next":
		raw, ok := lookup(document, cfg.NextPath)
		if !ok || strings.TrimSpace(fmt.Sprint(raw)) == "" {
			return nil, false, nil
		}
		reference, err := url.Parse(fmt.Sprint(raw))
		if err != nil {
			return nil, false, fmt.Errorf("parse next URL: %w", err)
		}
		next := current.ResolveReference(reference)
		if !strings.EqualFold(next.Scheme, current.Scheme) || !strings.EqualFold(next.Host, current.Host) {
			return nil, false, fmt.Errorf("next URL must keep the original origin")
		}
		return next, true, nil
	case "cursor":
		raw, ok := lookup(document, cfg.CursorPath)
		if !ok || strings.TrimSpace(fmt.Sprint(raw)) == "" {
			return nil, false, nil
		}
		next := cloneURL(current)
		setQuery(next, cfg.CursorParam, fmt.Sprint(raw))
		return next, true, nil
	default:
		return nil, false, fmt.Errorf("unsupported pagination mode %q", cfg.Mode)
	}
}

func setQuery(target *url.URL, name, value string) {
	query := target.Query()
	query.Set(name, value)
	target.RawQuery = query.Encode()
}

func cloneURL(target *url.URL) *url.URL {
	copy := *target
	return &copy
}

func batch(candidates []ingestion.Candidate, details map[string]string) ingestion.AcquisitionBatch {
	return ingestion.AcquisitionBatch{
		Candidates: candidates,
		Diagnostics: ingestion.Diagnostics{
			Status:         "ok",
			CandidateCount: len(candidates),
			Details:        details,
		},
	}
}
