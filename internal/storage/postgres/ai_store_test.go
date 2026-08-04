package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/catwenlabs/pulse/internal/ai"
	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/story"
)

func TestAIStorePersistsStorySummaryAndDigestSnapshots(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE ai_jobs, story_ai_summaries, ai_digests CASCADE"); err != nil {
		t.Fatalf("truncate AI data: %v", err)
	}

	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	storyStore := NewStoryStore(pool)
	src := createTestSource(t, sourceStore, "ai-store-lifecycle")
	acquisition := claimTestAcquisition(t, acquisitionStore, src.ID, "ai-store-lifecycle")
	if err := entryStore.CommitBatch(ctx, acquisition, "worker", []ingestion.Candidate{{
		ExternalID:  "ai-entry-1",
		Title:       "AI 摘要测试标题",
		Summary:     "AI 摘要测试摘要",
		ContentHTML: "<p>AI 摘要测试正文</p>",
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}
	if _, err := story.NewProcessor(storyStore, nil).RunOnce(ctx, 10); err != nil {
		t.Fatalf("Story processor RunOnce() error = %v", err)
	}
	stories, err := storyStore.Search(ctx, story.Query{Limit: 10})
	if err != nil || len(stories) != 1 {
		t.Fatalf("Stories = %+v, error = %v", stories, err)
	}

	store := NewAIStore(pool, AIStoreOptions{MaxActiveJobs: 4})
	snapshot, err := store.SnapshotStory(ctx, string(stories[0].ID))
	if err != nil {
		t.Fatalf("SnapshotStory() error = %v", err)
	}
	if snapshot.MembershipFingerprint == "" || len(snapshot.Entries) != 1 {
		t.Fatalf("Story snapshot = %+v", snapshot)
	}

	_, receipt, err := store.EnqueueStorySummary(ctx, snapshot, ai.ProviderMetadata{Name: "fake", Model: "fake-model"})
	if err != nil {
		t.Fatalf("EnqueueStorySummary() error = %v", err)
	}
	if receipt.ID == "" || receipt.TargetID != string(stories[0].ID) {
		t.Fatalf("StorySummary receipt = %+v", receipt)
	}
	job, err := store.Claim(ctx, "ai-test-worker", time.Minute)
	if err != nil {
		t.Fatalf("Claim() StorySummary error = %v", err)
	}
	if job.ID != receipt.ID {
		t.Fatalf("claimed job = %+v, receipt = %+v", job, receipt)
	}
	if err := store.CompleteStorySummary(ctx, job, "ai-test-worker", ai.GeneratedStorySummary{
		Overview:  "测试摘要",
		KeyPoints: []string{"测试要点"},
	}, ai.ProviderMetadata{Name: "fake", Model: "fake-model"}); err != nil {
		t.Fatalf("CompleteStorySummary() error = %v", err)
	}
	summary, err := store.GetStorySummary(ctx, string(stories[0].ID))
	if err != nil || summary.Status != ai.StatusCompleted || summary.Overview != "测试摘要" {
		t.Fatalf("StorySummary = %+v, error = %v", summary, err)
	}

	items, err := store.SnapshotUnreadStories(ctx, ai.DigestScope{})
	if err != nil || len(items) != 1 || items[0].InputFingerprint == "" {
		t.Fatalf("Digest snapshot = %+v, error = %v", items, err)
	}
	digest, digestReceipt, err := store.EnqueueDigest(ctx, ai.DigestScope{}, items, "digest-fingerprint", ai.ProviderMetadata{Name: "fake", Model: "fake-model"})
	if err != nil {
		t.Fatalf("EnqueueDigest() error = %v", err)
	}
	if digestReceipt.ID == "" || digest.ID != digestReceipt.TargetID {
		t.Fatalf("Digest receipt = %+v, digest = %+v", digestReceipt, digest)
	}
	digestJob, err := store.Claim(ctx, "ai-test-worker", time.Minute)
	if err != nil {
		t.Fatalf("Claim() Digest error = %v", err)
	}
	if err := store.CompleteDigest(ctx, digestJob, "ai-test-worker", ai.GeneratedDigest{
		Overview: "标题级测试追更",
		Themes:   []ai.GeneratedDigestTheme{{Title: "测试主题", Summary: "标题级归类", StoryLabels: []string{items[0].Label}}},
	}, ai.ProviderMetadata{Name: "fake", Model: "fake-model"}); err != nil {
		t.Fatalf("CompleteDigest() error = %v", err)
	}
	gotDigest, err := store.GetDigest(ctx, digest.ID)
	if err != nil || gotDigest.Status != ai.StatusCompleted || len(gotDigest.Stories) != 1 || !gotDigest.Stories[0].Available {
		t.Fatalf("Digest = %+v, error = %v", gotDigest, err)
	}
}
