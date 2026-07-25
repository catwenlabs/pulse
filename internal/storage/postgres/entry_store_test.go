package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/wenpengfei/pulse/internal/entry"
	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/source"
)

func TestEntryStoreCommitBatchIsAtomicAndUpdatesExistingEntry(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	ctx := context.Background()
	src := createTestSource(t, sourceStore, "pipeline-source")

	first := claimTestAcquisition(t, acquisitionStore, src.ID, "first")
	if err := entryStore.CommitBatch(ctx, first, "worker", []ingestion.Candidate{{
		ExternalID: "post-1",
		Title:      "First title",
	}}, json.RawMessage(`{"cursor":"one"}`)); err != nil {
		t.Fatalf("first CommitBatch() error = %v", err)
	}

	second := claimTestAcquisition(t, acquisitionStore, src.ID, "second")
	if err := entryStore.CommitBatch(ctx, second, "worker", []ingestion.Candidate{{
		ExternalID: "post-1",
		Title:      "Updated title",
	}}, json.RawMessage(`{"cursor":"two"}`)); err != nil {
		t.Fatalf("second CommitBatch() error = %v", err)
	}

	var count int
	var title string
	if err := pool.QueryRow(ctx, "SELECT count(*), max(source_title) FROM entries").Scan(&count, &title); err != nil {
		t.Fatalf("query entries: %v", err)
	}
	if count != 1 || title != "Updated title" {
		t.Errorf("entry count = %d, title = %q", count, title)
	}

	var checkpoint string
	if err := pool.QueryRow(ctx,
		"SELECT checkpoint::text FROM source_checkpoints WHERE source_id = $1",
		src.ID,
	).Scan(&checkpoint); err != nil {
		t.Fatalf("query checkpoint: %v", err)
	}
	if checkpoint != `{"cursor": "two"}` {
		t.Errorf("checkpoint = %s", checkpoint)
	}
	loadedCheckpoint, err := sourceStore.Checkpoint(ctx, src.ID)
	if err != nil {
		t.Fatalf("Checkpoint() error = %v", err)
	}
	if string(loadedCheckpoint) != `{"cursor": "two"}` {
		t.Errorf("loaded checkpoint = %s", loadedCheckpoint)
	}

	entries, err := entryStore.List(ctx, 10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 || entries[0].SourceTitle != "Updated title" {
		t.Errorf("List() = %+v", entries)
	}
}

func TestEntryStoreRollbackDoesNotWriteEntryOrCheckpoint(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	ctx := context.Background()
	src := createTestSource(t, sourceStore, "rollback-source")
	acquisition := claimTestAcquisition(t, acquisitionStore, src.ID, "rollback")

	err := entryStore.CommitBatch(ctx, acquisition, "worker", []ingestion.Candidate{{
		ExternalID: "post-rollback",
		Title:      "Must roll back",
	}}, json.RawMessage(`not-json`))
	if err == nil {
		t.Fatal("CommitBatch() error = nil, want invalid checkpoint error")
	}

	var entryCount, checkpointCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM entries").Scan(&entryCount); err != nil {
		t.Fatalf("count entries: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM source_checkpoints").Scan(&checkpointCount); err != nil {
		t.Fatalf("count checkpoints: %v", err)
	}
	if entryCount != 0 || checkpointCount != 0 {
		t.Errorf("entry count = %d, checkpoint count = %d; want 0, 0", entryCount, checkpointCount)
	}
}

func TestEntryStoreSearchStatePatchAndTags(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	store := NewEntryStore(pool)
	ctx := context.Background()
	src := createTestSource(t, sourceStore, "reader-source")
	acquisition := claimTestAcquisition(t, acquisitionStore, src.ID, "reader")
	if err := store.CommitBatch(ctx, acquisition, "worker", []ingestion.Candidate{
		{ExternalID: "one", Title: "Go concurrency", Author: "Ada"},
		{ExternalID: "two", Title: "Gardening", Author: "Lin"},
	}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}
	results, err := store.Search(ctx, entry.Query{Limit: 10, Search: "concurrency"})
	if err != nil || len(results) != 1 || results[0].ExternalID != "one" {
		t.Fatalf("Search() = %+v, %v", results, err)
	}
	id := results[0].ID
	displayTitle := "My Go note"
	note := "Read again"
	yes := true
	updated, err := store.Update(ctx, id, entry.Patch{
		Read: &yes, Starred: &yes, Later: &yes, DisplayTitle: &displayTitle, Note: &note,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.ReadAt == nil || updated.StarredAt == nil || updated.LaterAt == nil ||
		updated.DisplayTitle != displayTitle || updated.Note != note {
		t.Errorf("updated = %+v", updated)
	}
	starred, err := store.Search(ctx, entry.Query{Limit: 10, State: "starred"})
	if err != nil || len(starred) != 1 {
		t.Fatalf("starred Search() = %+v, %v", starred, err)
	}
	tag, err := store.AddTag(ctx, id, " Go ")
	if err != nil || tag.Name != "Go" {
		t.Fatalf("AddTag() = %+v, %v", tag, err)
	}
	tagged, err := store.Search(ctx, entry.Query{Limit: 10, Tag: "go"})
	if err != nil || len(tagged) != 1 {
		t.Fatalf("tag Search() = %+v, %v", tagged, err)
	}
	if err := store.RemoveTag(ctx, id, tag.ID); err != nil {
		t.Fatalf("RemoveTag() error = %v", err)
	}
}

func claimTestAcquisition(
	t *testing.T,
	store *AcquisitionStore,
	sourceID source.ID,
	key string,
) ingestion.Acquisition {
	t.Helper()
	ctx := context.Background()
	if _, err := store.Enqueue(ctx, ingestion.EnqueueRequest{
		SourceID:       sourceID,
		Trigger:        ingestion.TriggerManual,
		IdempotencyKey: key,
	}); err != nil {
		t.Fatalf("enqueue test acquisition: %v", err)
	}
	acquisition, err := store.Claim(ctx, "worker", time.Minute)
	if err != nil {
		t.Fatalf("claim test acquisition: %v", err)
	}
	return acquisition
}
