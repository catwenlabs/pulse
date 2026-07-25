package preview

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wenpengfei/pulse/internal/entry"
	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/source"
)

const maxPreviewCandidates = 20

type Candidate struct {
	ExternalID      string     `json:"external_id,omitempty"`
	URL             string     `json:"url,omitempty"`
	Title           string     `json:"title,omitempty"`
	Author          string     `json:"author,omitempty"`
	Summary         string     `json:"summary,omitempty"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
	IdentityKey     string     `json:"identity_key"`
	IdentityWarning string     `json:"identity_warning,omitempty"`
}

type Result struct {
	Candidates  []Candidate           `json:"candidates"`
	Diagnostics ingestion.Diagnostics `json:"diagnostics"`
}

type Service struct {
	drivers *ingestion.Registry
}

func New(drivers *ingestion.Registry) *Service {
	return &Service{drivers: drivers}
}

func (service *Service) Run(ctx context.Context, spec source.Spec) (Result, error) {
	driver, err := service.drivers.Driver(spec.Kind)
	if err != nil {
		return Result{}, err
	}
	validated, err := driver.Validate(ctx, spec)
	if err != nil {
		return Result{}, err
	}
	src := source.Source{
		ID:                "preview",
		Name:              validated.Name,
		Kind:              validated.Kind,
		Locator:           validated.Locator,
		NormalizedLocator: validated.NormalizedLocator,
		Config:            validated.Config,
		SecretRef:         validated.SecretRef,
		Enabled:           true,
	}
	batch, err := driver.Acquire(ctx, ingestion.AcquireRequest{
		Source:  src,
		Trigger: ingestion.TriggerManual,
		Limits: ingestion.Limits{
			MaxBytes:    2 << 20,
			MaxEntries:  maxPreviewCandidates,
			MaxPages:    3,
			MaxDuration: 15 * time.Second,
		},
	})
	if err != nil {
		return Result{}, err
	}
	candidates := batch.Candidates
	if len(candidates) > maxPreviewCandidates {
		candidates = candidates[:maxPreviewCandidates]
	}
	result := Result{
		Candidates:  make([]Candidate, 0, len(candidates)),
		Diagnostics: batch.Diagnostics,
	}
	for index, candidate := range candidates {
		identityKey, err := entry.Identity(candidate)
		if err != nil {
			return Result{}, fmt.Errorf("candidate %d: %w", index+1, err)
		}
		item := Candidate{
			ExternalID:  candidate.ExternalID,
			URL:         candidate.URL,
			Title:       candidate.Title,
			Author:      candidate.Author,
			Summary:     candidate.Summary,
			PublishedAt: candidate.PublishedAt,
			IdentityKey: identityKey,
		}
		if strings.HasPrefix(identityKey, "title:") {
			item.IdentityWarning = "仅使用标题识别；标题变化可能产生重复内容"
		}
		result.Candidates = append(result.Candidates, item)
	}
	return result, nil
}
