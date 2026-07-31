package story

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/catwenlabs/pulse/internal/embedding"
	"github.com/catwenlabs/pulse/internal/entry"
)

const candidateLimit = 200

type Repository interface {
	Pending(context.Context, int, string) ([]Candidate, error)
	Candidates(context.Context, Candidate, int) ([]Candidate, error)
	SaveFeatures(context.Context, entry.ID, Features) error
	SaveEmbedding(context.Context, entry.ID, string, []float32) error
	MarkEmbeddingAttempted(context.Context, entry.ID) error
	Merge(context.Context, ID, ID, Result) error
	MarkClustered(context.Context, ID) error
}

type Processor struct {
	repository Repository
	embedder   embedding.Provider
	now        func() time.Time

	mu                  sync.Mutex
	runMu               sync.Mutex
	embeddingRetryAfter time.Time
}

func NewProcessor(repository Repository, embedder embedding.Provider) *Processor {
	return &Processor{
		repository: repository,
		embedder:   embedder,
		now:        time.Now,
	}
}

func (processor *Processor) RunOnce(ctx context.Context, limit int) (int, error) {
	processor.runMu.Lock()
	defer processor.runMu.Unlock()

	model := ""
	if processor.embedder != nil {
		model = processor.embedder.Model()
	}
	items, err := processor.repository.Pending(ctx, limit, model)
	if err != nil {
		return 0, fmt.Errorf("list pending stories: %w", err)
	}
	for index, item := range items {
		if err := processor.process(ctx, item); err != nil {
			return index, err
		}
	}
	return len(items), nil
}

func (processor *Processor) Run(ctx context.Context) error {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			if _, err := processor.RunOnce(ctx, 50); err != nil && ctx.Err() == nil {
				slog.Error("Story aggregation pass failed", "error", err)
			}
			timer.Reset(30 * time.Second)
		}
	}
}

func (processor *Processor) process(ctx context.Context, item Candidate) error {
	model := ""
	if processor.embedder != nil {
		model = processor.embedder.Model()
	}
	if item.Features.NormalizedTitle == "" &&
		item.Features.ContentHash == "" &&
		item.Features.ContentSimHash == 0 {
		content := item.Entry.ContentHTML
		if content == "" {
			content = item.Entry.Summary
		}
		item.Features = BuildFeatures(item.Entry.SourceTitle, content)
		item.Features.CanonicalURL = item.Entry.CanonicalURL
		if err := processor.repository.SaveFeatures(ctx, item.Entry.ID, item.Features); err != nil {
			return fmt.Errorf("save features for Entry %s: %w", item.Entry.ID, err)
		}
	}
	if (len(item.Features.Embedding) == 0 || item.Features.EmbeddingModel != model) &&
		processor.embeddingAvailable() {
		if err := processor.addEmbedding(ctx, &item); err != nil {
			slog.Warn("story embedding unavailable; using text features", "error", err)
			processor.pauseEmbedding()
		}
	}

	candidates, err := processor.repository.Candidates(ctx, item, candidateLimit)
	if err != nil {
		return fmt.Errorf("list candidates for story %s: %w", item.StoryID, err)
	}
	bestScore := -1.0
	var best Candidate
	var bestMatch Result
	for _, candidate := range candidates {
		if candidate.StoryID == item.StoryID {
			continue
		}
		match := Match(
			item.Features,
			candidate.Features,
			entryTime(item.Entry),
			entryTime(candidate.Entry),
		)
		if match.Matched && match.FinalScore > bestScore {
			bestScore = match.FinalScore
			best = candidate
			bestMatch = match
		}
	}
	if bestScore >= 0 {
		if err := processor.repository.Merge(
			ctx,
			item.StoryID,
			best.StoryID,
			bestMatch,
		); err != nil {
			return fmt.Errorf("merge story %s into %s: %w", item.StoryID, best.StoryID, err)
		}
		return nil
	}
	if err := processor.repository.MarkClustered(ctx, item.StoryID); err != nil {
		return fmt.Errorf("mark story %s clustered: %w", item.StoryID, err)
	}
	return nil
}

func (processor *Processor) addEmbedding(ctx context.Context, item *Candidate) error {
	input := embeddingInput(item.Entry)
	vectors, err := processor.embedder.Embed(ctx, []string{input})
	if markErr := processor.repository.MarkEmbeddingAttempted(ctx, item.Entry.ID); markErr != nil {
		return fmt.Errorf("mark embedding attempted: %w", markErr)
	}
	if err != nil {
		return err
	}
	if len(vectors) != 1 {
		return fmt.Errorf("embedding provider returned %d vectors for one Entry", len(vectors))
	}
	if err := processor.repository.SaveEmbedding(
		ctx,
		item.Entry.ID,
		processor.embedder.Model(),
		vectors[0],
	); err != nil {
		return fmt.Errorf("save embedding: %w", err)
	}
	item.Features.Embedding = vectors[0]
	item.Features.EmbeddingModel = processor.embedder.Model()
	return nil
}

func (processor *Processor) embeddingAvailable() bool {
	if processor.embedder == nil {
		return false
	}
	processor.mu.Lock()
	defer processor.mu.Unlock()
	return !processor.now().Before(processor.embeddingRetryAfter)
}

func (processor *Processor) pauseEmbedding() {
	processor.mu.Lock()
	defer processor.mu.Unlock()
	processor.embeddingRetryAfter = processor.now().Add(time.Minute)
}

func embeddingInput(item entry.Entry) string {
	const maxBodyRunes = 500
	body := strings.Join([]string{item.Summary, item.ContentHTML}, "\n")
	if runes := []rune(body); len(runes) > maxBodyRunes {
		body = string(runes[:maxBodyRunes])
	}
	parts := []string{item.SourceTitle}
	if item.PublishedAt != nil {
		parts = append(parts, item.PublishedAt.UTC().Format(time.RFC3339))
	}
	parts = append(parts, body)
	return strings.Join(parts, "\n")
}

func entryTime(item entry.Entry) time.Time {
	if item.PublishedAt != nil {
		return *item.PublishedAt
	}
	return item.DiscoveredAt
}
