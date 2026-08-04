package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type ServiceOptions struct {
	MaxDigestStories int
}

type Service struct {
	store            Store
	provider         Provider
	maxDigestStories int
}

func NewService(store Store, provider Provider, options ServiceOptions) *Service {
	maxStories := options.MaxDigestStories
	if maxStories <= 0 {
		maxStories = 100
	}
	return &Service{
		store:            store,
		provider:         provider,
		maxDigestStories: maxStories,
	}
}

func (service *Service) RequestStorySummary(ctx context.Context, storyID string) (JobReceipt, error) {
	if service.provider == nil {
		return JobReceipt{}, ErrUnavailable
	}
	if storyID == "" {
		return JobReceipt{}, fmt.Errorf("story ID is required")
	}
	snapshot, err := service.store.SnapshotStory(ctx, storyID)
	if err != nil {
		return JobReceipt{}, err
	}
	_, receipt, err := service.store.EnqueueStorySummary(
		ctx,
		snapshot,
		service.provider.Metadata(),
	)
	if err != nil {
		return JobReceipt{}, err
	}
	return receipt, nil
}

func (service *Service) GetStorySummary(ctx context.Context, storyID string) (StorySummary, error) {
	if storyID == "" {
		return StorySummary{}, fmt.Errorf("story ID is required")
	}
	item, err := service.store.GetStorySummary(ctx, storyID)
	if errors.Is(err, ErrNotFound) {
		return StorySummary{StoryID: storyID, Status: StatusNotRequested}, nil
	}
	if err != nil {
		return StorySummary{}, err
	}
	if item.Status == StatusCompleted || item.Status == StatusPartial {
		current, snapshotErr := service.store.SnapshotStory(ctx, storyID)
		if snapshotErr == nil && current.InputFingerprint != "" &&
			current.InputFingerprint != item.InputFingerprint {
			item.Status = StatusStale
		}
	}
	return item, nil
}

func (service *Service) RequestDigest(ctx context.Context, scope DigestScope) (JobReceipt, error) {
	if service.provider == nil {
		return JobReceipt{}, ErrUnavailable
	}
	if err := validateDigestScope(scope, service.maxDigestStories); err != nil {
		return JobReceipt{}, err
	}

	queryScope := scope
	if queryScope.MaxStories == 0 {
		// Ask for one extra row so the Store can tell us whether the default
		// catch-up scope needs to be narrowed instead of silently dropping Stories.
		queryScope.MaxStories = service.maxDigestStories + 1
	}
	items, err := service.store.SnapshotUnreadStories(ctx, queryScope)
	if err != nil {
		return JobReceipt{}, err
	}
	if len(items) == 0 {
		return JobReceipt{}, ErrNoStories
	}
	if scope.MaxStories == 0 && len(items) > service.maxDigestStories {
		return JobReceipt{}, &ScopeLimitError{Count: len(items), Limit: service.maxDigestStories}
	}
	if scope.MaxStories > 0 && len(items) > scope.MaxStories {
		items = items[:scope.MaxStories]
	}
	fingerprint, err := fingerprint(struct {
		Scope DigestScope           `json:"scope"`
		Items []DigestStorySnapshot `json:"items"`
	}{Scope: scope, Items: items})
	if err != nil {
		return JobReceipt{}, fmt.Errorf("fingerprint Digest input: %w", err)
	}
	_, receipt, err := service.store.EnqueueDigest(
		ctx,
		scope,
		items,
		fingerprint,
		service.provider.Metadata(),
	)
	if err != nil {
		return JobReceipt{}, err
	}
	return receipt, nil
}

func (service *Service) PreviewDigest(ctx context.Context, scope DigestScope) (DigestPreview, error) {
	if err := validateDigestScope(scope, service.maxDigestStories); err != nil {
		return DigestPreview{}, err
	}
	queryScope := scope
	queryScope.MaxStories = service.maxDigestStories + 1
	items, err := service.store.SnapshotUnreadStories(ctx, queryScope)
	if err != nil {
		return DigestPreview{}, err
	}
	truncated := len(items) > service.maxDigestStories
	selected := len(items)
	if scope.MaxStories == 0 && truncated {
		selected = 0
	}
	if scope.MaxStories > 0 && selected > scope.MaxStories {
		selected = scope.MaxStories
	}
	return DigestPreview{
		Scope:                    scope,
		MatchingStories:          len(items),
		MatchingStoriesTruncated: truncated,
		SelectedStories:          selected,
		SafetyLimit:              service.maxDigestStories,
		CanQueue:                 len(items) > 0 && (scope.MaxStories > 0 || !truncated),
	}, nil
}

func (service *Service) ListDigests(ctx context.Context, limit int) ([]Digest, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return service.store.ListDigests(ctx, limit)
}

func (service *Service) GetDigest(ctx context.Context, id string) (Digest, error) {
	if id == "" {
		return Digest{}, fmt.Errorf("Digest ID is required")
	}
	return service.store.GetDigest(ctx, id)
}

func fingerprint(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func retryDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 6 {
		attempts = 6
	}
	return time.Duration(1<<(attempts-1)) * time.Second
}

func validateDigestScope(scope DigestScope, limit int) error {
	if scope.StartAt != nil && scope.EndAt != nil && scope.StartAt.After(*scope.EndAt) {
		return &ScopeValidationError{Field: "end_at", Message: "Digest start_at must not be after end_at"}
	}
	if scope.MaxStories < 0 {
		return &ScopeValidationError{Field: "max_stories", Message: "Digest max_stories must not be negative"}
	}
	if scope.MaxStories > limit {
		return &ScopeLimitError{Count: scope.MaxStories, Limit: limit}
	}
	return nil
}
