package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/security"
	"github.com/catwenlabs/pulse/internal/source"
)

type SourceStore struct {
	pool   *pgxpool.Pool
	cipher *security.CredentialCipher
}

func NewSourceStore(pool *pgxpool.Pool, ciphers ...*security.CredentialCipher) *SourceStore {
	store := &SourceStore{pool: pool}
	if len(ciphers) > 0 {
		store.cipher = ciphers[0]
	}
	return store
}

// sourceColumns is the column list every Source read returns. The trailing scalar
// subquery computes distinct Story unread counts: Source browsing still exposes
// Entry rows, but Reader state belongs to the owning Story.
const sourceColumns = `id, name, driver_kind, locator, normalized_locator, config,
	secret_ref, enabled, created_at, updated_at, archived_at,
	(SELECT COUNT(DISTINCT member.story_id)::int
	 FROM story_entries AS member
	 JOIN entries AS unread_entry ON unread_entry.id = member.entry_id
	 JOIN stories AS unread_story ON unread_story.id = member.story_id
	 WHERE unread_entry.source_id = sources.id
	   AND unread_story.read_at IS NULL
	   AND unread_story.hidden_at IS NULL) AS unread_count`

func (store *SourceStore) Create(ctx context.Context, spec source.Spec) (source.Source, error) {
	validated, err := spec.Validate()
	if err != nil {
		return source.Source{}, err
	}

	config := validated.Config
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	config, err = store.cipher.Protect(config)
	if err != nil {
		return source.Source{}, fmt.Errorf("protect source credentials: %w", err)
	}

	row := store.pool.QueryRow(ctx, `
		INSERT INTO sources (
			name, driver_kind, locator, normalized_locator, config, secret_ref
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING
		`+sourceColumns+`
	`,
		validated.Name,
		validated.Kind,
		validated.Locator,
		validated.NormalizedLocator,
		config,
		validated.SecretRef,
	)

	created, err := store.scanSource(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return source.Source{}, fmt.Errorf("%w: %s %s", source.ErrDuplicate, validated.Kind, validated.NormalizedLocator)
		}
		return source.Source{}, fmt.Errorf("create source: %w", err)
	}
	return created, nil
}

func (store *SourceStore) Get(ctx context.Context, id source.ID) (source.Source, error) {
	row := store.pool.QueryRow(ctx, `
		SELECT
		`+sourceColumns+`
		FROM sources
		WHERE id = $1
	`, id)

	got, err := store.scanSource(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return source.Source{}, fmt.Errorf("%w: %s", source.ErrNotFound, id)
	}
	if err != nil {
		return source.Source{}, fmt.Errorf("get source %s: %w", id, err)
	}
	return got, nil
}

