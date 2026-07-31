package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/catwenlabs/pulse/internal/ingestion"
)

type sequenceProcessor struct {
	errors []error
	calls  int
}

func (processor *sequenceProcessor) ProcessNext(context.Context, string) error {
	err := processor.errors[processor.calls]
	processor.calls++
	return err
}

func TestRunnerWaitsWhenQueueIsEmptyAndContinues(t *testing.T) {
	processor := &sequenceProcessor{errors: []error{
		nil,
		ingestion.ErrNoAcquisition,
		errors.New("processed failure already scheduled for retry"),
	}}
	ctx, cancel := context.WithCancel(context.Background())
	var waits []time.Duration
	runner := New(processor, "worker-1")
	runner.wait = func(_ context.Context, duration time.Duration) error {
		waits = append(waits, duration)
		if len(waits) == 2 {
			cancel()
		}
		return nil
	}

	if err := runner.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if processor.calls != 3 {
		t.Errorf("processor calls = %d, want 3", processor.calls)
	}
	if len(waits) != 2 || waits[0] >= waits[1] {
		t.Errorf("waits = %v, want short empty-queue wait then longer error wait", waits)
	}
}

func TestWaitCompletesAndCancels(t *testing.T) {
	if err := wait(context.Background(), 0); err != nil {
		t.Fatalf("zero wait error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := wait(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled wait error = %v", err)
	}
}

func TestRunnerStopsWhileWaiting(t *testing.T) {
	processor := &sequenceProcessor{errors: []error{ingestion.ErrNoAcquisition}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := New(processor, "worker-1").Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}
