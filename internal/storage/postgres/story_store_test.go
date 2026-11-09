package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/catwenlabs/pulse/internal/entry"
	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/pagination"
	"github.com/catwenlabs/pulse/internal/source"
	"github.com/catwenlabs/pulse/internal/story"
)

func TestStoryProcessorGroupsMatchingEntriesAcrossSources(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	storyStore := NewStoryStore(pool)
	ctx := context.Background()

	firstSource := createTestSource(t, sourceStore, "first-story-source")
	secondSource := createTestSource(t, sourceStore, "second-story-source")
	first := claimTestAcquisition(t, acquisitionStore, firstSource.ID, "first-story")
	second := claimTestAcquisition(t, acquisitionStore, secondSource.ID, "second-story")
	content := "<p>OpenAI 今天发布了同一个新模型。</p>"

	if err := entryStore.CommitBatch(ctx, first, "worker", []ingestion.Candidate{{
		ExternalID:  "one",
		Title:       "OpenAI 发布新模型",
		ContentHTML: content,
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("first CommitBatch() error = %v", err)
	}
	if err := entryStore.CommitBatch(ctx, second, "worker", []ingestion.Candidate{{
		ExternalID:  "two",
		Title:       "OpenAI 正式推出新模型",
		ContentHTML: content,
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("second CommitBatch() error = %v", err)
	}

	processor := story.NewProcessor(storyStore, nil)
	if _, err := processor.RunOnce(ctx, 10); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	items, err := storyStore.Search(ctx, story.Query{Limit: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("Stories = %#v", items)
	}
	if items[0].EntryCount != 2 || items[0].SourceCount != 2 {
		t.Errorf("Story = %#v", items[0])
	}
	detail, err := storyStore.Get(ctx, items[0].ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(detail.Entries) != 2 {
		t.Errorf("detail Entries = %#v", detail.Entries)
	}
}

func TestStoryStoreMergeManualCombinesStories(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	storyStore := NewStoryStore(pool)
	ctx := context.Background()

	firstSource := createTestSource(t, sourceStore, "merge-manual-first")
	secondSource := createTestSource(t, sourceStore, "merge-manual-second")
	first := claimTestAcquisition(t, acquisitionStore, firstSource.ID, "merge-manual-first")
	second := claimTestAcquisition(t, acquisitionStore, secondSource.ID, "merge-manual-second")

	if err := entryStore.CommitBatch(ctx, first, "worker", []ingestion.Candidate{{
		ExternalID:  "one",
		Title:       "手动合并的第一条",
		ContentHTML: "<p>alpha body</p>",
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("first CommitBatch() error = %v", err)
	}
	if err := entryStore.CommitBatch(ctx, second, "worker", []ingestion.Candidate{{
		ExternalID:  "two",
		Title:       "手动合并的第二条",
		ContentHTML: "<p>beta body</p>",
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("second CommitBatch() error = %v", err)
	}

	items, err := storyStore.Search(ctx, story.Query{Limit: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 stories before merge, got %d", len(items))
	}
	from, into := items[0].ID, items[1].ID

	if err := storyStore.MergeManual(ctx, from, into, story.MergeOptions{}); err != nil {
		t.Fatalf("MergeManual() error = %v", err)
	}

	merged, err := storyStore.Get(ctx, into)
	if err != nil {
		t.Fatalf("Get() merged story error = %v", err)
	}
	if merged.EntryCount != 2 {
		t.Errorf("EntryCount = %d, want 2", merged.EntryCount)
	}
	if merged.SourceCount != 2 {
		t.Errorf("SourceCount = %d, want 2", merged.SourceCount)
	}
	if len(merged.Entries) != 2 {
		t.Errorf("Entries len = %d, want 2", len(merged.Entries))
	}

	alias, err := storyStore.Get(ctx, from)
	if err != nil {
		t.Fatalf("Get() aliased story error = %v", err)
	}
	if alias.ID != into || alias.EntryCount != 2 {
		t.Errorf("aliased Story = %+v, want canonical Story %q", alias, into)
	}
}

func TestStoryStoreMergeManualRejectsSelfMerge(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	storyStore := NewStoryStore(pool)
	ctx := context.Background()

	src := createTestSource(t, sourceStore, "merge-manual-self")
	acquisition := claimTestAcquisition(t, acquisitionStore, src.ID, "merge-manual-self")
	if err := entryStore.CommitBatch(ctx, acquisition, "worker", []ingestion.Candidate{{
		ExternalID:  "one",
		Title:       "自合并保护",
		ContentHTML: "<p>gamma body</p>",
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}

	items, err := storyStore.Search(ctx, story.Query{Limit: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 story, got %d", len(items))
	}
	id := items[0].ID

	if err := storyStore.MergeManual(ctx, id, id, story.MergeOptions{}); err == nil {
		t.Fatal("expected error merging a Story into itself, got nil")
	}
	if _, err := storyStore.Get(ctx, id); err != nil {
		t.Errorf("Story should still exist after rejected self-merge, got err = %v", err)
	}
}

func TestStoryStoreSplitDetachesEntry(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	storyStore := NewStoryStore(pool)
	ctx := context.Background()

	firstSource := createTestSource(t, sourceStore, "split-first")
	secondSource := createTestSource(t, sourceStore, "split-second")
	first := claimTestAcquisition(t, acquisitionStore, firstSource.ID, "split-first")
	second := claimTestAcquisition(t, acquisitionStore, secondSource.ID, "split-second")
	if err := entryStore.CommitBatch(ctx, first, "worker", []ingestion.Candidate{{
		ExternalID:  "one",
		Title:       "拆分前的第一条",
		ContentHTML: "<p>alpha</p>",
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("first CommitBatch() error = %v", err)
	}
	if err := entryStore.CommitBatch(ctx, second, "worker", []ingestion.Candidate{{
		ExternalID:  "two",
		Title:       "拆分前的第二条",
		ContentHTML: "<p>beta</p>",
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("second CommitBatch() error = %v", err)
	}

	items, err := storyStore.Search(ctx, story.Query{Limit: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if err := storyStore.MergeManual(ctx, items[0].ID, items[1].ID, story.MergeOptions{}); err != nil {
		t.Fatalf("MergeManual() error = %v", err)
	}
	storyID := items[1].ID

	before, err := storyStore.Get(ctx, storyID)
	if err != nil {
		t.Fatalf("Get() before split error = %v", err)
	}
	if len(before.Entries) != 2 {
		t.Fatalf("expected 2 entries before split, got %d", len(before.Entries))
	}
	splitEntryID := before.Entries[0].ID

	newID, err := storyStore.Split(ctx, storyID, splitEntryID, story.SplitOptions{})
	if err != nil {
		t.Fatalf("Split() error = %v", err)
	}

	split, err := storyStore.Get(ctx, newID)
	if err != nil {
		t.Fatalf("Get() split story error = %v", err)
	}
	if split.EntryCount != 1 || len(split.Entries) != 1 || split.Entries[0].ID != splitEntryID {
		t.Errorf("split story = %#v", split)
	}

	remaining, err := storyStore.Get(ctx, storyID)
	if err != nil {
		t.Fatalf("Get() remaining story error = %v", err)
	}
	if remaining.EntryCount != 1 || len(remaining.Entries) != 1 || remaining.Entries[0].ID == splitEntryID {
		t.Errorf("remaining story = %#v", remaining)
	}
}

func TestStoryStoreSearchSurfacesUnreadBeforeRead(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	storyStore := NewStoryStore(pool)
	ctx := context.Background()

	src := createTestSource(t, sourceStore, "story-order")
	acquisition := claimTestAcquisition(t, acquisitionStore, src.ID, "story-order")
	older := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	if err := entryStore.CommitBatch(ctx, acquisition, "worker", []ingestion.Candidate{
		{ExternalID: "unread-story", Title: "Unread story", ContentHTML: "<p>unread body alpha</p>", PublishedAt: &older},
		{ExternalID: "read-story", Title: "Read story", ContentHTML: "<p>read body beta</p>", PublishedAt: &newer},
	}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}

	stories, err := storyStore.Search(ctx, story.Query{Limit: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(stories) != 2 {
		t.Fatalf("expected 2 distinct stories, got %d: %+v", len(stories), stories)
	}
	byExternal := make(map[string]story.Story, len(stories))
	for _, item := range stories {
		byExternal[item.Representative.ExternalID] = item
	}
	readStory, ok := byExternal["read-story"]
	if !ok {
		t.Fatalf("missing read-story in search results: %+v", stories)
	}

	yes := true
	if _, err := storyStore.Update(ctx, readStory.ID, story.Patch{Read: &yes}); err != nil {
		t.Fatalf("mark read Update() error = %v", err)
	}

	results, err := storyStore.Search(ctx, story.Query{Limit: 10})
	if err != nil {
		t.Fatalf("Search() after mark-read error = %v", err)
	}
	// "read-story" has the newer published time, so pure time-ordering would
	// place it first; the unread story must sort ahead of it.
	if got := results[0].Representative.ExternalID; got != "unread-story" {
		t.Errorf("first story ExternalID = %q, want %q (unread before read)", got, "unread-story")
	}
}

func TestStoryStoreMarkClusteredRepairsSingletonAggregateAndPreservesSortTime(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	storyStore := NewStoryStore(pool)
	ctx := context.Background()

	src := createTestSource(t, sourceStore, "story-publication-revision")
	initialAcquisition := claimTestAcquisition(t, acquisitionStore, src.ID, "story-publication-revision-initial")
	future := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Microsecond)
	if err := entryStore.CommitBatch(ctx, initialAcquisition, "worker", []ingestion.Candidate{{
		ExternalID:  "publication-revision",
		Title:       "发布时间会被修正的条目",
		ContentHTML: "<p>revision body</p>",
		PublishedAt: &future,
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("initial CommitBatch() error = %v", err)
	}

	items, err := storyStore.Search(ctx, story.Query{Limit: 10})
	if err != nil || len(items) != 1 {
		t.Fatalf("initial Search() = %+v, %v", items, err)
	}
	storyID := items[0].ID
	var initialSortTime, discoveredAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT story.sort_time, entry.discovered_at
		FROM stories AS story
		JOIN story_entries AS member ON member.story_id = story.id
		JOIN entries AS entry ON entry.id = member.entry_id
		WHERE story.id = $1
	`, storyID).Scan(&initialSortTime, &discoveredAt); err != nil {
		t.Fatalf("query initial Story timing: %v", err)
	}
	if initialSortTime.After(discoveredAt) {
		t.Fatalf("initial sort_time = %s, discovered_at = %s; sort time must be bounded", initialSortTime, discoveredAt)
	}

	corrected := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Microsecond)
	updatedAcquisition := claimTestAcquisition(t, acquisitionStore, src.ID, "story-publication-revision-updated")
	if err := entryStore.CommitBatch(ctx, updatedAcquisition, "worker", []ingestion.Candidate{{
		ExternalID:  "publication-revision",
		Title:       "发布时间会被修正的条目",
		ContentHTML: "<p>revision body</p>",
		PublishedAt: &corrected,
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("updated CommitBatch() error = %v", err)
	}
	updatedBeforeProcessing, err := storyStore.Get(ctx, storyID)
	if err != nil {
		t.Fatalf("Get() before MarkClustered() error = %v", err)
	}
	if updatedBeforeProcessing.FirstPublishedAt == nil || !updatedBeforeProcessing.FirstPublishedAt.Equal(corrected) ||
		updatedBeforeProcessing.LastPublishedAt == nil || !updatedBeforeProcessing.LastPublishedAt.Equal(corrected) {
		t.Errorf("live aggregate before MarkClustered() = first=%v last=%v, want %v", updatedBeforeProcessing.FirstPublishedAt, updatedBeforeProcessing.LastPublishedAt, corrected)
	}

	if err := storyStore.MarkClustered(ctx, storyID); err != nil {
		t.Fatalf("first MarkClustered() error = %v", err)
	}
	got, err := storyStore.Get(ctx, storyID)
	if err != nil {
		t.Fatalf("Get() after MarkClustered() error = %v", err)
	}
	if got.EntryCount != 1 || got.SourceCount != 1 {
		t.Fatalf("repaired counts = %d/%d, want 1/1", got.EntryCount, got.SourceCount)
	}
	if got.FirstPublishedAt == nil || !got.FirstPublishedAt.Equal(corrected) {
		t.Errorf("FirstPublishedAt = %v, want %v", got.FirstPublishedAt, corrected)
	}
	if got.LastPublishedAt == nil || !got.LastPublishedAt.Equal(corrected) {
		t.Errorf("LastPublishedAt = %v, want %v", got.LastPublishedAt, corrected)
	}

	var repairedSortTime time.Time
	if err := pool.QueryRow(ctx, "SELECT sort_time FROM stories WHERE id = $1", storyID).Scan(&repairedSortTime); err != nil {
		t.Fatalf("query repaired sort_time: %v", err)
	}
	if !repairedSortTime.Equal(initialSortTime) {
		t.Errorf("sort_time changed from %s to %s after Entry revision", initialSortTime, repairedSortTime)
	}
	var firstClusteredAt, firstUpdatedAt time.Time
	if err := pool.QueryRow(ctx, "SELECT clustered_at, updated_at FROM stories WHERE id = $1", storyID).Scan(&firstClusteredAt, &firstUpdatedAt); err != nil {
		t.Fatalf("query first clustering timestamps: %v", err)
	}

	if err := storyStore.MarkClustered(ctx, storyID); err != nil {
		t.Fatalf("second MarkClustered() error = %v", err)
	}
	repeated, err := storyStore.Get(ctx, storyID)
	if err != nil {
		t.Fatalf("Get() after repeated MarkClustered() error = %v", err)
	}
	if repeated.FirstPublishedAt == nil || !repeated.FirstPublishedAt.Equal(*got.FirstPublishedAt) ||
		repeated.LastPublishedAt == nil || !repeated.LastPublishedAt.Equal(*got.LastPublishedAt) {
		t.Errorf("repeated MarkClustered() changed aggregate times: first=%v last=%v", repeated.FirstPublishedAt, repeated.LastPublishedAt)
	}
	var repeatedClusteredAt, repeatedUpdatedAt time.Time
	if err := pool.QueryRow(ctx, "SELECT clustered_at, updated_at FROM stories WHERE id = $1", storyID).Scan(&repeatedClusteredAt, &repeatedUpdatedAt); err != nil {
		t.Fatalf("query repeated clustering timestamps: %v", err)
	}
	if !repeatedClusteredAt.Equal(firstClusteredAt) || !repeatedUpdatedAt.Equal(firstUpdatedAt) {
		t.Errorf("repeated MarkClustered() changed timestamps: first=(%s, %s), repeated=(%s, %s)", firstClusteredAt, firstUpdatedAt, repeatedClusteredAt, repeatedUpdatedAt)
	}
}

func TestStoryStoreSearchUsesDiscoveryBoundedSortTime(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	storyStore := NewStoryStore(pool)
	ctx := context.Background()

	src := createTestSource(t, sourceStore, "story-sort-time")
	firstAcquisition := claimTestAcquisition(t, acquisitionStore, src.ID, "story-sort-time-first")
	firstPublished := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Microsecond)
	if err := entryStore.CommitBatch(ctx, firstAcquisition, "worker", []ingestion.Candidate{{
		ExternalID:  "future-older",
		Title:       "source time is further in the future",
		ContentHTML: "<p>first body</p>",
		PublishedAt: &firstPublished,
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("first CommitBatch() error = %v", err)
	}
	if _, err := pool.Exec(ctx, "SELECT pg_sleep(0.01)"); err != nil {
		t.Fatalf("separate discovery timestamps: %v", err)
	}

	secondAcquisition := claimTestAcquisition(t, acquisitionStore, src.ID, "story-sort-time-second")
	secondPublished := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	if err := entryStore.CommitBatch(ctx, secondAcquisition, "worker", []ingestion.Candidate{{
		ExternalID:  "future-newer",
		Title:       "source time is less far in the future",
		ContentHTML: "<p>second body</p>",
		PublishedAt: &secondPublished,
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("second CommitBatch() error = %v", err)
	}

	items, err := storyStore.Search(ctx, story.Query{Limit: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("Search() returned %d stories, want 2: %+v", len(items), items)
	}
	if got := items[0].Representative.ExternalID; got != "future-newer" {
		t.Errorf("first Story = %q, want discovery-newer Story %q", got, "future-newer")
	}
}

func TestStoryStoreMarkClusteredRepairsMultiEntryAggregates(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	storyStore := NewStoryStore(pool)
	ctx := context.Background()

	firstSource := createTestSource(t, sourceStore, "story-publication-multi-first")
	secondSource := createTestSource(t, sourceStore, "story-publication-multi-second")
	firstPublished := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)
	secondPublished := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Microsecond)
	content := "<p>same event body</p>"

	firstAcquisition := claimTestAcquisition(t, acquisitionStore, firstSource.ID, "story-publication-multi-first")
	if err := entryStore.CommitBatch(ctx, firstAcquisition, "worker", []ingestion.Candidate{{
		ExternalID:  "multi-first",
		Title:       "同一个事件的第一条报道",
		ContentHTML: content,
		PublishedAt: &firstPublished,
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("first CommitBatch() error = %v", err)
	}
	secondAcquisition := claimTestAcquisition(t, acquisitionStore, secondSource.ID, "story-publication-multi-second")
	if err := entryStore.CommitBatch(ctx, secondAcquisition, "worker", []ingestion.Candidate{{
		ExternalID:  "multi-second",
		Title:       "同一个事件的第二条报道",
		ContentHTML: content,
		PublishedAt: &secondPublished,
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("second CommitBatch() error = %v", err)
	}

	processor := story.NewProcessor(storyStore, nil)
	if _, err := processor.RunOnce(ctx, 10); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	items, err := storyStore.Search(ctx, story.Query{Limit: 10})
	if err != nil || len(items) != 1 {
		t.Fatalf("Search() after grouping = %+v, %v", items, err)
	}
	storyID := items[0].ID

	corrected := time.Now().UTC().Add(-72 * time.Hour).Truncate(time.Microsecond)
	updatedAcquisition := claimTestAcquisition(t, acquisitionStore, secondSource.ID, "story-publication-multi-second-updated")
	if err := entryStore.CommitBatch(ctx, updatedAcquisition, "worker", []ingestion.Candidate{{
		ExternalID:  "multi-second",
		Title:       "同一个事件的第二条报道",
		ContentHTML: content,
		PublishedAt: &corrected,
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("updated CommitBatch() error = %v", err)
	}
	if err := storyStore.MarkClustered(ctx, storyID); err != nil {
		t.Fatalf("MarkClustered() error = %v", err)
	}

	got, err := storyStore.Get(ctx, storyID)
	if err != nil {
		t.Fatalf("Get() after MarkClustered() error = %v", err)
	}
	if got.EntryCount != 2 || got.SourceCount != 2 {
		t.Fatalf("repaired counts = %d/%d, want 2/2", got.EntryCount, got.SourceCount)
	}
	if got.FirstPublishedAt == nil || !got.FirstPublishedAt.Equal(corrected) {
		t.Errorf("FirstPublishedAt = %v, want %v", got.FirstPublishedAt, corrected)
	}
	if got.LastPublishedAt == nil || !got.LastPublishedAt.Equal(firstPublished) {
		t.Errorf("LastPublishedAt = %v, want %v", got.LastPublishedAt, firstPublished)
	}
}

func TestStoryStoreOwnsReaderMetadataAndState(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	storyStore := NewStoryStore(pool)
	ctx := context.Background()

	src := createTestSource(t, sourceStore, "story-reader-owner")
	acquisition := claimTestAcquisition(t, acquisitionStore, src.ID, "story-reader-owner")
	if err := entryStore.CommitBatch(ctx, acquisition, "worker", []ingestion.Candidate{{
		ExternalID: "owner-entry",
		Title:      "来源标题",
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}
	items, err := storyStore.Search(ctx, story.Query{Limit: 10})
	if err != nil || len(items) != 1 {
		t.Fatalf("Search() = %+v, %v", items, err)
	}

	displayTitle := "我的 Story 标题"
	note := "我的 Reader 笔记"
	read := true
	updated, err := storyStore.Update(ctx, items[0].ID, story.Patch{
		DisplayTitle: &displayTitle,
		Note:         &note,
		Read:         &read,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.DisplayTitle != displayTitle || updated.Note != note || updated.ReadAt == nil {
		t.Fatalf("updated Story = %+v, want Story-owned metadata and state", updated)
	}

	entryItem, err := entryStore.Get(ctx, items[0].Representative.ID)
	if err != nil {
		t.Fatalf("Get Entry() error = %v", err)
	}
	if entryItem.SourceTitle != "来源标题" {
		t.Errorf("Entry source title = %q, want source content to remain intact", entryItem.SourceTitle)
	}
}

func TestSourceEntryProjectionSharesRealStoryStateAcrossSources(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	storyStore := NewStoryStore(pool)
	ctx := context.Background()

	firstSource := createTestSource(t, sourceStore, "source-projection-one")
	secondSource := createTestSource(t, sourceStore, "source-projection-two")
	firstAcquisition := claimTestAcquisition(t, acquisitionStore, firstSource.ID, "source-projection-one")
	secondAcquisition := claimTestAcquisition(t, acquisitionStore, secondSource.ID, "source-projection-two")
	if err := entryStore.CommitBatch(ctx, firstAcquisition, "worker", []ingestion.Candidate{{
		ExternalID: "first", Title: "同一事件的一则报道",
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("first CommitBatch() error = %v", err)
	}
	if err := entryStore.CommitBatch(ctx, secondAcquisition, "worker", []ingestion.Candidate{{
		ExternalID: "second", Title: "同一事件的另一则报道",
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("second CommitBatch() error = %v", err)
	}

	items, err := storyStore.Search(ctx, story.Query{Limit: 10})
	if err != nil || len(items) != 2 {
		t.Fatalf("Search() = %+v, %v", items, err)
	}
	if err := storyStore.MergeManual(ctx, items[1].ID, items[0].ID, story.MergeOptions{}); err != nil {
		t.Fatalf("MergeManual() error = %v", err)
	}
	title := "统一的 Story 标题"
	read := true
	if _, err := storyStore.Update(ctx, items[0].ID, story.Patch{DisplayTitle: &title, Read: &read}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if _, err := storyStore.AddTag(ctx, items[0].ID, "跨来源"); err != nil {
		t.Fatalf("AddTag() error = %v", err)
	}

	for _, sourceID := range []string{string(firstSource.ID), string(secondSource.ID)} {
		projected, err := entryStore.SearchSourceEntries(ctx, entry.Query{Limit: 10, SourceID: source.ID(sourceID)})
		if err != nil || len(projected) != 1 {
			t.Fatalf("SearchSourceEntries(%q) = %+v, %v", sourceID, projected, err)
		}
		item := projected[0]
		if item.Story.ID != items[0].ID || item.Story.DisplayTitle != title || item.Story.ReadAt == nil {
			t.Errorf("Story projection = %+v, want shared owning Story", item.Story)
		}
		if len(item.Story.Tags) != 1 || item.Story.Tags[0].Name != "跨来源" {
			t.Errorf("Story tags = %+v", item.Story.Tags)
		}
	}
}

func TestReaderPagesUseExactCountsAndCursors(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	storyStore := NewStoryStore(pool)
	ctx := context.Background()

	locator := "page-cursor-" + time.Now().UTC().Format("20060102150405.000000000")
	src := createTestSource(t, sourceStore, locator)
	acquisition := claimTestAcquisition(t, acquisitionStore, src.ID, locator)
	if err := entryStore.CommitBatch(ctx, acquisition, "worker", []ingestion.Candidate{
		{ExternalID: "one", Title: "Page one", ContentHTML: "<p>one</p>", PublishedAt: timePtr(time.Now().Add(-3 * time.Hour))},
		{ExternalID: "two", Title: "Page two", ContentHTML: "<p>two</p>", PublishedAt: timePtr(time.Now().Add(-2 * time.Hour))},
		{ExternalID: "three", Title: "Page three", ContentHTML: "<p>three</p>", PublishedAt: timePtr(time.Now().Add(-1 * time.Hour))},
	}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}

	items, err := storyStore.Search(ctx, story.Query{Limit: 10, SourceID: string(src.ID)})
	if err != nil || len(items) != 3 {
		t.Fatalf("Search() = %d items, error = %v", len(items), err)
	}
	if _, err := storyStore.Update(ctx, items[0].ID, story.Patch{Read: boolPtr(true)}); err != nil {
		t.Fatalf("mark Story read: %v", err)
	}
	if _, err := storyStore.Update(ctx, items[1].ID, story.Patch{Hidden: boolPtr(true)}); err != nil {
		t.Fatalf("hide Story: %v", err)
	}

	first, err := storyStore.SearchPage(ctx, story.Query{Limit: 2, SourceID: string(src.ID)})
	if err != nil {
		t.Fatalf("SearchPage() first error = %v", err)
	}
	if first.TotalStories != 3 || first.ReaderCounts.InboxStories != 2 ||
		first.ReaderCounts.UnreadStories != 1 || len(first.Stories) != 2 || first.NextCursor == "" {
		t.Fatalf("first Story page = %+v", first)
	}
	second, err := storyStore.SearchPage(ctx, story.Query{
		Limit: 2, SourceID: string(src.ID), Cursor: first.NextCursor,
	})
	if err != nil {
		t.Fatalf("SearchPage() second error = %v", err)
	}
	// Cursor pages skip the whole-table reader aggregate: counts and totals
	// are zero, and clients reuse the numbers from their first page.
	if len(second.Stories) != 1 || second.NextCursor != "" || second.TotalStories != 0 {
		t.Fatalf("second Story page = %+v", second)
	}
	if second.ReaderCounts.UnreadStories != 0 {
		t.Fatalf("second Story page counts = %+v, want zero", second.ReaderCounts)
	}
	if first.Stories[0].ID == second.Stories[0].ID || first.Stories[1].ID == second.Stories[0].ID {
		t.Fatalf("cursor returned a duplicate Story: first=%+v second=%+v", first.Stories, second.Stories)
	}

	entryPage, err := entryStore.SearchSourceEntryPage(ctx, entry.Query{Limit: 2, SourceID: src.ID})
	if err != nil {
		t.Fatalf("SearchSourceEntryPage() first error = %v", err)
	}
	if entryPage.TotalEntries != 3 || entryPage.ReaderCounts.InboxStories != 2 ||
		entryPage.ReaderCounts.UnreadStories != 1 || len(entryPage.Entries) != 2 || entryPage.NextCursor == "" {
		t.Fatalf("first Source Entry page = %+v", entryPage)
	}
	entryPage2, err := entryStore.SearchSourceEntryPage(ctx, entry.Query{
		Limit: 2, SourceID: src.ID, Cursor: entryPage.NextCursor,
	})
	if err != nil {
		t.Fatalf("SearchSourceEntryPage() second error = %v", err)
	}
	if len(entryPage2.Entries) != 1 || entryPage2.NextCursor != "" {
		t.Fatalf("second Source Entry page = %+v", entryPage2)
	}
	if entryPage2.TotalEntries != 0 {
		t.Fatalf("second Source Entry page total = %d, want zero on cursor pages", entryPage2.TotalEntries)
	}
}

// explainNode mirrors the subset of EXPLAIN (FORMAT JSON) output that the
// cursor-index regression tests assert on.
type explainNode struct {
	NodeType            string        `json:"Node Type"`
	RelationName        string        `json:"Relation Name"`
	IndexName           string        `json:"Index Name"`
	IndexCond           string        `json:"Index Cond"`
	RowsRemovedByFilter *float64      `json:"Rows Removed by Filter"`
	Plans               []explainNode `json:"Plans"`
}

func findExplainNode(node explainNode, relation string) (explainNode, bool) {
	if node.RelationName == relation {
		return node, true
	}
	for _, child := range node.Plans {
		if found, ok := findExplainNode(child, relation); ok {
			return found, true
		}
	}
	return explainNode{}, false
}

// explainWithSortDisabled runs EXPLAIN (ANALYZE) on a dedicated pooled
// connection with enable_sort turned off. Small test fixtures would
// otherwise let the planner satisfy ORDER BY with a cheap explicit Sort
// over the whole (tiny) table, hiding whether the cursor predicate can act
// as an index boundary — the property these tests exist to pin down. The
// setting is reset before the connection returns to the pool.
func explainWithSortDisabled(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) explainNode {
	t.Helper()
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire EXPLAIN connection: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SET enable_sort = off"); err != nil {
		t.Fatalf("disable sort for EXPLAIN: %v", err)
	}
	defer func() { _, _ = conn.Exec(ctx, "RESET enable_sort") }()
	var raw []byte
	if err := conn.QueryRow(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+sql, args...).Scan(&raw); err != nil {
		t.Fatalf("EXPLAIN failed: %v", err)
	}
	var plan []struct {
		Plan explainNode `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatalf("unmarshal EXPLAIN JSON: %v\n%s", err, raw)
	}
	if len(plan) != 1 {
		t.Fatalf("expected exactly one EXPLAIN statement, got %d", len(plan))
	}
	return plan[0].Plan
}

func TestSearchCursorPagesUseReaderOrderIndex(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	storyStore := NewStoryStore(pool)
	ctx := context.Background()

	locator := "cursor-index-" + time.Now().UTC().Format("20060102150405.000000000")
	src := createTestSource(t, sourceStore, locator)
	acquisition := claimTestAcquisition(t, acquisitionStore, src.ID, locator)

	words := []string{
		"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel",
		"india", "juliet", "kilo", "lima", "mike", "november", "oscar", "papa",
		"quebec", "romeo", "sierra", "tango", "uniform", "victor", "whiskey", "xray",
	}
	base := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	candidates := make([]ingestion.Candidate, 0, 120)
	for i := 0; i < 120; i++ {
		published := base.Add(time.Duration(119-i) * time.Minute)
		if i%14 == 4 {
			// Duplicate the previous same-parity sort time so the
			// (sort_time, id) tie-break arm of the cursor comparison is
			// exercised within one reader bucket.
			published = base.Add(time.Duration(119-(i-2)) * time.Minute)
		}
		candidates = append(candidates, ingestion.Candidate{
			ExternalID:  fmt.Sprintf("cursor-story-%03d", i),
			Title:       fmt.Sprintf("Cursor story %03d %s", i, words[i%len(words)]),
			ContentHTML: fmt.Sprintf("<p>cursor body %03d paragraph %s</p>", i, words[(i+5)%len(words)]),
			PublishedAt: &published,
		})
	}
	if err := entryStore.CommitBatch(ctx, acquisition, "worker", candidates, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}

	// Split the stories across both reader-order buckets: even indexes read.
	all, err := storyStore.Search(ctx, story.Query{Limit: 200, SourceID: string(src.ID)})
	if err != nil {
		t.Fatalf("seed Search() error = %v", err)
	}
	if len(all) != 120 {
		t.Fatalf("seed Search() returned %d stories, want 120 (clustering must not merge them)", len(all))
	}
	readIDs := make([]string, 0, 60)
	for i, item := range all {
		if i%2 == 0 {
			readIDs = append(readIDs, string(item.ID))
		}
	}
	if _, err := storyStore.MarkRead(ctx, "", readIDs); err != nil {
		t.Fatalf("MarkRead() error = %v", err)
	}

	// Walk every page: the concatenation must be duplicate-free and globally
	// ordered (unread bucket first, then read, each (sort_time, id) DESC).
	// This pins the semantic equivalence of the row-wise cursor predicate.
	var seen []story.Story
	ids := make(map[string]struct{}, 120)
	pageTwoCursor := ""
	cursor := ""
	for {
		page, err := storyStore.SearchPage(ctx, story.Query{Limit: 50, SourceID: string(src.ID), Cursor: cursor})
		if err != nil {
			t.Fatalf("SearchPage() error = %v", err)
		}
		seen = append(seen, page.Stories...)
		if len(seen) == 100 {
			pageTwoCursor = page.NextCursor
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 120 {
		t.Fatalf("paged through %d stories, want 120", len(seen))
	}
	lastBucket, lastTime, lastID := -1, time.Time{}, ""
	for i, item := range seen {
		if _, dup := ids[string(item.ID)]; dup {
			t.Fatalf("story %s returned by two pages (index %d)", item.ID, i)
		}
		ids[string(item.ID)] = struct{}{}
		if item.SortTime == nil {
			t.Fatalf("story %s has nil SortTime", item.ID)
		}
		bucket := 1
		if item.ReadAt != nil {
			bucket = 0
		}
		if i > 0 {
			if lastBucket == 0 && bucket == 1 {
				t.Fatalf("reader bucket regressed at index %d: read before unread", i)
			}
			if lastBucket == bucket {
				timeAfter := lastTime.After(*item.SortTime)
				idAfter := lastTime.Equal(*item.SortTime) && lastID > string(item.ID)
				if !timeAfter && !idAfter {
					t.Fatalf("order violated at index %d: (%v, %s) followed by (%v, %s)",
						i, lastTime, lastID, *item.SortTime, item.ID)
				}
			}
		}
		lastBucket, lastTime, lastID = bucket, *item.SortTime, string(item.ID)
	}
	if pageTwoCursor == "" {
		t.Fatal("page two cursor was not captured")
	}

	// Re-run page two under EXPLAIN with the exact parameters the store
	// sends and require the cursor predicate to be an index boundary on
	// stories_reader_order_idx. The pre-row-value predicate degraded to a
	// filter that rescanned the index from the top (~50k rows removed per
	// deep page in production).
	position, err := pagination.Decode(pageTwoCursor, "stories", "", "", "", string(src.ID))
	if err != nil {
		t.Fatalf("Decode() page-two cursor: %v", err)
	}
	plan := explainWithSortDisabled(t, pool, storySearchSQL,
		51, "", "", "", string(src.ID), 0,
		position.Bucket == 1, position.Time, position.ID)
	storiesNode, ok := findExplainNode(plan, "stories")
	if !ok {
		t.Fatal("no stories scan found in EXPLAIN plan")
	}
	if storiesNode.IndexName != "stories_reader_order_idx" {
		t.Fatalf("stories scan uses index %q, want stories_reader_order_idx", storiesNode.IndexName)
	}
	if !strings.Contains(storiesNode.IndexCond, "ROW(") {
		t.Fatalf("stories scan Index Cond = %q, want the row-wise cursor comparison", storiesNode.IndexCond)
	}
	if storiesNode.RowsRemovedByFilter != nil && *storiesNode.RowsRemovedByFilter > 0 {
		t.Fatalf("stories scan removed %v rows by filter; the cursor is not an index boundary",
			*storiesNode.RowsRemovedByFilter)
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func timePtr(value time.Time) *time.Time {
	return &value
}
