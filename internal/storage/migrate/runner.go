package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const advisoryLockID int64 = 0x50554c5345

func Run(ctx context.Context, pool *pgxpool.Pool) error {
	plans, err := EmbeddedPlans()
	if err != nil {
		return err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", advisoryLockID)
	}()

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version bigint PRIMARY KEY,
			name text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("ensure migration table: %w", err)
	}

	for _, plan := range plans {
		var applied bool
		if err := conn.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)",
			plan.Version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %d: %w", plan.Version, err)
		}
		if applied {
			continue
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", plan.Version, err)
		}
		if _, err := tx.Exec(ctx, plan.SQL); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %d (%s): %w", plan.Version, plan.Name, err)
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (version, name) VALUES ($1, $2)",
			plan.Version,
			plan.Name,
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %d: %w", plan.Version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %d: %w", plan.Version, err)
		}
	}

	return nil
}
