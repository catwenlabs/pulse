package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	xhtml "golang.org/x/net/html"

	"github.com/catwenlabs/pulse/internal/ai"
	"github.com/catwenlabs/pulse/internal/security"
)

const (
	defaultAIDigestLimit   = 1000
	defaultAIMaxActiveJobs = 4
	maxAIStoryEntries      = 8
	maxAIEntryText         = 20000
)

type AIStoreOptions struct {
	MaxActiveJobs int
}

type AIStore struct {
	pool          *pgxpool.Pool
	maxActiveJobs int
}

func NewAIStore(pool *pgxpool.Pool, options ...AIStoreOptions) *AIStore {
	maxActiveJobs := defaultAIMaxActiveJobs
	if len(options) > 0 && options[0].MaxActiveJobs > 0 {
		maxActiveJobs = options[0].MaxActiveJobs
	}
	return &AIStore{pool: pool, maxActiveJobs: maxActiveJobs}
}

func (store *AIStore) SnapshotStory(ctx context.Context, storyID string) (ai.StorySnapshot, error) {
	resolvedID, err := store.resolveStoryID(ctx, storyID)
	if err != nil {
		return ai.StorySnapshot{}, err
	}
	var snapshot ai.StorySnapshot
	var membership string
	err = store.pool.QueryRow(ctx, `
		SELECT
			coalesce(nullif(story.display_title, ''), nullif(representative.source_title, ''), ''),
			coalesce((
				SELECT string_agg(member.entry_id::text, ',' ORDER BY member.entry_id)
				FROM story_entries AS member
				WHERE member.story_id = story.id
			), '')
		FROM stories AS story
		JOIN entries AS representative ON representative.id = story.representative_entry_id
		WHERE story.id = $1
	`, resolvedID).Scan(&snapshot.Title, &membership)
	if errors.Is(err, pgx.ErrNoRows) {
		return ai.StorySnapshot{}, fmt.Errorf("%w: Story %s", ai.ErrNotFound, storyID)
	}
	if err != nil {
		return ai.StorySnapshot{}, fmt.Errorf("load Story summary snapshot: %w", err)
	}
	snapshot.StoryID = resolvedID
	snapshot.MembershipFingerprint = hashText(membership)
	rows, err := store.pool.Query(ctx, `
		SELECT
			entry.id, entry.source_title, entry.author,
			entry.summary, entry.content_html, entry.published_at
		FROM story_entries AS member
		JOIN entries AS entry ON entry.id = member.entry_id
		WHERE member.story_id = $1
		ORDER BY
			CASE WHEN entry.id = (SELECT representative_entry_id FROM stories WHERE id = $1) THEN 0 ELSE 1 END,
			member.joined_at ASC, entry.id ASC
		LIMIT $2
	`, resolvedID, maxAIStoryEntries)
	if err != nil {
		return ai.StorySnapshot{}, fmt.Errorf("load Story summary entries: %w", err)
	}
	defer rows.Close()
	for index := 0; rows.Next(); index++ {
		var item ai.StoryEntrySnapshot
		var summary, contentHTML string
		if err := rows.Scan(
			&item.EntryID,
			&item.SourceTitle,
			&item.Author,
			&summary,
			&contentHTML,
			&item.PublishedAt,
		); err != nil {
			return ai.StorySnapshot{}, fmt.Errorf("scan Story summary entry: %w", err)
		}
		item.Label = fmt.Sprintf("E%d", index+1)
		item.Title = strings.TrimSpace(item.SourceTitle)
		item.Summary = truncateText(plainText(summary), maxAIEntryText)
		item.Content = truncateText(plainText(contentHTML), maxAIEntryText)
		if item.Title == "" {
			item.Title = item.SourceTitle
		}
		snapshot.Entries = append(snapshot.Entries, item)
	}
	if err := rows.Err(); err != nil {
		return ai.StorySnapshot{}, fmt.Errorf("read Story summary entries: %w", err)
	}
	if len(snapshot.Entries) == 0 {
		return ai.StorySnapshot{}, fmt.Errorf("%w: Story %s has no Entries", ai.ErrNotFound, storyID)
	}
	if snapshot.Title == "" {
		snapshot.Title = snapshot.Entries[0].Title
	}
	snapshot.InputFingerprint, err = snapshotFingerprint(snapshot)
	if err != nil {
		return ai.StorySnapshot{}, fmt.Errorf("fingerprint Story summary snapshot: %w", err)
	}
	return snapshot, nil
}

