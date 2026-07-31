package story

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/catwenlabs/pulse/internal/embedding"
	"github.com/catwenlabs/pulse/internal/entry"
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

// TestProcessorRunOnceSerializes verifies that two concurrent RunOnce passes do
// not overlap: an HTTP-triggered recompute must not race the background loop.
func TestProcessorRunOnceSerializes(t *testing.T) {
	repository := &serializeRepository{proceed: make(chan struct{}), entered: make(chan struct{}, 1)}
	processor := NewProcessor(repository, nil)

	go func() { _, _ = processor.RunOnce(context.Background(), 1) }()
	<-repository.entered // first pass is inside Pending and holds runMu

	done := make(chan struct{}, 2)
	go func() { _, _ = processor.RunOnce(context.Background(), 1); done <- struct{}{} }()

	// While the first pass is parked inside Pending, the second must not enter it.
	time.Sleep(50 * time.Millisecond)
	if got := repository.count.Load(); got != 1 {
		t.Fatalf("concurrent Pending count = %d, want 1 (RunOnce passes must serialize)", got)
	}

	close(repository.proceed) // release the first pass; the second may now proceed
	<-done
}

type serializeRepository struct {
	count   atomic.Int32
	proceed chan struct{}
	entered chan struct{}
}

func (repository *serializeRepository) Pending(context.Context, int, string) ([]Candidate, error) {
	repository.count.Add(1)
	select {
	case repository.entered <- struct{}{}:
	default:
	}
	<-repository.proceed
	repository.count.Add(-1)
	return nil, nil
}

func (repository *serializeRepository) Candidates(context.Context, Candidate, int) ([]Candidate, error) {
	return nil, nil
}

func (repository *serializeRepository) SaveFeatures(context.Context, entry.ID, Features) error {
	return nil
}

func (repository *serializeRepository) SaveEmbedding(context.Context, entry.ID, string, []float32) error {
	return nil
}

func (repository *serializeRepository) MarkEmbeddingAttempted(context.Context, entry.ID) error {
	return nil
}

func (repository *serializeRepository) Merge(context.Context, ID, ID, Result) error {
	return nil
}

func (repository *serializeRepository) MarkClustered(context.Context, ID) error {
	return nil
}
