package annotations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"strings"
	"time"

	"github.com/wenpengfei/pulse/internal/annotation"
	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/source"
)

const (
	defaultMaxBytes int64 = 1 << 20
)

type Driver struct{}

func New() *Driver {
	return &Driver{}
}

func (*Driver) Kind() source.Kind {
	return source.KindAnnotations
}

func (*Driver) Validate(_ context.Context, spec source.Spec) (source.ValidatedSpec, error) {
	if spec.Kind != source.KindAnnotations {
		return source.ValidatedSpec{}, &source.ValidationError{
			Field: "kind", Message: "annotation driver requires annotations kind",
		}
	}
	return spec.Validate()
}

func (*Driver) Acquire(_ context.Context, request ingestion.AcquireRequest) (ingestion.AcquisitionBatch, error) {
	if request.Source.Kind != source.KindAnnotations {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("annotation driver kind mismatch")
	}
	if request.Payload == nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("decode annotations: payload is required")
	}
	maxBytes := request.Limits.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	body, err := io.ReadAll(io.LimitReader(request.Payload, maxBytes+1))
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("read annotations: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("read annotations: payload exceeds %d bytes", maxBytes)
	}
	value, err := annotation.DecodeBatch(body)
	if err != nil {
		return ingestion.AcquisitionBatch{}, err
	}

	candidates := make([]ingestion.Candidate, 0, len(value.Annotations))
	for index, item := range value.Annotations {
		candidate, err := candidateFromPayload(item)
		if err != nil {
			return ingestion.AcquisitionBatch{}, fmt.Errorf("annotations[%d].%w", index, err)
		}
		candidates = append(candidates, candidate)
	}
	return ingestion.AcquisitionBatch{
		Candidates: candidates,
		Diagnostics: ingestion.Diagnostics{
			Status: "ok", CandidateCount: len(candidates),
		},
	}, nil
}

func candidateFromPayload(value annotation.Input) (ingestion.Candidate, error) {
	if value.BookIdentity == "" {
		value.BookIdentity = fingerprint(strings.ToLower(value.BookTitle) + "\x00" + strings.ToLower(value.BookAuthor))
	}
	var highlightedAt *time.Time
	if strings.TrimSpace(value.HighlightedAt) != "" {
		parsed, err := time.Parse(time.RFC3339, value.HighlightedAt)
		if err != nil {
			return ingestion.Candidate{}, fmt.Errorf("highlighted_at: must be RFC3339: %w", err)
		}
		highlightedAt = &parsed
	}
	externalID := strings.TrimSpace(value.ID)
	if externalID == "" {
		externalID = annotationIdentity(value)
	}
	content := "<blockquote>" + html.EscapeString(value.Highlight) + "</blockquote>"
	if value.Note != "" {
		content += "<p>" + html.EscapeString(value.Note) + "</p>"
	}
	return ingestion.Candidate{
		ExternalID:  externalID,
		Title:       value.BookTitle,
		Author:      value.BookAuthor,
		Summary:     value.Highlight,
		ContentHTML: content,
		PublishedAt: highlightedAt,
		Annotation: &annotation.Detail{
			Provider:       value.Provider,
			BookIdentity:   value.BookIdentity,
			BookTitle:      value.BookTitle,
			BookAuthor:     value.BookAuthor,
			Chapter:        value.Chapter,
			Location:       value.Location,
			HighlightColor: value.HighlightColor,
			AnnotationNote: value.Note,
			HighlightedAt:  highlightedAt,
		},
	}, nil
}

func annotationIdentity(value annotation.Input) string {
	bookIdentity := value.BookIdentity
	if value.Location != "" {
		return value.Provider + ":" + bookIdentity + ":" + value.Location
	}
	return value.Provider + ":" + bookIdentity + ":" + fingerprint(value.Highlight)
}

func fingerprint(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(digest[:16])
}