func (store *AIStore) SnapshotUnreadStories(ctx context.Context, scope ai.DigestScope) ([]ai.DigestStorySnapshot, error) {
	limit := scope.MaxStories
	if limit <= 0 {
		limit = defaultAIDigestLimit
	}
	rows, err := store.pool.Query(ctx, `
		SELECT
			story.id,
			coalesce(nullif(story.display_title, ''), nullif(entry.source_title, ''), ''),
			entry.source_title,
			aggregate.entry_count,
			aggregate.source_count,
			story.sort_time
		FROM stories AS story
		JOIN entries AS entry ON entry.id = story.representative_entry_id
		JOIN LATERAL (
			SELECT
				count(*)::integer AS entry_count,
				count(DISTINCT member_entry.source_id)::integer AS source_count
			FROM story_entries AS member
			JOIN entries AS member_entry ON member_entry.id = member.entry_id
			WHERE member.story_id = story.id
		) AS aggregate ON true
		WHERE story.read_at IS NULL
		  AND story.hidden_at IS NULL
		  AND ($1::timestamptz IS NULL OR story.sort_time >= $1)
		  AND ($2::timestamptz IS NULL OR story.sort_time < $2)
		ORDER BY story.sort_time DESC, story.id DESC
		LIMIT $3
	`, scope.StartAt, scope.EndAt, limit)
	if err != nil {
		return nil, fmt.Errorf("snapshot unread Stories for Digest: %w", err)
	}
	defer rows.Close()
	items := make([]ai.DigestStorySnapshot, 0, limit)
	for index := 0; rows.Next(); index++ {
		var item ai.DigestStorySnapshot
		if err := rows.Scan(
			&item.StoryID,
			&item.Title,
			&item.SourceTitle,
			&item.EntryCount,
			&item.SourceCount,
			&item.SortTime,
		); err != nil {
			return nil, fmt.Errorf("scan unread Story for Digest: %w", err)
		}
		item.Label = fmt.Sprintf("S%d", index+1)
		item.InputFingerprint = digestStoryFingerprint(item)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read unread Stories for Digest: %w", err)
	}
	return items, nil
}

func (store *AIStore) GetStorySummary(ctx context.Context, storyID string) (ai.StorySummary, error) {
	resolvedID, err := store.resolveStoryID(ctx, storyID)
	if err != nil {
		return ai.StorySummary{}, err
	}
	var item ai.StorySummary
	var keyPointsJSON, sourcesJSON []byte
	err = store.pool.QueryRow(ctx, `
		SELECT story_id, status, overview, key_points, source_notes,
		       provider, model, prompt_version, input_fingerprint, error,
		       created_at, updated_at
		FROM story_ai_summaries
		WHERE story_id = $1
	`, resolvedID).Scan(
		&item.StoryID, &item.Status, &item.Overview, &keyPointsJSON, &sourcesJSON,
		&item.Provider, &item.Model, &item.PromptVersion, &item.InputFingerprint, &item.Error,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ai.StorySummary{}, ai.ErrNotFound
	}
	if err != nil {
		return ai.StorySummary{}, fmt.Errorf("get StorySummary: %w", err)
	}
	if err := decodeJSONValue(keyPointsJSON, &item.KeyPoints); err != nil {
		return ai.StorySummary{}, fmt.Errorf("decode StorySummary key points: %w", err)
	}
	if err := decodeJSONValue(sourcesJSON, &item.Sources); err != nil {
		return ai.StorySummary{}, fmt.Errorf("decode StorySummary sources: %w", err)
	}
	return item, nil
}

