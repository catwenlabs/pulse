package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/source"
)

func createTestSource(t *testing.T, store *SourceStore, locator string) source.Source {
	t.Helper()
	created, err := store.Create(context.Background(), source.Spec{
		Name:    locator,
		Kind:    source.KindManual,
		Locator: locator,
	})
	if err != nil {
		t.Fatalf("create test source: %v", err)
	}
	return created
}

func TestAcquisitionStoreEnqueueIsIdempotent(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	src := createTestSource(t, sourceStore, "manual-one")
	ctx := context.Background()
	request := ingestion.EnqueueRequest{
		SourceID:       src.ID,
		Trigger:        ingestion.TriggerManual,
		IdempotencyKey: "save-123",
	}

	first, err := acquisitionStore.Enqueue(ctx, request)
	if err != nil {
		t.Fatalf("first Enqueue() error = %v", err)
	}
	second, err := acquisitionStore.Enqueue(ctx, request)
	if err != nil {
		t.Fatalf("second Enqueue() error = %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("IDs = %q and %q, want same acquisition", first.ID, second.ID)
	}
}

func TestAcquisitionStoreConcurrentClaimHasSingleWinner(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	src := createTestSource(t, sourceStore, "manual-concurrent")
	ctx := context.Background()

	if _, err := acquisitionStore.Enqueue(ctx, ingestion.EnqueueRequest{
		SourceID:       src.ID,
		Trigger:        ingestion.TriggerManual,
		IdempotencyKey: "single-job",
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, owner := range []string{"worker-a", "worker-b"} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := acquisitionStore.Claim(ctx, owner, time.Minute)
			results <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	var wins, empty int
	for err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, ingestion.ErrNoAcquisition):
			empty++
		default:
			t.Fatalf("Claim() unexpected error = %v", err)
		}
	}
	if wins != 1 || empty != 1 {
		t.Errorf("wins = %d, empty = %d; want 1 and 1", wins, empty)
	}
}

func TestAcquisitionStoreDoesNotClaimArchivedSource(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	src := createTestSource(t, sourceStore, "manual-archived")
	ctx := context.Background()

	if _, err := acquisitionStore.Enqueue(ctx, ingestion.EnqueueRequest{
		SourceID:       src.ID,
		Trigger:        ingestion.TriggerManual,
		IdempotencyKey: "archived-job",
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if err := sourceStore.Archive(ctx, src.ID); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}

	if _, err := acquisitionStore.Claim(ctx, "worker-a", time.Minute); !errors.Is(err, ingestion.ErrNoAcquisition) {
		t.Fatalf("Claim() error = %v, want ErrNoAcquisition", err)
	}
}

func TestAcquisitionStoreReclaimsExpiredLease(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	src := createTestSource(t, sourceStore, "manual-expired")
	ctx := context.Background()

	if _, err := acquisitionStore.Enqueue(ctx, ingestion.EnqueueRequest{
		SourceID:       src.ID,
		Trigger:        ingestion.TriggerManual,
		IdempotencyKey: "expired-job",
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	first, err := acquisitionStore.Claim(ctx, "worker-a", -time.Second)
	if err != nil {
		t.Fatalf("first Claim() error = %v", err)
	}
	second, err := acquisitionStore.Claim(ctx, "worker-b", time.Minute)
	if err != nil {
		t.Fatalf("second Claim() error = %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("reclaimed ID = %q, want %q", second.ID, first.ID)
	}
	if second.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", second.Attempts)
	}
}

func TestAcquisitionStoreRetryAndComplete(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	store := NewAcquisitionStore(pool)
	src := createTestSource(t, sourceStore, "manual-state")
	ctx := context.Background()

	if _, err := store.Enqueue(ctx, ingestion.EnqueueRequest{
		SourceID: src.ID, Trigger: ingestion.TriggerManual, IdempotencyKey: "state-job",
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	claimed, err := store.Claim(ctx, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if err := store.Retry(ctx, claimed.ID, "worker-a", time.Now().Add(-time.Second), errors.New("temporary")); err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	reclaimed, err := store.Claim(ctx, "worker-b", time.Minute)
	if err != nil {
		t.Fatalf("reclaim error = %v", err)
	}
	if err := store.Complete(ctx, reclaimed.ID, "worker-b"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if err := store.Complete(ctx, reclaimed.ID, "wrong-owner"); err == nil {
		t.Fatal("Complete() wrong owner error = nil")
	}
}
