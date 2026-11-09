package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/catwenlabs/pulse/internal/entry"
	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/pagination"
	"github.com/catwenlabs/pulse/internal/source"
	"github.com/catwenlabs/pulse/internal/story"
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

	var storyCount, membershipCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM stories").Scan(&storyCount); err != nil {
		t.Fatalf("count stories: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM story_entries").Scan(&membershipCount); err != nil {
		t.Fatalf("count story entries: %v", err)
	}
	if storyCount != 1 || membershipCount != 1 {
		t.Errorf("story count = %d, membership count = %d", storyCount, membershipCount)
	}
}

func TestEntryStoreFinalDeletionRequiresConfirmationAndWritesTombstone(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	storyStore := NewStoryStore(pool)
	ctx := context.Background()
	src := createTestSource(t, sourceStore, "final-entry-delete")
	acquisition := claimTestAcquisition(t, acquisitionStore, src.ID, "final-entry-delete")
	if err := entryStore.CommitBatch(ctx, acquisition, "worker", []ingestion.Candidate{{
		ExternalID: "final", Title: "Final Entry", ContentHTML: "<p>body</p>",
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}
	items, err := storyStore.Search(ctx, story.Query{Limit: 10})
	if err != nil || len(items) != 1 {
		t.Fatalf("Search() = %+v, %v", items, err)
	}
	displayTitle, note := "A Story title", "A Story note"
	if _, err := storyStore.Update(ctx, items[0].ID, story.Patch{DisplayTitle: &displayTitle, Note: &note}); err != nil {
		t.Fatalf("set Story metadata: %v", err)
	}
	entryID := items[0].Representative.ID
	err = entryStore.Delete(ctx, entryID, false)
	var confirmation *entry.DeletionConfirmationError
	if !errors.As(err, &confirmation) || confirmation.StoryID != string(items[0].ID) ||
		confirmation.DisplayTitle != displayTitle || confirmation.Note != note {
		t.Fatalf("Delete() error = %v, confirmation = %+v", err, confirmation)
	}
	if err := entryStore.Delete(ctx, entryID, true); err != nil {
		t.Fatalf("confirmed Delete() error = %v", err)
	}
	var entries, stories, tombstones int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM entries").Scan(&entries); err != nil {
		t.Fatalf("count entries: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM stories").Scan(&stories); err != nil {
		t.Fatalf("count stories: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM entry_tombstones WHERE source_id = $1", src.ID).Scan(&tombstones); err != nil {
		t.Fatalf("count tombstones: %v", err)
	}
	if entries != 0 || stories != 0 || tombstones != 1 {
		t.Fatalf("after deletion entries=%d stories=%d tombstones=%d", entries, stories, tombstones)
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
	storyStore := NewStoryStore(pool)
	ctx := context.Background()
	src := createTestSource(t, sourceStore, "reader-source")
	acquisition := claimTestAcquisition(t, acquisitionStore, src.ID, "reader")
	if err := store.CommitBatch(ctx, acquisition, "worker", []ingestion.Candidate{
		{ExternalID: "one", Title: "Go concurrency", Author: "Ada"},
		{ExternalID: "two", Title: "Gardening", Author: "Lin"},
		{ExternalID: "three", Title: "深入理解并发编程", Author: "王明"},
	}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}
	results, err := store.Search(ctx, entry.Query{Limit: 10, Search: "concurrency"})
	if err != nil || len(results) != 1 || results[0].ExternalID != "one" {
		t.Fatalf("Search() = %+v, %v", results, err)
	}
	for query, externalID := range map[string]string{
		"并发":         "three",
		"并収编程":       "three",
		"concurency": "one",
	} {
		results, err := store.Search(ctx, entry.Query{Limit: 10, Search: query})
		if err != nil || len(results) == 0 || results[0].ExternalID != externalID {
			t.Fatalf("Search(%q) = %+v, %v", query, results, err)
		}
	}
	results, err = store.Search(ctx, entry.Query{Limit: 10, Search: "reader-source"})
	if err != nil || len(results) != 3 {
		t.Fatalf("Search(source name) = %+v, %v", results, err)
	}
	id := results[0].ID
	displayTitle := "My Go note"
	note := "Read again"
	yes := true
	var storyID story.ID
	if err := pool.QueryRow(ctx, "SELECT story_id FROM story_entries WHERE entry_id = $1", id).Scan(&storyID); err != nil {
		t.Fatalf("find owning Story: %v", err)
	}
	updated, err := storyStore.Update(ctx, storyID, story.Patch{
		Read: &yes, Starred: &yes, Later: &yes, DisplayTitle: &displayTitle, Note: &note,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.ReadAt == nil || updated.StarredAt == nil || updated.LaterAt == nil ||
		updated.DisplayTitle != displayTitle || updated.Note != note {
		t.Errorf("updated = %+v", updated)
	}
	starred, err := storyStore.Search(ctx, story.Query{Limit: 10, State: "starred"})
	if err != nil || len(starred) != 1 {
		t.Fatalf("starred Search() = %+v, %v", starred, err)
	}
	tag, err := storyStore.AddTag(ctx, storyID, " Go ")
	if err != nil || tag.Name != "Go" {
		t.Fatalf("AddTag() = %+v, %v", tag, err)
	}
	tagged, err := storyStore.Search(ctx, story.Query{Limit: 10, Tag: "go"})
	if err != nil || len(tagged) != 1 {
		t.Fatalf("tag Search() = %+v, %v", tagged, err)
	}
	if err := storyStore.RemoveTag(ctx, storyID, tag.ID); err != nil {
		t.Fatalf("RemoveTag() error = %v", err)
	}
}

func TestEntryStoreMarkReadCanBeScopedToSourceOrStoryIDs(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	store := NewEntryStore(pool)
	storyStore := NewStoryStore(pool)
	ctx := context.Background()
	first := createTestSource(t, sourceStore, "mark-read-first")
	second := createTestSource(t, sourceStore, "mark-read-second")

	for _, item := range []struct {
		sourceID source.ID
		key      string
	}{
		{sourceID: first.ID, key: "mark-read-first"},
		{sourceID: second.ID, key: "mark-read-second"},
	} {
		acquisition := claimTestAcquisition(t, acquisitionStore, item.sourceID, item.key)
		if err := store.CommitBatch(ctx, acquisition, "worker", []ingestion.Candidate{
			{ExternalID: item.key, Title: item.key},
		}, json.RawMessage(`{}`)); err != nil {
			t.Fatalf("CommitBatch(%s) error = %v", item.key, err)
		}
	}

	updated, err := storyStore.MarkRead(ctx, string(first.ID), nil)
	if err != nil || updated != 1 {
		t.Fatalf("MarkRead() = %d, %v", updated, err)
	}
	firstUnread, err := storyStore.Search(ctx, story.Query{
		Limit: 10, State: "unread", SourceID: string(first.ID),
	})
	if err != nil || len(firstUnread) != 0 {
		t.Fatalf("first unread = %+v, %v", firstUnread, err)
	}
	secondUnread, err := storyStore.Search(ctx, story.Query{
		Limit: 10, State: "unread", SourceID: string(second.ID),
	})
	if err != nil || len(secondUnread) != 1 {
		t.Fatalf("second unread = %+v, %v", secondUnread, err)
	}

	updated, err = storyStore.MarkRead(ctx, "", []string{string(secondUnread[0].ID)})
	if err != nil || updated != 1 {
		t.Fatalf("MarkRead() by Story ID = %d, %v", updated, err)
	}
	remainingUnread, err := storyStore.Search(ctx, story.Query{Limit: 10, State: "unread"})
	if err != nil || len(remainingUnread) != 0 {
		t.Fatalf("remaining unread = %+v, %v", remainingUnread, err)
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

func TestSearchSourceEntriesCursorPagesUseIndex(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	ctx := context.Background()

	locator := "source-cursor-index-" + time.Now().UTC().Format("20060102150405.000000000")
	src := createTestSource(t, sourceStore, locator)
	acquisition := claimTestAcquisition(t, acquisitionStore, src.ID, locator)

	words := []string{
		"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel",
		"india", "juliet", "kilo", "lima", "mike", "november", "oscar", "papa",
	}
	base := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	candidates := make([]ingestion.Candidate, 0, 120)
	for i := 0; i < 120; i++ {
		published := base.Add(time.Duration(119-i) * time.Minute)
		candidates = append(candidates, ingestion.Candidate{
			ExternalID:  fmt.Sprintf("source-entry-%03d", i),
			Title:       fmt.Sprintf("Source entry %03d %s", i, words[i%len(words)]),
			ContentHTML: fmt.Sprintf("<p>source body %03d paragraph %s</p>", i, words[(i+3)%len(words)]),
			PublishedAt: &published,
		})
	}
	if err := entryStore.CommitBatch(ctx, acquisition, "worker", candidates, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}

	// Walk every page: duplicate-free and globally ordered by
	// (discovered_at, id) DESC. Entries committed in one batch share
	// discovered_at, so ordering rides the id tie-break arm of the cursor
	// comparison.
	var seen []story.SourceEntry
	ids := make(map[string]struct{}, 120)
	pageTwoCursor := ""
	cursor := ""
	for {
		page, err := entryStore.SearchSourceEntryPage(ctx, entry.Query{Limit: 50, SourceID: src.ID, Cursor: cursor})
		if err != nil {
			t.Fatalf("SearchSourceEntryPage() error = %v", err)
		}
		seen = append(seen, page.Entries...)
		if len(seen) == 100 {
			pageTwoCursor = page.NextCursor
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 120 {
		t.Fatalf("paged through %d entries, want 120", len(seen))
	}
	for i, item := range seen {
		if _, dup := ids[string(item.Entry.ID)]; dup {
			t.Fatalf("entry %s returned by two pages (index %d)", item.Entry.ID, i)
		}
		ids[string(item.Entry.ID)] = struct{}{}
		if i > 0 {
			prev := seen[i-1]
			timeAfter := prev.Entry.DiscoveredAt.After(item.Entry.DiscoveredAt)
			idAfter := prev.Entry.DiscoveredAt.Equal(item.Entry.DiscoveredAt) && string(prev.Entry.ID) > string(item.Entry.ID)
			if !timeAfter && !idAfter {
				t.Fatalf("order violated at index %d: (%v, %s) followed by (%v, %s)",
					i, prev.Entry.DiscoveredAt, prev.Entry.ID, item.Entry.DiscoveredAt, item.Entry.ID)
			}
		}
	}
	if pageTwoCursor == "" {
		t.Fatal("page two cursor was not captured")
	}

	// EXPLAIN page two with the store's exact parameters: the cursor
	// predicate must be an index boundary on entries_source_order_idx
	// (equality on source_id plus the row-wise (discovered_at, id)
	// comparison) instead of forcing a Sort over the whole Source.
	position, err := pagination.Decode(pageTwoCursor, "source_entries", "", "", "", string(src.ID))
	if err != nil {
		t.Fatalf("Decode() page-two cursor: %v", err)
	}
	plan := explainWithSortDisabled(t, pool, sourceEntriesSearchSQL,
		51, "", "", "", string(src.ID), 0,
		position.Time, position.ID)
	entriesNode, ok := findExplainNode(plan, "entries")
	if !ok {
		t.Fatal("no entries scan found in EXPLAIN plan")
	}
	if entriesNode.IndexName != "entries_source_order_idx" {
		t.Fatalf("entries scan uses index %q, want entries_source_order_idx", entriesNode.IndexName)
	}
	if !strings.Contains(entriesNode.IndexCond, "discovered_at") {
		t.Fatalf("entries scan Index Cond = %q, want the row-wise cursor comparison on (discovered_at, id)", entriesNode.IndexCond)
	}
}
