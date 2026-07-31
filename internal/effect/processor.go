package effect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/rule"
)

type Store interface {
	Claim(context.Context, string, time.Duration) (rule.Effect, error)
	Succeed(context.Context, rule.Effect, string) error
	Retry(context.Context, string, string, time.Time, error) error
}

type Processor struct {
	store  Store
	client *http.Client
	now    func() time.Time
}

func NewProcessor(store Store, client *http.Client) *Processor {
	return &Processor{store: store, client: client, now: time.Now}
}

func (processor *Processor) ProcessNext(ctx context.Context, owner string) error {
	item, err := processor.store.Claim(ctx, owner, 30*time.Second)
	if errors.Is(err, rule.ErrNoEffect) {
		return ingestion.ErrNoAcquisition
	}
	if err != nil {
		return err
	}
	if item.Kind == rule.ActionNotification {
		return processor.store.Succeed(ctx, item, owner)
	}
	if item.Kind != rule.ActionWebhook {
		return processor.retry(ctx, item, owner, fmt.Errorf("unsupported effect kind %q", item.Kind))
	}
	if processor.client == nil {
		return processor.retry(ctx, item, owner, fmt.Errorf("webhook HTTP client is unavailable"))
	}
	payload, err := json.Marshal(map[string]any{
		"effect_id": item.ID, "rule_id": item.RuleID, "rule_version": item.RuleVersion,
		"entry_id": item.EntryID,
	})
	if err != nil {
		return processor.retry(ctx, item, owner, fmt.Errorf("encode webhook payload: %w", err))
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, item.Value, bytes.NewReader(payload))
	if err != nil {
		return processor.retry(ctx, item, owner, fmt.Errorf("create webhook request: %w", err))
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", item.EffectKey)
	response, err := processor.client.Do(request)
	if err != nil {
		return processor.retry(ctx, item, owner, fmt.Errorf("deliver webhook: %w", err))
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return processor.retry(ctx, item, owner, fmt.Errorf("deliver webhook: HTTP %d", response.StatusCode))
	}
	return processor.store.Succeed(ctx, item, owner)
}

func (processor *Processor) retry(
	ctx context.Context,
	item rule.Effect,
	owner string,
	cause error,
) error {
	delay := time.Second * time.Duration(1<<min(item.Attempts, 8))
	if err := processor.store.Retry(ctx, item.ID, owner, processor.now().Add(delay), cause); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}
