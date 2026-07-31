package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/source"
)

type sourceRepository interface {
	List(context.Context) ([]source.Source, error)
	Health(context.Context, source.ID) (source.Health, error)
}

type acquisitionQueue interface {
	Enqueue(context.Context, ingestion.EnqueueRequest) (ingestion.Acquisition, error)
}

type Scheduler struct {
	sources sourceRepository
	queue   acquisitionQueue
	now     func() time.Time
}

func New(sources sourceRepository, queue acquisitionQueue) *Scheduler {
	return &Scheduler{sources: sources, queue: queue, now: time.Now}
}

func (scheduler *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if err := scheduler.RunOnce(ctx); err != nil && ctx.Err() == nil {
			// Individual failures are retried on the next tick; keep the scheduler alive.
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (scheduler *Scheduler) RunOnce(ctx context.Context) error {
	items, err := scheduler.sources.List(ctx)
	if err != nil {
		return fmt.Errorf("list scheduled sources: %w", err)
	}
	now := scheduler.now().UTC()
	var failures []error
	for _, item := range items {
		if !item.Enabled || !item.Kind.IsHTTP() {
			continue
		}
		interval := scheduleInterval(item.Config)
		health, err := scheduler.sources.Health(ctx, item.ID)
		if err != nil {
			failures = append(failures, fmt.Errorf("health for source %s: %w", item.ID, err))
			continue
		}
		if health.LastRequestedAt != nil && health.LastRequestedAt.Add(interval).After(now) {
			continue
		}
		slot := now.Unix() / int64(interval/time.Second)
		_, err = scheduler.queue.Enqueue(ctx, ingestion.EnqueueRequest{
			SourceID: item.ID, Trigger: ingestion.TriggerSchedule,
			IdempotencyKey: fmt.Sprintf("schedule:%d", slot),
		})
		if err != nil {
			failures = append(failures, fmt.Errorf("schedule source %s: %w", item.ID, err))
		}
	}
	return errors.Join(failures...)
}

func scheduleInterval(config json.RawMessage) time.Duration {
	var value struct {
		Minutes int `json:"schedule_minutes"`
	}
	_ = json.Unmarshal(config, &value)
	if value.Minutes < 1 || value.Minutes > 24*60 {
		value.Minutes = 30
	}
	return time.Duration(value.Minutes) * time.Minute
}
