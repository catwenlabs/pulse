package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/wenpengfei/pulse/internal/entry"
	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/story"
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

	if err := storyStore.MergeManual(ctx, from, into); err != nil {
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

	if _, err := storyStore.Get(ctx, from); !errors.Is(err, entry.ErrNotFound) {
		t.Errorf("expected from-story deleted (ErrNotFound), got err = %v", err)
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

	if err := storyStore.MergeManual(ctx, id, id); err == nil {
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
	if err := storyStore.MergeManual(ctx, items[0].ID, items[1].ID); err != nil {
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

	newID, err := storyStore.Split(ctx, storyID, splitEntryID)
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
