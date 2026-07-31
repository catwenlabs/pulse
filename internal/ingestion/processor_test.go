package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/catwenlabs/pulse/internal/source"
)

type fakeQueue struct {
	claimed Acquisition
	retried bool
}

func (queue *fakeQueue) Claim(context.Context, string, time.Duration) (Acquisition, error) {
	return queue.claimed, nil
}
func (queue *fakeQueue) Retry(context.Context, AcquisitionID, string, time.Time, error) error {
	queue.retried = true
	return nil
}

type fakeSources struct {
	value source.Source
}

func (sources fakeSources) Get(context.Context, source.ID) (source.Source, error) {
	return sources.value, nil
}
func (sources fakeSources) Checkpoint(context.Context, source.ID) (Checkpoint, error) {
	return nil, nil
}

type fakeCommitter struct {
	candidates []Candidate
	checkpoint json.RawMessage
}

func (committer *fakeCommitter) CommitBatch(
	_ context.Context,
	_ Acquisition,
	_ string,
	candidates []Candidate,
	checkpoint json.RawMessage,
) error {
	committer.candidates = candidates
	committer.checkpoint = checkpoint
	return nil
}

type processorDriver struct {
	stubDriver
	err     error
	payload *string
}

func (driver processorDriver) Acquire(_ context.Context, request AcquireRequest) (AcquisitionBatch, error) {
	if driver.payload != nil && request.Payload != nil {
		body, _ := io.ReadAll(request.Payload)
		*driver.payload = string(body)
	}
	return AcquisitionBatch{
		Candidates:     []Candidate{{ExternalID: "item-1"}},
		NextCheckpoint: Checkpoint(`{"cursor":"next"}`),
	}, driver.err
}

func TestProcessorCommitsDriverBatch(t *testing.T) {
	var payload string
	queue := &fakeQueue{claimed: Acquisition{
		ID: "job", SourceID: "source", Trigger: TriggerSchedule,
		Payload: json.RawMessage(`{"title":"pushed"}`),
	}}
	committer := &fakeCommitter{}
	registry, err := NewRegistry(processorDriver{
		stubDriver: stubDriver{kind: source.KindRSS},
		payload:    &payload,
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	processor := NewProcessor(
		queue,
		fakeSources{value: source.Source{ID: "source", Kind: source.KindRSS}},
		committer,
		registry,
	)

	if err := processor.ProcessNext(context.Background(), "worker"); err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	if len(committer.candidates) != 1 || string(committer.checkpoint) != `{"cursor":"next"}` {
		t.Errorf("committed candidates = %+v, checkpoint = %s", committer.candidates, committer.checkpoint)
	}
	if payload != `{"title":"pushed"}` {
		t.Errorf("driver payload = %q", payload)
	}
}

func TestProcessorRetriesDriverFailure(t *testing.T) {
	queue := &fakeQueue{claimed: Acquisition{ID: "job", SourceID: "source", Trigger: TriggerSchedule}}
	registry, err := NewRegistry(processorDriver{
		stubDriver: stubDriver{kind: source.KindRSS},
		err:        errors.New("remote failure"),
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	processor := NewProcessor(
		queue,
		fakeSources{value: source.Source{ID: "source", Kind: source.KindRSS}},
		&fakeCommitter{},
		registry,
	)

	if err := processor.ProcessNext(context.Background(), "worker"); err == nil {
		t.Fatal("ProcessNext() error = nil")
	}
	if !queue.retried {
		t.Fatal("failed acquisition was not scheduled for retry")
	}
}
