package preview

import (
	"context"
	"testing"

	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/source"
)

type fakeDriver struct {
	batch ingestion.AcquisitionBatch
	err   error
}

func (fake fakeDriver) Kind() source.Kind {
	return source.KindRSS
}

func (fake fakeDriver) Validate(_ context.Context, spec source.Spec) (source.ValidatedSpec, error) {
	return spec.Validate()
}

func (fake fakeDriver) Acquire(context.Context, ingestion.AcquireRequest) (ingestion.AcquisitionBatch, error) {
	return fake.batch, fake.err
}

func TestServicePreviewsCandidatesWithoutPersistence(t *testing.T) {
	registry, err := ingestion.NewRegistry(fakeDriver{batch: ingestion.AcquisitionBatch{
		Candidates: []ingestion.Candidate{
			{ExternalID: "article-1", Title: "First"},
			{Title: "Title fallback"},
		},
		Diagnostics: ingestion.Diagnostics{Status: "ok", CandidateCount: 2},
	}})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	result, err := New(registry).Run(context.Background(), source.Spec{
		Name: "Preview feed", Kind: source.KindRSS, Locator: "https://example.com/feed",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("candidate count = %d", len(result.Candidates))
	}
	if result.Candidates[0].IdentityKey != "external:article-1" {
		t.Errorf("identity = %q", result.Candidates[0].IdentityKey)
	}
	if result.Candidates[1].IdentityKey != "title:Title fallback" || result.Candidates[1].IdentityWarning == "" {
		t.Errorf("fallback preview = %+v", result.Candidates[1])
	}
	if result.Diagnostics.Status != "ok" {
		t.Errorf("diagnostics = %+v", result.Diagnostics)
	}
}

func TestServiceRejectsCandidateWithoutIdentity(t *testing.T) {
	registry, err := ingestion.NewRegistry(fakeDriver{batch: ingestion.AcquisitionBatch{
		Candidates: []ingestion.Candidate{{Author: "anonymous"}},
	}})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	if _, err := New(registry).Run(context.Background(), source.Spec{
		Name: "Preview feed", Kind: source.KindRSS, Locator: "https://example.com/feed",
	}); err == nil {
		t.Fatal("Run() error = nil")
	}
}
