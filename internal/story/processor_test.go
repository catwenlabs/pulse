package story

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wenpengfei/pulse/internal/embedding"
	"github.com/wenpengfei/pulse/internal/entry"
)

func TestProcessorMergesExactContentStory(t *testing.T) {
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	repository := &fakeRepository{
		pending: []Candidate{{
			StoryID:  "new-story",
			Entry:    entry.Entry{ID: "new-entry", SourceTitle: "OpenAI 发布模型", DiscoveredAt: now},
			Features: BuildFeatures("OpenAI 发布模型", "相同的新闻正文"),
		}},
		candidates: []Candidate{{
			StoryID:  "existing-story",
			Entry:    entry.Entry{ID: "existing-entry", SourceTitle: "OpenAI正式发布模型", DiscoveredAt: now},
			Features: BuildFeatures("OpenAI正式发布模型", "相同的新闻正文"),
		}},
	}
	processor := NewProcessor(repository, nil)

	processed, err := processor.RunOnce(context.Background(), 10)

	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if processed != 1 || repository.mergedInto != "existing-story" {
		t.Fatalf("processed = %d, mergedInto = %q", processed, repository.mergedInto)
	}
	if repository.match.Method != MatchContentHash {
		t.Errorf("match = %+v", repository.match)
	}
}

func TestProcessorFallsBackWhenEmbeddingFails(t *testing.T) {
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	repository := &fakeRepository{
		pending: []Candidate{{
			StoryID:  "story",
			Entry:    entry.Entry{ID: "entry", SourceTitle: "独立新闻", DiscoveredAt: now},
			Features: BuildFeatures("独立新闻", "正文"),
		}},
	}
	processor := NewProcessor(repository, failingProvider{})

	processed, err := processor.RunOnce(context.Background(), 10)

	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if processed != 1 || repository.marked != "story" {
		t.Fatalf("processed = %d, marked = %q", processed, repository.marked)
	}
	if repository.embeddingAttempted != "entry" {
		t.Errorf("embeddingAttempted = %q", repository.embeddingAttempted)
	}
}

type fakeRepository struct {
	pending            []Candidate
	candidates         []Candidate
	mergedInto         ID
	marked             ID
	embeddingAttempted entry.ID
	match              Result
}

func (repository *fakeRepository) Pending(context.Context, int, string) ([]Candidate, error) {
	return repository.pending, nil
}

func (repository *fakeRepository) Candidates(context.Context, Candidate, int) ([]Candidate, error) {
	return repository.candidates, nil
}

func (repository *fakeRepository) SaveFeatures(context.Context, entry.ID, Features) error {
	return nil
}

func (repository *fakeRepository) SaveEmbedding(
	context.Context,
	entry.ID,
	string,
	[]float32,
) error {
	return nil
}

func (repository *fakeRepository) MarkEmbeddingAttempted(
	_ context.Context,
	id entry.ID,
) error {
	repository.embeddingAttempted = id
	return nil
}

func (repository *fakeRepository) Merge(
	_ context.Context,
	from ID,
	into ID,
	match Result,
) error {
	repository.mergedInto = into
	repository.match = match
	return nil
}

func (repository *fakeRepository) MarkClustered(_ context.Context, id ID) error {
	repository.marked = id
	return nil
}

type failingProvider struct{}

func (failingProvider) Model() string {
	return "qwen3-embedding"
}

func (failingProvider) Embed(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("ollama unavailable")
}

var _ embedding.Provider = failingProvider{}
