package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/catwenlabs/pulse/internal/ingestion"
)

const (
	defaultAILease = 2 * time.Minute
	maxAIAttempts  = 3
)

type ProcessorOptions struct {
	Lease time.Duration
}

type Processor struct {
	store    Store
	provider Provider
	lease    time.Duration
	now      func() time.Time
}

func NewProcessor(store Store, provider Provider, options ...ProcessorOptions) *Processor {
	lease := defaultAILease
	if len(options) > 0 && options[0].Lease > 0 {
		lease = options[0].Lease
	}
	return &Processor{store: store, provider: provider, lease: lease, now: time.Now}
}

func (processor *Processor) ProcessNext(ctx context.Context, owner string) error {
	if processor.store == nil || processor.provider == nil {
		return ingestion.ErrNoAcquisition
	}
	job, err := processor.store.Claim(ctx, owner, processor.lease)
	if errors.Is(err, ErrNoJob) {
		return ingestion.ErrNoAcquisition
	}
	if err != nil {
		return err
	}

	var processErr error
	switch job.Kind {
	case JobKindStorySummary:
		processErr = processor.processStorySummary(ctx, job)
	case JobKindDigest:
		processErr = processor.processDigest(ctx, job)
	default:
		processErr = fmt.Errorf("unsupported AI job kind %q", job.Kind)
	}
	if processErr == nil {
		return nil
	}
	if job.Attempts >= maxAIAttempts || !shouldRetry(processErr) {
		if err := processor.store.Fail(ctx, job, owner, processErr); err != nil {
			return errors.Join(processErr, err)
		}
		return processErr
	}
	if err := processor.store.Retry(
		ctx,
		job,
		owner,
		processor.now().Add(retryDelay(job.Attempts)),
		processErr,
	); err != nil {
		return errors.Join(processErr, err)
	}
	return processErr
}

func shouldRetry(err error) bool {
	var classified interface{ Retryable() bool }
	if errors.As(err, &classified) {
		return classified.Retryable()
	}
	return true
}

func (processor *Processor) processStorySummary(ctx context.Context, job Job) error {
	var snapshot StorySnapshot
	if err := json.Unmarshal(job.Payload, &snapshot); err != nil {
		return fmt.Errorf("decode StorySummary job payload: %w", err)
	}
	response, err := processor.provider.Generate(ctx, storySummaryRequest(snapshot))
	if err != nil {
		return fmt.Errorf("generate StorySummary: %w", err)
	}
	result, err := parseStorySummary(response.Content, snapshot)
	if err != nil {
		return err
	}
	return processor.store.CompleteStorySummary(
		ctx,
		job,
		job.LeaseOwner,
		result,
		processor.provider.Metadata(),
	)
}

func (processor *Processor) processDigest(ctx context.Context, job Job) error {
	var payload struct {
		Scope       DigestScope           `json:"scope"`
		Items       []DigestStorySnapshot `json:"items"`
		Fingerprint string                `json:"input_fingerprint"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("decode Digest job payload: %w", err)
	}
	response, err := processor.provider.Generate(ctx, digestRequest(payload.Items))
	if err != nil {
		return fmt.Errorf("generate Digest: %w", err)
	}
	result, err := parseDigest(response.Content, payload.Items)
	if err != nil {
		return err
	}
	return processor.store.CompleteDigest(
		ctx,
		job,
		job.LeaseOwner,
		result,
		processor.provider.Metadata(),
	)
}
