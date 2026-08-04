package ai

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeProvider struct{}

func (fakeProvider) Metadata() ProviderMetadata {
	return ProviderMetadata{Name: "fake", Model: "fake-model"}
}

func (fakeProvider) Generate(context.Context, GenerateRequest) (GenerateResponse, error) {
	return GenerateResponse{}, nil
}

type fakeStore struct {
	storySnapshot StorySnapshot
	digestItems   []DigestStorySnapshot
	storySummary  StorySummary
	storyJob      JobReceipt
	digestJob     JobReceipt
	digest        Digest
	list          []Digest
}

func (store *fakeStore) SnapshotStory(context.Context, string) (StorySnapshot, error) {
	return store.storySnapshot, nil
}
func (store *fakeStore) SnapshotUnreadStories(_ context.Context, scope DigestScope) ([]DigestStorySnapshot, error) {
	items := store.digestItems
	if scope.MaxStories > 0 && len(items) > scope.MaxStories {
		items = items[:scope.MaxStories]
	}
	return items, nil
}
func (store *fakeStore) GetStorySummary(context.Context, string) (StorySummary, error) {
	if store.storySummary.Status == "" {
		return StorySummary{}, ErrNotFound
	}
	return store.storySummary, nil
}
func (store *fakeStore) EnqueueStorySummary(context.Context, StorySnapshot, ProviderMetadata) (StorySummary, JobReceipt, error) {
	return store.storySummary, store.storyJob, nil
}
func (store *fakeStore) GetDigest(context.Context, string) (Digest, error) {
	return store.digest, nil
}
func (store *fakeStore) ListDigests(context.Context, int) ([]Digest, error) {
	return store.list, nil
}
func (store *fakeStore) EnqueueDigest(context.Context, DigestScope, []DigestStorySnapshot, string, ProviderMetadata) (Digest, JobReceipt, error) {
	return store.digest, store.digestJob, nil
}
func (*fakeStore) Claim(context.Context, string, time.Duration) (Job, error) { return Job{}, ErrNoJob }
func (*fakeStore) CompleteStorySummary(context.Context, Job, string, GeneratedStorySummary, ProviderMetadata) error {
	return nil
}
func (*fakeStore) CompleteDigest(context.Context, Job, string, GeneratedDigest, ProviderMetadata) error {
	return nil
}
func (*fakeStore) Retry(context.Context, Job, string, time.Time, error) error { return nil }
func (*fakeStore) Fail(context.Context, Job, string, error) error             { return nil }

func TestServiceRequestsStorySummaryThroughTheConfiguredProvider(t *testing.T) {
	store := &fakeStore{
		storySnapshot: StorySnapshot{StoryID: "story-1", InputFingerprint: "fingerprint"},
		storyJob:      JobReceipt{ID: "job-1", Kind: JobKindStorySummary, TargetID: "story-1", Status: JobPending},
	}
	receipt, err := NewService(store, fakeProvider{}, ServiceOptions{}).RequestStorySummary(context.Background(), "story-1")
	if err != nil {
		t.Fatalf("RequestStorySummary() error = %v", err)
	}
	if receipt.ID != "job-1" || receipt.Kind != JobKindStorySummary {
		t.Errorf("receipt = %+v", receipt)
	}
}

func TestServiceRejectsOversizedDefaultDigestScope(t *testing.T) {
	items := make([]DigestStorySnapshot, 3)
	store := &fakeStore{digestItems: items}
	_, err := NewService(store, fakeProvider{}, ServiceOptions{MaxDigestStories: 2}).RequestDigest(context.Background(), DigestScope{})
	var limitErr *ScopeLimitError
	if !errors.As(err, &limitErr) || limitErr.Limit != 2 {
		t.Fatalf("RequestDigest() error = %v, want ScopeLimitError", err)
	}
}

func TestServicePreviewsDigestScopeWithoutQueueing(t *testing.T) {
	store := &fakeStore{digestItems: make([]DigestStorySnapshot, 3)}
	preview, err := NewService(store, fakeProvider{}, ServiceOptions{MaxDigestStories: 2}).PreviewDigest(context.Background(), DigestScope{})
	if err != nil {
		t.Fatalf("PreviewDigest() error = %v", err)
	}
	if preview.MatchingStories != 3 || !preview.MatchingStoriesTruncated || preview.SelectedStories != 0 || preview.CanQueue {
		t.Fatalf("preview = %+v", preview)
	}
}

func TestServiceReturnsTypedDigestScopeValidationErrors(t *testing.T) {
	start := time.Date(2026, time.August, 2, 0, 0, 0, 0, time.UTC)
	end := start.Add(-time.Hour)
	_, err := NewService(&fakeStore{}, fakeProvider{}, ServiceOptions{}).RequestDigest(context.Background(), DigestScope{
		StartAt: &start,
		EndAt:   &end,
	})
	var validationErr *ScopeValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "end_at" {
		t.Fatalf("RequestDigest() error = %v, want end_at validation", err)
	}
}

func TestServiceDoesNotTreatDigestAsReadOperation(t *testing.T) {
	store := &fakeStore{
		digestItems: []DigestStorySnapshot{{Label: "S1", StoryID: "story-1", Title: "title"}},
		digestJob:   JobReceipt{ID: "job-1", Kind: JobKindDigest, TargetID: "digest-1", Status: JobPending},
	}
	if _, err := NewService(store, fakeProvider{}, ServiceOptions{}).RequestDigest(context.Background(), DigestScope{MaxStories: 1}); err != nil {
		t.Fatalf("RequestDigest() error = %v", err)
	}
	if store.digestJob.TargetID != "digest-1" {
		t.Errorf("digest job = %+v", store.digestJob)
	}
}
