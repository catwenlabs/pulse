package push

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/source"
)

const defaultMaxBytes int64 = 1 << 20

type Driver struct {
	kind source.Kind
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
	_ context.Context,
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
	candidate := ingestion.Candidate{
		ExternalID: value.ID, URL: value.URL, Title: value.Title,
		Author: value.Author, Summary: value.Summary, ContentHTML: value.ContentHTML,
		PublishedAt: publishedAt, RawMeta: value.RawMeta,
	}
	return ingestion.AcquisitionBatch{
		Candidates: []ingestion.Candidate{candidate},
		Diagnostics: ingestion.Diagnostics{
			Status: "ok", CandidateCount: 1,
		},
	}, nil
}
