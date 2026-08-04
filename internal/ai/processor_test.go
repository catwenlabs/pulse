package ai

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type processingProvider struct {
	content string
	err     error
	request GenerateRequest
}

func (provider *processingProvider) Metadata() ProviderMetadata {
	return ProviderMetadata{Name: "fake", Model: "fake-model"}
}

func (provider *processingProvider) Generate(_ context.Context, request GenerateRequest) (GenerateResponse, error) {
	provider.request = request
	if provider.err != nil {
		return GenerateResponse{}, provider.err
	}
	return GenerateResponse{Content: provider.content}, nil
}

type processingStore struct {
	fakeStore
	job       Job
	lease     time.Duration
	retries   int
	failures  int
	completed GeneratedStorySummary
}

func (store *processingStore) Claim(_ context.Context, _ string, lease time.Duration) (Job, error) {
	store.lease = lease
	return store.job, nil
}

func (store *processingStore) CompleteStorySummary(_ context.Context, _ Job, _ string, result GeneratedStorySummary, _ ProviderMetadata) error {
	store.completed = result
	return nil
}

func (store *processingStore) Retry(context.Context, Job, string, time.Time, error) error {
	store.retries++
	return nil
}

func (store *processingStore) Fail(context.Context, Job, string, error) error {
	store.failures++
	return nil
}

func TestProcessorCompletesStorySummaryThroughTheStoreSeam(t *testing.T) {
	snapshot := StorySnapshot{
		StoryID: "story-1",
		Title:   "Story title",
		Entries: []StoryEntrySnapshot{{Label: "E1", EntryID: "entry-1", SourceTitle: "Source", Title: "Entry title", Content: "body"}},
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	provider := &processingProvider{content: `{"overview":"overview","key_points":["point"],"source_notes":[]}`}
	store := &processingStore{job: Job{ID: "job-1", Kind: JobKindStorySummary, TargetID: "story-1", Payload: payload, Attempts: 1, LeaseOwner: "worker"}}
	if err := NewProcessor(store, provider).ProcessNext(context.Background(), "worker"); err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	if store.completed.Overview != "overview" || store.retries != 0 || store.failures != 0 || store.lease != defaultAILease {
		t.Errorf("store = %+v", store)
	}
	if len(provider.request.Messages) != 2 || provider.request.Messages[1].Content == "" {
		t.Errorf("provider request = %+v", provider.request)
	}
}

func TestProcessorRetriesMalformedProviderOutput(t *testing.T) {
	snapshot := StorySnapshot{StoryID: "story-1", Title: "Story title", Entries: []StoryEntrySnapshot{{Label: "E1"}}}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	store := &processingStore{job: Job{ID: "job-1", Kind: JobKindStorySummary, TargetID: "story-1", Payload: payload, Attempts: 1, LeaseOwner: "worker"}}
	provider := &processingProvider{content: `{"overview":"ok","source_notes":[{"label":"E9","note":"bad"}]}`}
	if err := NewProcessor(store, provider).ProcessNext(context.Background(), "worker"); err == nil {
		t.Fatal("ProcessNext() error = nil")
	}
	if store.retries != 1 || store.failures != 0 {
		t.Errorf("retries/failures = %d/%d", store.retries, store.failures)
	}
}

func TestProcessorDoesNotRetryNonRetryableProviderErrors(t *testing.T) {
	store := &processingStore{job: Job{
		ID: "job-1", Kind: JobKindStorySummary, Attempts: 1, LeaseOwner: "worker",
		Payload: json.RawMessage(`{"story_id":"story-1","entries":[{"label":"E1"}]}`),
	}}
	provider := &processingProvider{err: &providerHTTPError{status: 401}}
	if err := NewProcessor(store, provider).ProcessNext(context.Background(), "worker"); err == nil {
		t.Fatal("ProcessNext() error = nil")
	}
	if store.retries != 0 || store.failures != 1 {
		t.Errorf("retries/failures = %d/%d", store.retries, store.failures)
	}
}