func (store *SourceStore) List(ctx context.Context) ([]source.Source, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT
		`+sourceColumns+`
		FROM sources
		WHERE archived_at IS NULL
		ORDER BY name, id
	`)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	defer rows.Close()

	var result []source.Source
	for rows.Next() {
		item, err := store.scanSource(rows)
		if err != nil {
			return nil, fmt.Errorf("scan source: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	return result, nil
}

func (store *SourceStore) Checkpoint(ctx context.Context, id source.ID) (ingestion.Checkpoint, error) {
	var checkpoint json.RawMessage
	err := store.pool.QueryRow(ctx, `
		SELECT checkpoint
		FROM source_checkpoints
		WHERE source_id = $1
	`, id).Scan(&checkpoint)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get source %s checkpoint: %w", id, err)
	}
	return ingestion.Checkpoint(checkpoint), nil
}

func (store *SourceStore) SetEnabled(ctx context.Context, id source.ID, enabled bool) error {
	tag, err := store.pool.Exec(ctx, `
		UPDATE sources
		SET enabled = $2, updated_at = now()
		WHERE id = $1 AND archived_at IS NULL
	`, id, enabled)
	if err != nil {
		return fmt.Errorf("set source %s enabled: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", source.ErrNotFound, id)
	}
	return nil
}

func (store *SourceStore) Update(ctx context.Context, id source.ID, spec source.Spec) (source.Source, error) {
	validated, err := spec.Validate()
	if err != nil {
		return source.Source{}, err
	}

	row := store.pool.QueryRow(ctx, `
		UPDATE sources
		SET
			name = $2,
			locator = $3,
			normalized_locator = $4,
			updated_at = now()
		WHERE id = $1 AND archived_at IS NULL
		RETURNING
		`+sourceColumns+`
	`, id, validated.Name, validated.Locator, validated.NormalizedLocator)

	updated, err := store.scanSource(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return source.Source{}, fmt.Errorf("%w: %s", source.ErrNotFound, id)
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return source.Source{}, fmt.Errorf("%w: %s %s", source.ErrDuplicate, validated.Kind, validated.NormalizedLocator)
		}
		return source.Source{}, fmt.Errorf("update source %s: %w", id, err)
	}
	return updated, nil
}

func (store *SourceStore) SetSecretRef(ctx context.Context, id source.ID, secretRef string) error {
	tag, err := store.pool.Exec(ctx, `
		UPDATE sources
		SET secret_ref = $2, updated_at = now()
		WHERE id = $1 AND archived_at IS NULL
	`, id, secretRef)
	if err != nil {
		return fmt.Errorf("set source %s secret: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", source.ErrNotFound, id)
	}
	return nil
}

func (store *SourceStore) Health(ctx context.Context, id source.ID) (source.Health, error) {
	var health source.Health
	health.SourceID = id
	err := store.pool.QueryRow(ctx, `
		WITH latest AS (
			SELECT
				status, requested_at, finished_at, started_at,
				candidate_count, new_count, updated_count, last_error
			FROM acquisitions
			WHERE source_id = $1
			ORDER BY requested_at DESC, id DESC
			LIMIT 1
		), last_success AS (
			SELECT max(requested_at) AS requested_at
			FROM acquisitions
			WHERE source_id = $1 AND status = 'succeeded'
		)
		SELECT
			COALESCE(latest.status, 'never'),
			latest.requested_at,
			latest.finished_at,
			CASE
				WHEN sources.enabled AND sources.driver_kind IN ('rss', 'json-api', 'html')
				THEN COALESCE(latest.requested_at, now())
					+ COALESCE(NULLIF(sources.config->>'schedule_minutes', '')::integer, 30) * interval '1 minute'
				ELSE NULL
			END,
			COALESCE(
				(extract(epoch FROM (latest.finished_at - latest.started_at)) * 1000)::bigint,
				0
			),
			COALESCE(latest.candidate_count, 0),
			COALESCE(latest.new_count, 0),
			COALESCE(latest.updated_count, 0),
			(
				SELECT count(*)::integer
				FROM acquisitions, last_success
				WHERE source_id = $1
				  AND status IN ('retry', 'dead')
				  AND (last_success.requested_at IS NULL OR acquisitions.requested_at > last_success.requested_at)
			),
			COALESCE(latest.last_error, '')
		FROM sources
		LEFT JOIN latest ON true
		WHERE sources.id = $1
	`, id).Scan(
		&health.Status,
		&health.LastRequestedAt,
		&health.LastFinishedAt,
		&health.NextScheduledAt,
		&health.DurationMilliseconds,
		&health.CandidateCount,
		&health.NewCount,
		&health.UpdatedCount,
		&health.ConsecutiveFailures,
		&health.LastError,
	)
	if err != nil {
		return source.Health{}, fmt.Errorf("get source %s health: %w", id, err)
	}
	return health, nil
}

func (store *SourceStore) Archive(ctx context.Context, id source.ID) error {
	tag, err := store.pool.Exec(ctx, `
		UPDATE sources
		SET enabled = false, archived_at = COALESCE(archived_at, now()), updated_at = now()
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("archive source %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", source.ErrNotFound, id)
	}
	return nil
}

type sourceRow interface {
	Scan(...any) error
}

func (store *SourceStore) scanSource(row sourceRow) (source.Source, error) {
	var result source.Source
	err := row.Scan(
		&result.ID,
		&result.Name,
		&result.Kind,
		&result.Locator,
		&result.NormalizedLocator,
		&result.Config,
		&result.SecretRef,
		&result.Enabled,
		&result.CreatedAt,
		&result.UpdatedAt,
		&result.ArchivedAt,
		&result.UnreadCount,
	)
	if err == nil {
		result.Config, err = store.cipher.Reveal(result.Config)
	}
	return result, err
}
