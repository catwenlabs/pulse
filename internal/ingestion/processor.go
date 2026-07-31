package ingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/catwenlabs/pulse/internal/source"
)

const (
	defaultLease = 2 * time.Minute
	retryDelay   = time.Minute
)

type acquisitionQueue interface {
	Claim(context.Context, string, time.Duration) (Acquisition, error)
	Retry(context.Context, AcquisitionID, string, time.Time, error) error
}

type sourceReader interface {
	Get(context.Context, source.ID) (source.Source, error)
	Checkpoint(context.Context, source.ID) (Checkpoint, error)
}

type batchCommitter interface {
	CommitBatch(context.Context, Acquisition, string, []Candidate, json.RawMessage) error
}

type Processor struct {
	queue     acquisitionQueue
	sources   sourceReader
	committer batchCommitter
	drivers   *Registry
}

func NewProcessor(
	queue acquisitionQueue,
	sources sourceReader,
	committer batchCommitter,
	drivers *Registry,
) *Processor {
	return &Processor{
		queue:     queue,
		sources:   sources,
		committer: committer,
		drivers:   drivers,
	}
}

func (processor *Processor) ProcessNext(ctx context.Context, owner string) error {
	acquisition, err := processor.queue.Claim(ctx, owner, defaultLease)
	if err != nil {
		return err
	}

	src, err := processor.sources.Get(ctx, acquisition.SourceID)
	if err != nil {
		return processor.retry(ctx, acquisition, owner, fmt.Errorf("load source: %w", err))
	}
	driver, err := processor.drivers.Driver(src.Kind)
	if err != nil {
		return processor.retry(ctx, acquisition, owner, err)
	}
	checkpoint, err := processor.sources.Checkpoint(ctx, acquisition.SourceID)
	if err != nil {
		return processor.retry(ctx, acquisition, owner, fmt.Errorf("load checkpoint: %w", err))
	}
	batch, err := driver.Acquire(ctx, AcquireRequest{
		Source:     src,
		Trigger:    acquisition.Trigger,
		Payload:    bytes.NewReader(acquisition.Payload),
		Checkpoint: checkpoint,
		Limits: Limits{
			MaxBytes:    4 << 20,
			MaxEntries:  1000,
			MaxPages:    20,
			MaxDuration: time.Minute,
		},
	})
	if err != nil {
		return processor.retry(ctx, acquisition, owner, err)
	}
	if err := processor.committer.CommitBatch(
		ctx,
		acquisition,
		owner,
		batch.Candidates,
		json.RawMessage(batch.NextCheckpoint),
	); err != nil {
		return processor.retry(ctx, acquisition, owner, err)
	}
	return nil
}

func (processor *Processor) retry(
	ctx context.Context,
	acquisition Acquisition,
	owner string,
	cause error,
) error {
	if err := processor.queue.Retry(
		ctx,
		acquisition.ID,
		owner,
		time.Now().Add(retryDelay),
		cause,
	); err != nil {
		return fmt.Errorf("%v; schedule retry: %w", cause, err)
	}
	return cause
}
