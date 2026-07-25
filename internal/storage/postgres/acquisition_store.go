package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/security"
)

type AcquisitionStore struct {
	pool *pgxpool.Pool
}

func NewAcquisitionStore(pool *pgxpool.Pool) *AcquisitionStore {
	return &AcquisitionStore{pool: pool}
}

func (store *AcquisitionStore) Enqueue(
	ctx context.Context,
	request ingestion.EnqueueRequest,
) (ingestion.Acquisition, error) {
	if strings.TrimSpace(request.IdempotencyKey) == "" {
		return ingestion.Acquisition{}, fmt.Errorf("enqueue acquisition: idempotency key is required")
	}
	payload := request.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	row := store.pool.QueryRow(ctx, `
		INSERT INTO acquisitions (
			source_id, trigger, payload, idempotency_key, priority
		)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (source_id, idempotency_key)
		DO UPDATE SET idempotency_key = EXCLUDED.idempotency_key
		RETURNING
			id, source_id, trigger, payload, idempotency_key, status,
			priority, attempts, available_at, requested_at, lease_owner,
			lease_until, last_error
	`,
		request.SourceID,
		request.Trigger,
		payload,
		request.IdempotencyKey,
		request.Priority,
	)
	acquisition, err := scanAcquisition(row)
	if err != nil {
		return ingestion.Acquisition{}, fmt.Errorf("enqueue acquisition: %w", err)
	}
	return acquisition, nil
}

func (store *AcquisitionStore) Claim(
	ctx context.Context,
	owner string,
	leaseDuration time.Duration,
) (ingestion.Acquisition, error) {
	if strings.TrimSpace(owner) == "" {
		return ingestion.Acquisition{}, fmt.Errorf("claim acquisition: owner is required")
	}

	row := store.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id
			FROM acquisitions
			WHERE available_at <= now()
			  AND (
				status IN ('pending', 'retry')
				OR (status = 'running' AND lease_until < now())
			  )
			ORDER BY priority DESC, requested_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE acquisitions AS acquisition
		SET
			status = 'running',
			started_at = COALESCE(acquisition.started_at, now()),
			lease_owner = $1,
			lease_until = now() + $2::interval,
			attempts = acquisition.attempts + 1
		FROM candidate
		WHERE acquisition.id = candidate.id
		RETURNING
			acquisition.id, acquisition.source_id, acquisition.trigger,
			acquisition.payload, acquisition.idempotency_key, acquisition.status,
			acquisition.priority, acquisition.attempts, acquisition.available_at,
			acquisition.requested_at, acquisition.lease_owner,
			acquisition.lease_until, acquisition.last_error
	`, owner, leaseDuration.String())

	acquisition, err := scanAcquisition(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ingestion.Acquisition{}, ingestion.ErrNoAcquisition
	}
	if err != nil {
		return ingestion.Acquisition{}, fmt.Errorf("claim acquisition: %w", err)
	}
	return acquisition, nil
}

func (store *AcquisitionStore) Complete(
	ctx context.Context,
	id ingestion.AcquisitionID,
	owner string,
) error {
	tag, err := store.pool.Exec(ctx, `
		UPDATE acquisitions
		SET status = 'succeeded', finished_at = now(), lease_owner = '', lease_until = NULL, last_error = ''
		WHERE id = $1 AND status = 'running' AND lease_owner = $2
	`, id, owner)
	if err != nil {
		return fmt.Errorf("complete acquisition %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("complete acquisition %s: lease is not owned by %q", id, owner)
	}
	return nil
}

func (store *AcquisitionStore) Retry(
	ctx context.Context,
	id ingestion.AcquisitionID,
	owner string,
	availableAt time.Time,
	cause error,
) error {
	message := ""
	if cause != nil {
		message = security.RedactDiagnosticText(cause.Error())
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin retry acquisition %s: %w", id, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE acquisitions
		SET
			status = 'retry',
			finished_at = now(),
			available_at = $3,
			lease_owner = '',
			lease_until = NULL,
			last_error = $4
		WHERE id = $1 AND status = 'running' AND lease_owner = $2
	`, id, owner, availableAt, message)
	if err != nil {
		return fmt.Errorf("retry acquisition %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("retry acquisition %s: lease is not owned by %q", id, owner)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO diagnostic_snapshots (
			source_id, acquisition_id, status, summary
		)
		SELECT source_id, id, 'failure', left($2, 2000)
		FROM acquisitions
		WHERE id = $1
	`, id, message); err != nil {
		return fmt.Errorf("save acquisition diagnostic: %w", err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM diagnostic_snapshots WHERE expires_at < now()"); err != nil {
		return fmt.Errorf("prune acquisition diagnostics: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit retry acquisition %s: %w", id, err)
	}
	return nil
}

func scanAcquisition(row sourceRow) (ingestion.Acquisition, error) {
	var result ingestion.Acquisition
	err := row.Scan(
		&result.ID,
		&result.SourceID,
		&result.Trigger,
		&result.Payload,
		&result.IdempotencyKey,
		&result.Status,
		&result.Priority,
		&result.Attempts,
		&result.AvailableAt,
		&result.RequestedAt,
		&result.LeaseOwner,
		&result.LeaseUntil,
		&result.LastError,
	)
	return result, err
}