func (store *AIStore) EnqueueStorySummary(
	ctx context.Context,
	snapshot ai.StorySnapshot,
	metadata ai.ProviderMetadata,
) (ai.StorySummary, ai.JobReceipt, error) {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return ai.StorySummary{}, ai.JobReceipt{}, fmt.Errorf("encode StorySummary job: %w", err)
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return ai.StorySummary{}, ai.JobReceipt{}, fmt.Errorf("begin StorySummary enqueue: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := store.lockEnqueue(ctx, tx); err != nil {
		return ai.StorySummary{}, ai.JobReceipt{}, err
	}
	if _, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || $2, 0))
	`, ai.JobKindStorySummary, snapshot.StoryID); err != nil {
		return ai.StorySummary{}, ai.JobReceipt{}, fmt.Errorf("lock StorySummary enqueue: %w", err)
	}
	var existing ai.Job
	err = tx.QueryRow(ctx, `
		SELECT id, kind, target_id, status, attempts, available_at, requested_at,
		       lease_owner, lease_until, last_error, payload
		FROM ai_jobs
		WHERE kind = $1 AND target_id = $2 AND status IN ('pending', 'running', 'retry')
		ORDER BY requested_at DESC, id DESC
		LIMIT 1
		FOR UPDATE
	`, ai.JobKindStorySummary, snapshot.StoryID).Scan(
		&existing.ID, &existing.Kind, &existing.TargetID, &existing.Status, &existing.Attempts,
		&existing.AvailableAt, &existing.RequestedAt, &existing.LeaseOwner, &existing.LeaseUntil,
		&existing.LastError, &existing.Payload,
	)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return ai.StorySummary{}, ai.JobReceipt{}, fmt.Errorf("commit existing StorySummary job: %w", err)
		}
		item, getErr := store.GetStorySummary(ctx, snapshot.StoryID)
		if getErr != nil && !errors.Is(getErr, ai.ErrNotFound) {
			return ai.StorySummary{}, ai.JobReceipt{}, getErr
		}
		return item, jobReceipt(existing), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ai.StorySummary{}, ai.JobReceipt{}, fmt.Errorf("check StorySummary jobs: %w", err)
	}
	if err := store.ensureQueueCapacity(ctx, tx); err != nil {
		return ai.StorySummary{}, ai.JobReceipt{}, err
	}
	var summary ai.StorySummary
	if err := tx.QueryRow(ctx, `
		INSERT INTO story_ai_summaries (
			story_id, status, provider, model, prompt_version, input_fingerprint, error
		)
		VALUES ($1, 'queued', $2, $3, $4, $5, '')
		ON CONFLICT (story_id) DO UPDATE SET
			status = 'queued', overview = '', key_points = '[]'::jsonb,
			source_notes = '[]'::jsonb, provider = EXCLUDED.provider,
			model = EXCLUDED.model, prompt_version = EXCLUDED.prompt_version,
			input_fingerprint = EXCLUDED.input_fingerprint, error = '', updated_at = now()
		RETURNING story_id
	`, snapshot.StoryID, metadata.Name, metadata.Model, ai.StorySummaryPromptVersion, snapshot.InputFingerprint).Scan(&summary.StoryID); err != nil {
		return ai.StorySummary{}, ai.JobReceipt{}, fmt.Errorf("create StorySummary artifact: %w", err)
	}
	var job ai.Job
	if err := tx.QueryRow(ctx, `
		INSERT INTO ai_jobs (kind, target_id, payload)
		VALUES ($1, $2, $3)
		RETURNING id, kind, target_id, status, attempts, available_at, requested_at,
		          lease_owner, lease_until, last_error, payload
	`, ai.JobKindStorySummary, snapshot.StoryID, payload).Scan(
		&job.ID, &job.Kind, &job.TargetID, &job.Status, &job.Attempts, &job.AvailableAt,
		&job.RequestedAt, &job.LeaseOwner, &job.LeaseUntil, &job.LastError, &job.Payload,
	); err != nil {
		return ai.StorySummary{}, ai.JobReceipt{}, fmt.Errorf("create StorySummary job: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE story_ai_summaries SET job_id = $2 WHERE story_id = $1`, snapshot.StoryID, job.ID); err != nil {
		return ai.StorySummary{}, ai.JobReceipt{}, fmt.Errorf("link StorySummary job: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ai.StorySummary{}, ai.JobReceipt{}, fmt.Errorf("commit StorySummary enqueue: %w", err)
	}
	summary.Status = ai.StatusQueued
	summary.Provider = metadata.Name
	summary.Model = metadata.Model
	summary.PromptVersion = ai.StorySummaryPromptVersion
	summary.InputFingerprint = snapshot.InputFingerprint
	return summary, jobReceipt(job), nil
}

