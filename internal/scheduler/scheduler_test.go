package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/source"
)

type fakeSources struct {
	items  []source.Source
	health map[source.ID]source.Health
}

func (fake fakeSources) List(context.Context) ([]source.Source, error) { return fake.items, nil }
func (fake fakeSources) Health(_ context.Context, id source.ID) (source.Health, error) {
	return fake.health[id], nil
}

type fakeQueue struct {
	requests []ingestion.EnqueueRequest
}

func (queue *fakeQueue) Enqueue(_ context.Context, request ingestion.EnqueueRequest) (ingestion.Acquisition, error) {
	queue.requests = append(queue.requests, request)
	return ingestion.Acquisition{}, nil
}

func TestSchedulerEnqueuesOnlyDuePullSources(t *testing.T) {
	now := time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC)
	queue := &fakeQueue{}
	scheduler := New(fakeSources{
		items: []source.Source{
			{ID: "due", Kind: source.KindRSS, Enabled: true, Config: json.RawMessage(`{"schedule_minutes":15}`)},
			{ID: "fresh", Kind: source.KindJSONAPI, Enabled: true},
			{ID: "paused", Kind: source.KindRSS, Enabled: false},
			{ID: "webhook", Kind: source.KindWebhook, Enabled: true},
		},
		health: map[source.ID]source.Health{
			"due":   {LastRequestedAt: timePointer(now.Add(-16 * time.Minute))},
			"fresh": {LastRequestedAt: timePointer(now.Add(-5 * time.Minute))},
		},
	}, queue)
	scheduler.now = func() time.Time { return now }

	if err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(queue.requests) != 1 || queue.requests[0].SourceID != "due" {
		t.Fatalf("requests = %+v", queue.requests)
	}
	if queue.requests[0].Trigger != ingestion.TriggerSchedule ||
		queue.requests[0].IdempotencyKey == "" {
		t.Errorf("request = %+v", queue.requests[0])
	}
}

func timePointer(value time.Time) *time.Time { return &value }
