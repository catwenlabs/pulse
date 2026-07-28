package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/microcosm-cc/bluemonday"
	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/source"
)

const defaultMaxBytes int64 = 1 << 20

type Driver struct {
	kind   source.Kind
	client HTTPClient
}

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type payload struct {
	ID          string         `json:"id"`
	URL         string         `json:"url"`
	Title       string         `json:"title"`
	Author      string         `json:"author"`
	Summary     string         `json:"summary"`
	ContentHTML string         `json:"content_html"`
	PublishedAt string         `json:"published_at"`
	RawMeta     map[string]any `json:"raw_meta"`
}

func New(kind source.Kind) *Driver {
	return &Driver{kind: kind}
}

func NewManual(client HTTPClient) *Driver {
	return &Driver{kind: source.KindManual, client: client}
}

func (driver *Driver) Kind() source.Kind {
	return driver.kind
}

func (driver *Driver) Validate(_ context.Context, spec source.Spec) (source.ValidatedSpec, error) {
	if (driver.kind != source.KindWebhook && driver.kind != source.KindManual) || spec.Kind != driver.kind {
		return source.ValidatedSpec{}, &source.ValidationError{
			Field: "kind", Message: "push driver kind mismatch",
		}
	}
	return spec.Validate()
}

func (driver *Driver) Acquire(
	ctx context.Context,
	request ingestion.AcquireRequest,
) (ingestion.AcquisitionBatch, error) {
	if request.Source.Kind != driver.kind {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("push driver kind mismatch")
	}
	maxBytes := request.Limits.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	if request.Payload == nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("decode pushed candidate: payload is required")
	}
	body, err := io.ReadAll(io.LimitReader(request.Payload, maxBytes+1))
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("read pushed candidate: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("read pushed candidate: payload exceeds %d bytes", maxBytes)
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	var value payload
	if err := decoder.Decode(&value); err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("decode pushed candidate: %w", err)
	}
	var publishedAt *time.Time
	if value.PublishedAt != "" {
		parsed, err := time.Parse(time.RFC3339, value.PublishedAt)
		if err != nil {
			return ingestion.AcquisitionBatch{}, fmt.Errorf("decode pushed candidate published_at: %w", err)
		}
		publishedAt = &parsed
	}
	details := map[string]string{}
	if driver.kind == source.KindManual && driver.client != nil &&
		value.ContentHTML == "" && value.URL != "" {
		snapshot, reason := driver.snapshot(ctx, value.URL, maxBytes, request.Limits.MaxDuration)
		if reason != "" {
			details["snapshot"] = "failed"
			details["snapshot_error"] = reason
		} else {
			details["snapshot"] = "saved"
			value.ContentHTML = snapshot.contentHTML
			if value.Title == "" {
				value.Title = snapshot.title
			}
			if value.Author == "" {
				value.Author = snapshot.author
			}
			if value.Summary == "" {
				value.Summary = snapshot.summary
			}
			if publishedAt == nil {
				publishedAt = snapshot.publishedAt
			}
		}
	}
	candidate := ingestion.Candidate{
		ExternalID: value.ID, URL: value.URL, Title: value.Title,
		Author: value.Author, Summary: value.Summary, ContentHTML: value.ContentHTML,
		PublishedAt: publishedAt, RawMeta: value.RawMeta,
	}
	return ingestion.AcquisitionBatch{
		Candidates: []ingestion.Candidate{candidate},
		Diagnostics: ingestion.Diagnostics{
			Status: "ok", CandidateCount: 1, Details: details,
		},
	}, nil
}

type articleSnapshot struct {
	title       string
	author      string
	summary     string
	contentHTML string
	publishedAt *time.Time
}

func (driver *Driver) snapshot(
	ctx context.Context,
	rawURL string,
	maxBytes int64,
	maxDuration time.Duration,
) (articleSnapshot, string) {
	pageURL, err := url.Parse(rawURL)
	if err != nil || (pageURL.Scheme != "http" && pageURL.Scheme != "https") ||
		pageURL.Host == "" || pageURL.User != nil {
		return articleSnapshot{}, "URL is not a public HTTP or HTTPS page"
	}
	if maxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, maxDuration)
		defer cancel()
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL.String(), nil)
	if err != nil {
		return articleSnapshot{}, "request could not be created"
	}
	httpRequest.Header.Set("Accept", "text/html, application/xhtml+xml;q=0.9")
	httpRequest.Header.Set("User-Agent", "Pulse/1.0 (+read-later snapshot)")
	response, err := driver.client.Do(httpRequest)
	if err != nil {
		return articleSnapshot{}, "page could not be fetched"
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return articleSnapshot{}, fmt.Sprintf("page returned HTTP %d", response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || (mediaType != "text/html" && mediaType != "application/xhtml+xml") {
		return articleSnapshot{}, "response is not HTML"
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return articleSnapshot{}, "page body could not be read"
	}
	if int64(len(body)) > maxBytes {
		return articleSnapshot{}, fmt.Sprintf("page exceeds %d bytes", maxBytes)
	}
	finalURL := pageURL
	if response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL
	}
	article, err := readability.FromReader(bytes.NewReader(body), finalURL)
	if err != nil || article.Node == nil {
		return articleSnapshot{}, "readable article content was not found"
	}
	var rendered strings.Builder
	if err := article.RenderHTML(&rendered); err != nil {
		return articleSnapshot{}, "article HTML could not be rendered"
	}
	contentHTML := bluemonday.UGCPolicy().Sanitize(rendered.String())
	if strings.TrimSpace(contentHTML) == "" {
		return articleSnapshot{}, "readable article content was empty"
	}
	var publishedAt *time.Time
	if parsed, err := article.PublishedTime(); err == nil {
		publishedAt = &parsed
	}
	return articleSnapshot{
		title:       strings.TrimSpace(article.Title()),
		author:      strings.TrimSpace(article.Byline()),
		summary:     strings.TrimSpace(article.Excerpt()),
		contentHTML: contentHTML,
		publishedAt: publishedAt,
	}, ""
}
