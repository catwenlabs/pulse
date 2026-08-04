package ai

import (
	"strings"
	"testing"
)

func TestParseStorySummaryMapsAndValidatesSourceLabels(t *testing.T) {
	snapshot := StorySnapshot{
		StoryID: "story-1",
		Title:   "Story title",
		Entries: []StoryEntrySnapshot{{Label: "E1", EntryID: "entry-1", SourceTitle: "Source", Title: "Entry title"}},
	}
	result, err := parseStorySummary(`{"overview":"overview","key_points":["point"],"source_notes":[{"label":"E1","note":"adds detail"}]}`, snapshot)
	if err != nil {
		t.Fatalf("parseStorySummary() error = %v", err)
	}
	if result.Overview != "overview" || len(result.KeyPoints) != 1 || len(result.Sources) != 1 || result.Sources[0].EntryID != "entry-1" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := parseStorySummary(`{"overview":"overview","source_notes":[{"label":"E9","note":"unknown"}]}`, snapshot); err == nil {
		t.Fatal("unknown source label error = nil")
	}
	if _, err := parseStorySummary(`{"overview":"overview","source_notes":[{"label":"E1","note":"  "}]}`, snapshot); err == nil {
		t.Fatal("empty source note error = nil")
	}
}

func TestParseDigestMapsAndValidatesStoryLabels(t *testing.T) {
	items := []DigestStorySnapshot{
		{Label: "S1", StoryID: "story-1", Title: "one"},
		{Label: "S2", StoryID: "story-2", Title: "two"},
	}
	result, err := parseDigest(`{"overview":"overview","themes":[{"title":"Theme","summary":"summary","story_labels":[" S1 ","S2"]}],"priorities":[{"rank":1,"title":"Read first","reason":"important","story_labels":["S1"]}]}`, items)
	if err != nil {
		t.Fatalf("parseDigest() error = %v", err)
	}
	if len(result.Themes) != 1 || len(result.Priorities) != 1 || result.Priorities[0].Rank != 1 {
		t.Fatalf("result = %+v", result)
	}
	if _, err := parseDigest(`{"overview":"overview","themes":[{"title":"Theme","summary":"summary","story_labels":["S9"]}]}`, items); err == nil {
		t.Fatal("unknown Story label error = nil")
	}
}

func TestDigestRequestDoesNotIncludeEntryBody(t *testing.T) {
	request := digestRequest([]DigestStorySnapshot{{Label: "S1", Title: "A title", SourceTitle: "A source"}})
	if len(request.Messages) != 2 || strings.Contains(request.Messages[1].Content, "body-secret") {
		t.Errorf("Digest prompt exposes content fields: %q", request.Messages[1].Content)
	}
	if request.MaxTokens != 4096 {
		t.Errorf("Digest MaxTokens = %d, want 4096", request.MaxTokens)
	}
}