func (store *AIStore) GetDigest(ctx context.Context, digestID string) (ai.Digest, error) {
	item, err := store.getDigestMetadata(ctx, digestID)
	if err != nil {
		return ai.Digest{}, err
	}
	rows, err := store.pool.Query(ctx, `
		SELECT
			ref.label, ref.story_id, ref.story_title, ref.entry_count, ref.source_count,
			(EXISTS (SELECT 1 FROM stories WHERE id = ref.story_id)
			 OR EXISTS (SELECT 1 FROM story_aliases WHERE alias_id = ref.story_id))
		FROM ai_digest_stories AS ref
		WHERE ref.digest_id = $1
		ORDER BY ref.label
	`, digestID)
	if err != nil {
		return ai.Digest{}, fmt.Errorf("get Digest Story references: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var story ai.DigestStory
		if err := rows.Scan(&story.Label, &story.StoryID, &story.Title, &story.EntryCount, &story.SourceCount, &story.Available); err != nil {
			return ai.Digest{}, fmt.Errorf("scan Digest Story reference: %w", err)
		}
		item.Stories = append(item.Stories, story)
	}
	if err := rows.Err(); err != nil {
		return ai.Digest{}, fmt.Errorf("read Digest Story references: %w", err)
	}
	return item, nil
}

func hashText(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func (store *AIStore) lockEnqueue(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended('ai-active-jobs', 0))
	`); err != nil {
		return fmt.Errorf("lock AI queue capacity: %w", err)
	}
	return nil
}

func (store *AIStore) ensureQueueCapacity(ctx context.Context, tx pgx.Tx) error {
	var active int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)::integer
		FROM ai_jobs
		WHERE status IN ('pending', 'running', 'retry')
	`).Scan(&active); err != nil {
		return fmt.Errorf("count active AI jobs: %w", err)
	}
	if active >= store.maxActiveJobs {
		return &ai.QueueLimitError{Limit: store.maxActiveJobs}
	}
	return nil
}

