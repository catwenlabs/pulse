package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wenpengfei/pulse/internal/entry"
	"github.com/wenpengfei/pulse/internal/rule"
)

type EffectStore struct {
	pool *pgxpool.Pool
}

func NewEffectStore(pool *pgxpool.Pool) *EffectStore {
	return &EffectStore{pool: pool}
}

func (store *EffectStore) Enqueue(
	ctx context.Context,
	definition rule.Rule,
	entryID entry.ID,
	action rule.EvaluatedAction,
) (rule.Effect, error) {
	if action.EffectKey == "" {
		return rule.Effect{}, fmt.Errorf("enqueue effect: effect key is required")
	}
	payload, err := json.Marshal(map[string]string{"value": action.Value})
	if err != nil {
		return rule.Effect{}, fmt.Errorf("encode effect payload: %w", err)
	}
	row := store.pool.QueryRow(ctx, `
		INSERT INTO effects (
			effect_key, rule_id, rule_version, entry_id, kind, payload
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (effect_key)
		DO UPDATE SET effect_key = EXCLUDED.effect_key
		RETURNING
			id, effect_key, rule_id, rule_version, entry_id, kind,
			payload->>'value', status, attempts
	`, action.EffectKey, definition.ID, definition.Version, entryID, action.Kind, payload)
	effect, err := scanEffect(row)
	if err != nil {
		return rule.Effect{}, fmt.Errorf("enqueue effect: %w", err)
	}
	return effect, nil
}

func (store *EffectStore) Claim(
	ctx context.Context,
	owner string,
	leaseDuration time.Duration,
) (rule.Effect, error) {
	row := store.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id
			FROM effects
			WHERE available_at <= now()
			  AND (
				status IN ('pending', 'retry')
				OR (status = 'running' AND lease_until < now())
			  )
			ORDER BY available_at, created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE effects AS effect
		SET
			status = 'running',
			lease_owner = $1,
			lease_until = now() + $2::interval,
			attempts = effect.attempts + 1
		FROM candidate
		WHERE effect.id = candidate.id
		RETURNING
			effect.id, effect.effect_key, effect.rule_id, effect.rule_version,
			effect.entry_id, effect.kind, effect.payload->>'value',
			effect.status, effect.attempts
	`, owner, leaseDuration.String())
	effect, err := scanEffect(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return rule.Effect{}, rule.ErrNoEffect
	}
	if err != nil {
		return rule.Effect{}, fmt.Errorf("claim effect: %w", err)
	}
	return effect, nil
}

func (store *EffectStore) Succeed(ctx context.Context, effect rule.Effect, owner string) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin effect completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if effect.Kind == rule.ActionNotification {
		if _, err := tx.Exec(ctx, `
			INSERT INTO notifications (effect_id, entry_id, message)
			VALUES ($1, $2, $3)
			ON CONFLICT (effect_id) DO NOTHING
		`, effect.ID, effect.EntryID, effect.Value); err != nil {
			return fmt.Errorf("create notification: %w", err)
		}
	}
	tag, err := tx.Exec(ctx, `
		UPDATE effects
		SET
			status = 'succeeded',
			delivered_at = now(),
			lease_owner = '',
			lease_until = NULL,
			last_error = ''
		WHERE id = $1 AND status = 'running' AND lease_owner = $2
	`, effect.ID, owner)
	if err != nil {
		return fmt.Errorf("complete effect: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("complete effect: lease is not owned by %q", owner)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit effect completion: %w", err)
	}
	return nil
}

func (store *EffectStore) Retry(
	ctx context.Context,
	effectID string,
	owner string,
	availableAt time.Time,
	cause error,
) error {
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	tag, err := store.pool.Exec(ctx, `
		UPDATE effects
		SET
			status = 'retry',
			available_at = $3,
			lease_owner = '',
			lease_until = NULL,
			last_error = $4
		WHERE id = $1 AND status = 'running' AND lease_owner = $2
	`, effectID, owner, availableAt, message)
	if err != nil {
		return fmt.Errorf("retry effect: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("retry effect: lease is not owned by %q", owner)
	}
	return nil
}

func scanEffect(row entryRow) (rule.Effect, error) {
	var effect rule.Effect
	err := row.Scan(
		&effect.ID,
		&effect.EffectKey,
		&effect.RuleID,
		&effect.RuleVersion,
		&effect.EntryID,
		&effect.Kind,
		&effect.Value,
		&effect.Status,
		&effect.Attempts,
	)
	return effect, err
}
