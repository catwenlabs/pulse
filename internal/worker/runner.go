package worker

import (
	"context"
	"errors"
	"time"

	"github.com/wenpengfei/pulse/internal/ingestion"
)

const (
	emptyQueueWait = 250 * time.Millisecond
	errorWait      = time.Second
)

type processor interface {
	ProcessNext(context.Context, string) error
}

type Runner struct {
	processor processor
	owner     string
	wait      func(context.Context, time.Duration) error
}

func New(processor processor, owner string) *Runner {
	return &Runner{
		processor: processor,
		owner:     owner,
		wait:      wait,
	}
}

func (runner *Runner) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		err := runner.processor.ProcessNext(ctx, runner.owner)
		delay := errorWait
		if errors.Is(err, ingestion.ErrNoAcquisition) {
			delay = emptyQueueWait
		}
		if err == nil {
			continue
		}
		if err := runner.wait(ctx, delay); err != nil {
			return nil
		}
	}
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