func (store *AIStore) ListDigests(ctx context.Context, limit int) ([]ai.Digest, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := store.pool.Query(ctx, `
		SELECT id, mode, status, story_count, start_at, end_at, overview,
		       themes, priorities, omissions, provider, model, prompt_version,
		       input_fingerprint, error, created_at, updated_at
		FROM ai_digests
		ORDER BY created_at DESC, id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list Digests: %w", err)
	}
	defer rows.Close()
	items := make([]ai.Digest, 0, limit)
	for rows.Next() {
		item, err := scanDigest(rows)
		if err != nil {
			return nil, fmt.Errorf("scan Digest: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read Digests: %w", err)
	}
	return items, nil
}

func (store *AIStore) EnqueueDigest(
	ctx context.Context,
	scope ai.DigestScope,
	items []ai.DigestStorySnapshot,
	inputFingerprint string,
	metadata ai.ProviderMetadata,
) (ai.Digest, ai.JobReceipt, error) {
	payload, err := json.Marshal(struct {
		Scope       ai.DigestScope           `json:"scope"`
		Items       []ai.DigestStorySnapshot `json:"items"`
		Fingerprint string                   `json:"input_fingerprint"`
	}{Scope: scope, Items: items, Fingerprint: inputFingerprint})
	if err != nil {
		return ai.Digest{}, ai.JobReceipt{}, fmt.Errorf("encode Digest job: %w", err)
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return ai.Digest{}, ai.JobReceipt{}, fmt.Errorf("begin Digest enqueue: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := store.lockEnqueue(ctx, tx); err != nil {
		return ai.Digest{}, ai.JobReceipt{}, err
	}
	if err := store.ensureQueueCapacity(ctx, tx); err != nil {
		return ai.Digest{}, ai.JobReceipt{}, err
	}
	var digest ai.Digest
	if err := tx.QueryRow(ctx, `
		INSERT INTO ai_digests (
			mode, status, story_count, start_at, end_at, provider, model,
			prompt_version, input_fingerprint
		)
		VALUES ('catch_up', 'queued', $1, $2, $3, $4, $5, $6, $7)
		RETURNING id, mode, status, story_count, start_at, end_at, provider, model,
		          prompt_version, input_fingerprint, created_at, updated_at
	`, len(items), scope.StartAt, scope.EndAt, metadata.Name, metadata.Model,
		ai.DigestPromptVersion, inputFingerprint).Scan(
		&digest.ID, &digest.Mode, &digest.Status, &digest.StoryCount, &digest.StartAt,
		&digest.EndAt, &digest.Provider, &digest.Model, &digest.PromptVersion,
		&digest.InputFingerprint, &digest.CreatedAt, &digest.UpdatedAt,
	); err != nil {
		return ai.Digest{}, ai.JobReceipt{}, fmt.Errorf("create Digest artifact: %w", err)
	}
	for _, item := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ai_digest_stories (
				digest_id, label, story_id, story_title, entry_count, source_count, sort_time, input_fingerprint
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, digest.ID, item.Label, item.StoryID, item.Title, item.EntryCount, item.SourceCount, item.SortTime, item.InputFingerprint); err != nil {
			return ai.Digest{}, ai.JobReceipt{}, fmt.Errorf("save Digest Story snapshot: %w", err)
		}
	}
	var job ai.Job
	if err := tx.QueryRow(ctx, `
		INSERT INTO ai_jobs (kind, target_id, payload)
		VALUES ($1, $2, $3)
		RETURNING id, kind, target_id, status, attempts, available_at, requested_at,
		          lease_owner, lease_until, last_error, payload
	`, ai.JobKindDigest, digest.ID, payload).Scan(
		&job.ID, &job.Kind, &job.TargetID, &job.Status, &job.Attempts, &job.AvailableAt,
		&job.RequestedAt, &job.LeaseOwner, &job.LeaseUntil, &job.LastError, &job.Payload,
	); err != nil {
		return ai.Digest{}, ai.JobReceipt{}, fmt.Errorf("create Digest job: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE ai_digests SET job_id = $2 WHERE id = $1`, digest.ID, job.ID); err != nil {
		return ai.Digest{}, ai.JobReceipt{}, fmt.Errorf("link Digest job: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ai.Digest{}, ai.JobReceipt{}, fmt.Errorf("commit Digest enqueue: %w", err)
	}
	digest.Status = ai.StatusQueued
	return digest, jobReceipt(job), nil
}

func (store *AIStore) Claim(ctx context.Context, owner string, lease time.Duration) (ai.Job, error) {
	if strings.TrimSpace(owner) == "" {
		return ai.Job{}, fmt.Errorf("claim AI job: owner is required")
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return ai.Job{}, fmt.Errorf("begin AI job claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var job ai.Job
	err = tx.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id
			FROM ai_jobs
			WHERE available_at <= now()
			  AND (
				status IN ('pending', 'retry')
				OR (status = 'running' AND lease_until < now())
			  )
			ORDER BY available_at, requested_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE ai_jobs AS job
		SET status = 'running', started_at = coalesce(job.started_at, now()),
			lease_owner = $1, lease_until = now() + ($2 * interval '1 second'),
		    attempts = job.attempts + 1
		FROM candidate
		WHERE job.id = candidate.id
		RETURNING job.id, job.kind, job.target_id, job.status, job.attempts,
		          job.available_at, job.requested_at, job.lease_owner,
		          job.lease_until, job.last_error, job.payload
	`, owner, lease.Seconds()).Scan(
		&job.ID, &job.Kind, &job.TargetID, &job.Status, &job.Attempts, &job.AvailableAt,
		&job.RequestedAt, &job.LeaseOwner, &job.LeaseUntil, &job.LastError, &job.Payload,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ai.Job{}, ai.ErrNoJob
	}
	if err != nil {
		return ai.Job{}, fmt.Errorf("claim AI job: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE story_ai_summaries SET status = 'running', updated_at = now()
		WHERE job_id = $1
	`, job.ID); err != nil {
		return ai.Job{}, fmt.Errorf("mark StorySummary running: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ai_digests SET status = 'running', updated_at = now()
		WHERE job_id = $1
	`, job.ID); err != nil {
		return ai.Job{}, fmt.Errorf("mark Digest running: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ai.Job{}, fmt.Errorf("commit AI job claim: %w", err)
	}
	return job, nil
}

func (store *AIStore) CompleteStorySummary(
	ctx context.Context,
	job ai.Job,
	owner string,
	result ai.GeneratedStorySummary,
	metadata ai.ProviderMetadata,
) error {
	keyPoints, err := json.Marshal(result.KeyPoints)
	if err != nil {
		return fmt.Errorf("encode StorySummary key points: %w", err)
	}
	sources, err := json.Marshal(result.Sources)
	if err != nil {
		return fmt.Errorf("encode StorySummary sources: %w", err)
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin StorySummary completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE story_ai_summaries
		SET status = 'completed', overview = $2, key_points = $3, source_notes = $4,
		    provider = $5, model = $6, prompt_version = $7, error = '', updated_at = now()
		WHERE story_id = $1 AND job_id = $8
	`, job.TargetID, result.Overview, keyPoints, sources, metadata.Name, metadata.Model,
		ai.StorySummaryPromptVersion, job.ID)
	if err != nil {
		return fmt.Errorf("complete StorySummary artifact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("complete StorySummary: artifact is not owned by job %s", job.ID)
	}
	if err := completeJobTx(ctx, tx, job, owner, ai.JobCompleted, ""); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit StorySummary completion: %w", err)
	}
	return nil
}

func (store *AIStore) CompleteDigest(
	ctx context.Context,
	job ai.Job,
	owner string,
	result ai.GeneratedDigest,
	metadata ai.ProviderMetadata,
) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Digest completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	refs, err := loadDigestRefs(ctx, tx, job.TargetID)
	if err != nil {
		return err
	}
	themes, err := mapDigestThemes(result.Themes, refs)
	if err != nil {
		return err
	}
	priorities, err := mapDigestPriorities(result.Priorities, refs)
	if err != nil {
		return err
	}
	omissions, err := mapDigestOmissions(result.OmittedLabels, refs)
	if err != nil {
		return err
	}
	themesJSON, err := json.Marshal(themes)
	if err != nil {
		return fmt.Errorf("encode Digest themes: %w", err)
	}
	prioritiesJSON, err := json.Marshal(priorities)
	if err != nil {
		return fmt.Errorf("encode Digest priorities: %w", err)
	}
	omissionsJSON, err := json.Marshal(omissions)
	if err != nil {
		return fmt.Errorf("encode Digest omissions: %w", err)
	}
	jobStatus := ai.JobCompleted
	digestStatus := "completed"
	if len(omissions) > 0 {
		jobStatus = ai.JobPartial
		digestStatus = "partial"
	}
	tag, err := tx.Exec(ctx, `
		UPDATE ai_digests
		SET status = $2, overview = $3, themes = $4, priorities = $5, omissions = $6,
		    provider = $7, model = $8, prompt_version = $9, error = '', updated_at = now()
		WHERE id = $1 AND job_id = $10
	`, job.TargetID, digestStatus, result.Overview, themesJSON, prioritiesJSON, omissionsJSON,
		metadata.Name, metadata.Model, ai.DigestPromptVersion, job.ID)
	if err != nil {
		return fmt.Errorf("complete Digest artifact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("complete Digest: artifact is not owned by job %s", job.ID)
	}
	if err := completeJobTx(ctx, tx, job, owner, jobStatus, ""); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Digest completion: %w", err)
	}
	return nil
}

func (store *AIStore) Retry(ctx context.Context, job ai.Job, owner string, availableAt time.Time, cause error) error {
	return store.updateJobFailure(ctx, job, owner, "retry", availableAt, cause)
}

func (store *AIStore) Fail(ctx context.Context, job ai.Job, owner string, cause error) error {
	return store.updateJobFailure(ctx, job, owner, "dead", time.Time{}, cause)
}

func (store *AIStore) updateJobFailure(
	ctx context.Context,
	job ai.Job,
	owner string,
	status string,
	availableAt time.Time,
	cause error,
) error {
	message := ""
	if cause != nil {
		message = security.RedactDiagnosticText(cause.Error())
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin AI job failure update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if status == "retry" {
		tag, err := tx.Exec(ctx, `
				UPDATE ai_jobs
			SET status = 'retry', available_at = $3, lease_owner = '', lease_until = NULL, last_error = $4
			WHERE id = $1 AND status = 'running' AND lease_owner = $2
			`, job.ID, owner, availableAt, message)
		if err != nil {
			return fmt.Errorf("retry AI job: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("retry AI job: lease is not owned by %q", owner)
		}
	} else {
		tag, err := tx.Exec(ctx, `
			UPDATE ai_jobs
			SET status = 'dead', finished_at = now(), lease_owner = '', lease_until = NULL, last_error = $3
			WHERE id = $1 AND status = 'running' AND lease_owner = $2
			`, job.ID, owner, message)
		if err != nil {
			return fmt.Errorf("fail AI job: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("fail AI job: lease is not owned by %q", owner)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE story_ai_summaries SET status = $2, error = $3, updated_at = now()
		WHERE job_id = $1
	`, job.ID, mapArtifactFailureStatus(status), message); err != nil {
		return fmt.Errorf("update StorySummary failure: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ai_digests SET status = $2, error = $3, updated_at = now()
		WHERE job_id = $1
	`, job.ID, mapDigestFailureStatus(status), message); err != nil {
		return fmt.Errorf("update Digest failure: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit AI job failure update: %w", err)
	}
	return nil
}

func (store *AIStore) getDigestMetadata(ctx context.Context, digestID string) (ai.Digest, error) {
	row := store.pool.QueryRow(ctx, `
		SELECT id, mode, status, story_count, start_at, end_at, overview,
		       themes, priorities, omissions, provider, model, prompt_version,
		       input_fingerprint, error, created_at, updated_at
		FROM ai_digests
		WHERE id = $1
	`, digestID)
	item, err := scanDigest(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ai.Digest{}, fmt.Errorf("%w: Digest %s", ai.ErrNotFound, digestID)
	}
	if err != nil {
		return ai.Digest{}, fmt.Errorf("get Digest %s: %w", digestID, err)
	}
	return item, nil
}

func (store *AIStore) resolveStoryID(ctx context.Context, storyID string) (string, error) {
	var resolved string
	err := store.pool.QueryRow(ctx, `
		SELECT coalesce((SELECT canonical_story_id FROM story_aliases WHERE alias_id = $1::uuid), $1::uuid)::text
	`, storyID).Scan(&resolved)
	if err != nil {
		return "", fmt.Errorf("resolve Story %s: %w", storyID, err)
	}
	return resolved, nil
}

func scanDigest(row interface{ Scan(...any) error }) (ai.Digest, error) {
	var item ai.Digest
	var themesJSON, prioritiesJSON, omissionsJSON []byte
	err := row.Scan(
		&item.ID, &item.Mode, &item.Status, &item.StoryCount, &item.StartAt, &item.EndAt,
		&item.Overview, &themesJSON, &prioritiesJSON, &omissionsJSON,
		&item.Provider, &item.Model, &item.PromptVersion, &item.InputFingerprint,
		&item.Error, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return ai.Digest{}, err
	}
	if err := decodeJSONValue(themesJSON, &item.Themes); err != nil {
		return ai.Digest{}, fmt.Errorf("decode Digest themes: %w", err)
	}
	if err := decodeJSONValue(prioritiesJSON, &item.Priorities); err != nil {
		return ai.Digest{}, fmt.Errorf("decode Digest priorities: %w", err)
	}
	if err := decodeJSONValue(omissionsJSON, &item.Omissions); err != nil {
		return ai.Digest{}, fmt.Errorf("decode Digest omissions: %w", err)
	}
	return item, nil
}

func scanAIJob(row interface{ Scan(...any) error }) (ai.Job, error) {
	var job ai.Job
	err := row.Scan(
		&job.ID, &job.Kind, &job.TargetID, &job.Status, &job.Attempts,
		&job.AvailableAt, &job.RequestedAt, &job.LeaseOwner, &job.LeaseUntil,
		&job.LastError, &job.Payload,
	)
	return job, err
}

func jobReceipt(job ai.Job) ai.JobReceipt {
	status := job.Status
	return ai.JobReceipt{ID: job.ID, Kind: job.Kind, TargetID: job.TargetID, Status: status}
}

func completeJobTx(ctx context.Context, tx pgx.Tx, job ai.Job, owner string, status ai.JobStatus, message string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE ai_jobs
		SET status = $3, finished_at = now(), lease_owner = '', lease_until = NULL, last_error = $4
		WHERE id = $1 AND status = 'running' AND lease_owner = $2
	`, job.ID, owner, status, message)
	if err != nil {
		return fmt.Errorf("complete AI job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("complete AI job: lease is not owned by %q", owner)
	}
	return nil
}

type digestRef struct {
	Label       string
	StoryID     string
	Title       string
	EntryCount  int
	SourceCount int
}

func loadDigestRefs(ctx context.Context, tx pgx.Tx, digestID string) (map[string]digestRef, error) {
	rows, err := tx.Query(ctx, `
		SELECT label, story_id, story_title, entry_count, source_count
		FROM ai_digest_stories
		WHERE digest_id = $1
	`, digestID)
	if err != nil {
		return nil, fmt.Errorf("load Digest Story references: %w", err)
	}
	defer rows.Close()
	refs := make(map[string]digestRef)
	for rows.Next() {
		var ref digestRef
		if err := rows.Scan(&ref.Label, &ref.StoryID, &ref.Title, &ref.EntryCount, &ref.SourceCount); err != nil {
			return nil, fmt.Errorf("scan Digest Story reference: %w", err)
		}
		refs[ref.Label] = ref
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read Digest Story references: %w", err)
	}
	return refs, nil
}

func mapDigestThemes(input []ai.GeneratedDigestTheme, refs map[string]digestRef) ([]ai.DigestTheme, error) {
	result := make([]ai.DigestTheme, 0, len(input))
	for _, theme := range input {
		storyIDs, err := mapLabels(theme.StoryLabels, refs)
		if err != nil {
			return nil, err
		}
		result = append(result, ai.DigestTheme{Title: theme.Title, Summary: theme.Summary, StoryIDs: storyIDs})
	}
	return result, nil
}

func mapDigestPriorities(input []ai.GeneratedDigestPriority, refs map[string]digestRef) ([]ai.DigestPriority, error) {
	result := make([]ai.DigestPriority, 0, len(input))
	for _, priority := range input {
		storyIDs, err := mapLabels(priority.StoryLabels, refs)
		if err != nil {
			return nil, err
		}
		result = append(result, ai.DigestPriority{
			Rank: priority.Rank, Title: priority.Title, Reason: priority.Reason, StoryIDs: storyIDs,
		})
	}
	return result, nil
}

func mapDigestOmissions(labels []string, refs map[string]digestRef) ([]ai.DigestOmission, error) {
	result := make([]ai.DigestOmission, 0, len(labels))
	for _, label := range labels {
		ref, ok := refs[label]
		if !ok {
			return nil, fmt.Errorf("Digest references unknown Story label %q", label)
		}
		result = append(result, ai.DigestOmission{Label: ref.Label, StoryID: ref.StoryID, Title: ref.Title, Reason: "模型未在重点部分引用"})
	}
	return result, nil
}

func mapLabels(labels []string, refs map[string]digestRef) ([]string, error) {
	result := make([]string, 0, len(labels))
	for _, label := range labels {
		ref, ok := refs[label]
		if !ok {
			return nil, fmt.Errorf("Digest references unknown Story label %q", label)
		}
		result = append(result, ref.StoryID)
	}
	return result, nil
}

func decodeJSONValue(value []byte, target any) error {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	return json.Unmarshal(value, target)
}

func mapArtifactFailureStatus(status string) string {
	if status == "retry" {
		return "queued"
	}
	return "failed"
}

func mapDigestFailureStatus(status string) string {
	if status == "retry" {
		return "queued"
	}
	return "failed"
}

func snapshotFingerprint(snapshot ai.StorySnapshot) (string, error) {
	snapshot.InputFingerprint = ""
	body, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func digestStoryFingerprint(item ai.DigestStorySnapshot) string {
	item.InputFingerprint = ""
	body, err := json.Marshal(item)
	if err != nil {
		return ""
	}
	return hashText(string(body))
}

func plainText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	tokenizer := xhtml.NewTokenizer(strings.NewReader(value))
	var builder strings.Builder
	for {
		tokenType := tokenizer.Next()
		if tokenType == xhtml.ErrorToken {
			if errors.Is(tokenizer.Err(), io.EOF) {
				break
			}
			break
		}
		if tokenType != xhtml.TextToken {
			continue
		}
		text := strings.TrimSpace(stdhtml.UnescapeString(string(tokenizer.Raw())))
		if text == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte(' ')
		}
		builder.WriteString(text)
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func truncateText(value string, max int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
