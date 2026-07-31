package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wenpengfei/pulse/internal/entry"
	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/source"
	"github.com/wenpengfei/pulse/internal/story"
)

type EntryStore struct {
	pool *pgxpool.Pool
}

func NewEntryStore(pool *pgxpool.Pool) *EntryStore {
	return &EntryStore{pool: pool}
}

func (store *EntryStore) List(ctx context.Context, limit int) ([]entry.Entry, error) {
	return store.Search(ctx, entry.Query{Limit: limit})
}

func (store *EntryStore) Search(ctx context.Context, query entry.Query) ([]entry.Entry, error) {
	if query.Limit <= 0 || query.Limit > 200 {
		query.Limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	rows, err := store.pool.Query(ctx, `
		SELECT
			entry.id, entry.source_id, entry.identity_key, entry.external_id, entry.canonical_url,
			entry.source_title, entry.display_title, entry.author, entry.summary, entry.content_html,
			entry.published_at, entry.discovered_at, entry.read_at, entry.starred_at,
			entry.hidden_at, entry.later_at, entry.note,
			to_jsonb(entry_annotation) - 'entry_id' - 'imported_at'
		FROM entries AS entry
		JOIN sources AS source ON source.id = entry.source_id
		LEFT JOIN entry_annotations AS entry_annotation ON entry_annotation.entry_id = entry.id
		WHERE
			($2 = '' OR (
				to_tsvector(
					'simple',
					coalesce(display_title, '') || ' ' ||
					coalesce(source_title, '') || ' ' ||
					coalesce(author, '') || ' ' ||
					coalesce(summary, '') || ' ' ||
					coalesce(content_html, '') || ' ' ||
					coalesce(note, '') || ' ' ||
					coalesce(source.name, '') || ' ' ||
					coalesce(entry_annotation.book_title, '') || ' ' ||
					coalesce(entry_annotation.book_author, '') || ' ' ||
					coalesce(entry_annotation.chapter, '') || ' ' ||
					coalesce(entry_annotation.annotation_note, '')
				) @@ plainto_tsquery('simple', $2)
				OR display_title ILIKE '%' || $2 || '%'
				OR source_title ILIKE '%' || $2 || '%'
				OR author ILIKE '%' || $2 || '%'
				OR source.name ILIKE '%' || $2 || '%'
				OR lower(
					coalesce(summary, '') || ' ' ||
					coalesce(content_html, '') || ' ' ||
					coalesce(note, '') || ' ' ||
					coalesce(entry_annotation.book_title, '') || ' ' ||
					coalesce(entry_annotation.book_author, '') || ' ' ||
					coalesce(entry_annotation.chapter, '') || ' ' ||
					coalesce(entry_annotation.annotation_note, '')
				) LIKE '%' || lower($2) || '%'
				OR word_similarity(lower($2), lower(coalesce(display_title, ''))) >= 0.45
				OR word_similarity(lower($2), lower(coalesce(source_title, ''))) >= 0.45
				OR word_similarity(lower($2), lower(coalesce(author, ''))) >= 0.45
				OR word_similarity(lower($2), lower(coalesce(source.name, ''))) >= 0.45
				OR pulse_fuzzy_contains(coalesce(display_title, ''), $2)
				OR pulse_fuzzy_contains(coalesce(source_title, ''), $2)
				OR pulse_fuzzy_contains(coalesce(author, ''), $2)
				OR pulse_fuzzy_contains(coalesce(source.name, ''), $2)
			))
			AND (
				$3 = ''
				OR ($3 = 'inbox' AND hidden_at IS NULL)
				OR ($3 = 'unread' AND hidden_at IS NULL AND read_at IS NULL)
				OR ($3 = 'starred' AND starred_at IS NOT NULL)
				OR ($3 = 'later' AND later_at IS NOT NULL)
				OR ($3 = 'hidden' AND hidden_at IS NOT NULL)
			)
			AND (
				$4 = ''
				OR EXISTS (
					SELECT 1
					FROM entry_tags
					JOIN tags ON tags.id = entry_tags.tag_id
					WHERE entry_tags.entry_id = entry.id
					  AND tags.normalized_name = lower($4)
				)
			)
			AND ($5 = '' OR entry.source_id = $5::uuid)
		ORDER BY discovered_at DESC, id DESC
		LIMIT $1
		OFFSET $6
	`,
		query.Limit,
		strings.TrimSpace(query.Search),
		strings.TrimSpace(query.State),
		strings.TrimSpace(query.Tag),
		query.SourceID,
		query.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("search entries: %w", err)
	}
	defer rows.Close()

	result := make([]entry.Entry, 0, query.Limit)
	for rows.Next() {
		item, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list entries: %w", err)
	}
	return result, nil
}

func (store *EntryStore) Get(ctx context.Context, id entry.ID) (entry.Entry, error) {
	row := store.pool.QueryRow(ctx, `
		SELECT
			entry.id, entry.source_id, entry.identity_key, entry.external_id, entry.canonical_url,
			entry.source_title, entry.display_title, entry.author, entry.summary, entry.content_html,
			entry.published_at, entry.discovered_at, entry.read_at, entry.starred_at,
			entry.hidden_at, entry.later_at, entry.note,
			to_jsonb(entry_annotation) - 'entry_id' - 'imported_at'
		FROM entries AS entry
		LEFT JOIN entry_annotations AS entry_annotation ON entry_annotation.entry_id = entry.id
		WHERE entry.id = $1
	`, id)
	item, err := scanEntry(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return entry.Entry{}, fmt.Errorf("%w: %s", entry.ErrNotFound, id)
	}
	if err != nil {
		return entry.Entry{}, fmt.Errorf("get entry %s: %w", id, err)
	}
	return item, nil
}

func (store *EntryStore) Update(ctx context.Context, id entry.ID, patch entry.Patch) (entry.Entry, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return entry.Entry{}, fmt.Errorf("begin entry update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row := tx.QueryRow(ctx, `
		UPDATE entries
		SET
			read_at = CASE WHEN $2 THEN CASE WHEN $3 THEN COALESCE(read_at, now()) ELSE NULL END ELSE read_at END,
			starred_at = CASE WHEN $4 THEN CASE WHEN $5 THEN COALESCE(starred_at, now()) ELSE NULL END ELSE starred_at END,
			hidden_at = CASE WHEN $6 THEN CASE WHEN $7 THEN COALESCE(hidden_at, now()) ELSE NULL END ELSE hidden_at END,
			later_at = CASE WHEN $8 THEN CASE WHEN $9 THEN COALESCE(later_at, now()) ELSE NULL END ELSE later_at END,
			display_title = CASE WHEN $10 THEN $11 ELSE display_title END,
			note = CASE WHEN $12 THEN $13 ELSE note END
		WHERE id = $1
			RETURNING
				id, source_id, identity_key, external_id, canonical_url,
				source_title, display_title, author, summary, content_html,
				published_at, discovered_at, read_at, starred_at, hidden_at, later_at, note,
				NULL::jsonb
	`,
		id,
		patch.Read != nil, boolValue(patch.Read),
		patch.Starred != nil, boolValue(patch.Starred),
		patch.Hidden != nil, boolValue(patch.Hidden),
		patch.Later != nil, boolValue(patch.Later),
		patch.DisplayTitle != nil, stringValue(patch.DisplayTitle),
		patch.Note != nil, stringValue(patch.Note),
	)
	_, err = scanEntry(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return entry.Entry{}, fmt.Errorf("%w: %s", entry.ErrNotFound, id)
	}
	if err != nil {
		return entry.Entry{}, fmt.Errorf("update entry %s: %w", id, err)
	}
	if err := applyEnabledRulesTx(ctx, tx, id); err != nil {
		return entry.Entry{}, fmt.Errorf("re-evaluate rules for entry %s: %w", id, err)
	}
	if patch.Read != nil || patch.Starred != nil || patch.Hidden != nil || patch.Later != nil {
		if _, err := tx.Exec(ctx, `
			WITH target_story AS (
				UPDATE stories AS story
				SET
					read_at = target.read_at,
					starred_at = target.starred_at,
					hidden_at = target.hidden_at,
					later_at = target.later_at,
					updated_at = now()
				FROM story_entries AS membership
				JOIN entries AS target ON target.id = membership.entry_id
				WHERE membership.entry_id = $1 AND story.id = membership.story_id
				RETURNING story.id, story.read_at, story.starred_at, story.hidden_at, story.later_at
			)
			UPDATE entries AS member
			SET
				read_at = target_story.read_at,
				starred_at = target_story.starred_at,
				hidden_at = target_story.hidden_at,
				later_at = target_story.later_at
			FROM story_entries AS membership, target_story
			WHERE membership.story_id = target_story.id
			  AND membership.entry_id = member.id
		`, id); err != nil {
			return entry.Entry{}, fmt.Errorf("synchronize Story state for entry %s: %w", id, err)
		}
	}
	item, err := scanEntry(tx.QueryRow(ctx, `
		SELECT
			entry.id, entry.source_id, entry.identity_key, entry.external_id, entry.canonical_url,
			entry.source_title, entry.display_title, entry.author, entry.summary, entry.content_html,
			entry.published_at, entry.discovered_at, entry.read_at, entry.starred_at,
			entry.hidden_at, entry.later_at, entry.note,
			to_jsonb(entry_annotation) - 'entry_id' - 'imported_at'
		FROM entries AS entry
		LEFT JOIN entry_annotations AS entry_annotation ON entry_annotation.entry_id = entry.id
		WHERE entry.id = $1
	`, id))
	if err != nil {
		return entry.Entry{}, fmt.Errorf("reload entry %s: %w", id, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return entry.Entry{}, fmt.Errorf("commit entry update %s: %w", id, err)
	}
	return item, nil
}

func (store *EntryStore) MarkRead(ctx context.Context, sourceID source.ID) (int64, error) {
	return NewStoryStore(store.pool).MarkRead(ctx, string(sourceID))
}

func (store *EntryStore) AddTag(ctx context.Context, id entry.ID, name string) (entry.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return entry.Tag{}, fmt.Errorf("tag name is required")
	}
	normalized := strings.ToLower(name)
	var tag entry.Tag
	err := store.pool.QueryRow(ctx, `
		WITH selected_tag AS (
			INSERT INTO tags (name, normalized_name)
			VALUES ($2, $3)
			ON CONFLICT (normalized_name)
			DO UPDATE SET name = tags.name
			RETURNING id, name
		), linked AS (
			INSERT INTO entry_tags (entry_id, tag_id, origin)
			SELECT $1, id, 'user' FROM selected_tag
			ON CONFLICT DO NOTHING
		)
		SELECT id, name FROM selected_tag
	`, id, name, normalized).Scan(&tag.ID, &tag.Name)
	if err != nil {
		return entry.Tag{}, fmt.Errorf("add tag to entry %s: %w", id, err)
	}
	return tag, nil
}

func (store *EntryStore) RemoveTag(ctx context.Context, id entry.ID, tagID string) error {
	_, err := store.pool.Exec(ctx, `
		DELETE FROM entry_tags
		WHERE entry_id = $1 AND tag_id = $2 AND origin = 'user'
	`, id, tagID)
	if err != nil {
		return fmt.Errorf("remove tag from entry %s: %w", id, err)
	}
	return nil
}

type entryRow interface {
	Scan(...any) error
}

func scanEntry(row entryRow) (entry.Entry, error) {
	var item entry.Entry
	var annotationJSON []byte
	err := row.Scan(
		&item.ID,
		&item.SourceID,
		&item.IdentityKey,
		&item.ExternalID,
		&item.CanonicalURL,
		&item.SourceTitle,
		&item.DisplayTitle,
		&item.Author,
		&item.Summary,
		&item.ContentHTML,
		&item.PublishedAt,
		&item.DiscoveredAt,
		&item.ReadAt,
		&item.StarredAt,
		&item.HiddenAt,
		&item.LaterAt,
		&item.Note,
		&annotationJSON,
	)
	if err == nil && len(annotationJSON) > 0 && string(annotationJSON) != "null" {
		if err := json.Unmarshal(annotationJSON, &item.Annotation); err != nil {
			return entry.Entry{}, fmt.Errorf("decode entry annotation: %w", err)
		}
	}
	return item, err
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (store *EntryStore) CommitBatch(
	ctx context.Context,
	acquisition ingestion.Acquisition,
	owner string,
	candidates []ingestion.Candidate,
	checkpoint json.RawMessage,
) error {
	identified := make([]identifiedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		identityKey, err := entry.Identity(candidate)
		if err != nil {
			return err
		}
		content := candidate.ContentHTML
		if content == "" {
			content = candidate.Summary
		}
		identified = append(identified, identifiedCandidate{
			identityKey: identityKey,
			candidate:   candidate,
			features:    story.BuildFeatures(candidate.Title, content),
		})
	}
	if len(checkpoint) == 0 {
		checkpoint = json.RawMessage(`{}`)
	}

	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin entry batch: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	newCount := 0
	updatedCount := 0
	for _, item := range identified {
		candidate := item.candidate
		var entryID entry.ID
		var inserted bool
		err := tx.QueryRow(ctx, `
			INSERT INTO entries (
				source_id, identity_key, external_id, canonical_url,
				source_title, author, summary, content_html, published_at,
				normalized_title, content_hash, content_simhash
			)
			SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
			WHERE NOT EXISTS (
				SELECT 1
				FROM entry_tombstones
				WHERE source_id = $1 AND identity_key = $2
			)
			ON CONFLICT (source_id, identity_key)
			DO UPDATE SET
				external_id = EXCLUDED.external_id,
				canonical_url = EXCLUDED.canonical_url,
				source_title = EXCLUDED.source_title,
				author = EXCLUDED.author,
				summary = EXCLUDED.summary,
				content_html = EXCLUDED.content_html,
				published_at = EXCLUDED.published_at,
				normalized_title = EXCLUDED.normalized_title,
				content_hash = EXCLUDED.content_hash,
				content_simhash = EXCLUDED.content_simhash,
				embedding = CASE
					WHEN (entries.source_title, entries.summary, entries.content_html, entries.published_at)
						IS DISTINCT FROM
						(EXCLUDED.source_title, EXCLUDED.summary, EXCLUDED.content_html, EXCLUDED.published_at)
					THEN NULL ELSE entries.embedding
				END,
				embedding_model = CASE
					WHEN (entries.source_title, entries.summary, entries.content_html, entries.published_at)
						IS DISTINCT FROM
						(EXCLUDED.source_title, EXCLUDED.summary, EXCLUDED.content_html, EXCLUDED.published_at)
					THEN '' ELSE entries.embedding_model
				END,
				embedding_updated_at = CASE
					WHEN (entries.source_title, entries.summary, entries.content_html, entries.published_at)
						IS DISTINCT FROM
						(EXCLUDED.source_title, EXCLUDED.summary, EXCLUDED.content_html, EXCLUDED.published_at)
					THEN NULL ELSE entries.embedding_updated_at
				END,
				embedding_attempted_at = CASE
					WHEN (entries.source_title, entries.summary, entries.content_html, entries.published_at)
						IS DISTINCT FROM
						(EXCLUDED.source_title, EXCLUDED.summary, EXCLUDED.content_html, EXCLUDED.published_at)
					THEN NULL ELSE entries.embedding_attempted_at
				END,
				source_updated_at = now(),
				source_deleted = false
			RETURNING id, (xmax = 0)
		`,
			acquisition.SourceID,
			item.identityKey,
			candidate.ExternalID,
			candidate.URL,
			candidate.Title,
			candidate.Author,
			candidate.Summary,
			candidate.ContentHTML,
			candidate.PublishedAt,
			item.features.NormalizedTitle,
			item.features.ContentHash,
			int64(item.features.ContentSimHash),
		).Scan(&entryID, &inserted)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("upsert entry %q: %w", item.identityKey, err)
		}
		if candidate.Annotation != nil {
			detail := candidate.Annotation
			if _, err := tx.Exec(ctx, `
				INSERT INTO entry_annotations (
					entry_id, provider, book_identity, book_title, book_author,
					chapter, location, highlight_color, annotation_note, highlighted_at
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT (entry_id)
				DO UPDATE SET
					provider = EXCLUDED.provider,
					book_identity = EXCLUDED.book_identity,
					book_title = EXCLUDED.book_title,
					book_author = EXCLUDED.book_author,
					chapter = EXCLUDED.chapter,
					location = EXCLUDED.location,
					highlight_color = EXCLUDED.highlight_color,
					annotation_note = EXCLUDED.annotation_note,
					highlighted_at = EXCLUDED.highlighted_at,
					imported_at = now()
			`, entryID, detail.Provider, detail.BookIdentity, detail.BookTitle,
				detail.BookAuthor, detail.Chapter, detail.Location, detail.HighlightColor,
				detail.AnnotationNote, detail.HighlightedAt); err != nil {
				return fmt.Errorf("upsert annotation for entry %q: %w", item.identityKey, err)
			}
		}
		if err := applyEnabledRulesTx(ctx, tx, entryID); err != nil {
			return fmt.Errorf("apply rules to entry %q: %w", item.identityKey, err)
		}
		if _, err := tx.Exec(ctx, `
			WITH created AS (
				INSERT INTO stories (
					representative_entry_id,
					first_published_at,
					last_published_at,
					read_at,
					starred_at,
					hidden_at,
					later_at
				)
				SELECT
					entry.id,
					coalesce($2::timestamptz, entry.discovered_at),
					coalesce($2::timestamptz, entry.discovered_at),
					entry.read_at,
					entry.starred_at,
					entry.hidden_at,
					entry.later_at
				FROM entries AS entry
				WHERE entry.id = $1
				  AND NOT EXISTS (
					SELECT 1 FROM story_entries WHERE entry_id = $1
				  )
				RETURNING id
			)
			INSERT INTO story_entries (story_id, entry_id, match_method, final_score)
			SELECT id, $1, 'singleton', 1 FROM created
		`, entryID, candidate.PublishedAt); err != nil {
			return fmt.Errorf("ensure Story for entry %q: %w", item.identityKey, err)
		}
		if !inserted {
			if _, err := tx.Exec(ctx, `
				UPDATE stories
				SET clustered_at = NULL, updated_at = now()
				WHERE id = (
					SELECT story_id
					FROM story_entries
					WHERE entry_id = $1
				)
			`, entryID); err != nil {
				return fmt.Errorf("schedule Story refresh for entry %q: %w", item.identityKey, err)
			}
		}
		if inserted {
			newCount++
		} else {
			updatedCount++
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO source_checkpoints (source_id, checkpoint)
		VALUES ($1, $2)
		ON CONFLICT (source_id)
		DO UPDATE SET checkpoint = EXCLUDED.checkpoint, updated_at = now()
	`, acquisition.SourceID, checkpoint); err != nil {
		return fmt.Errorf("save source checkpoint: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE acquisitions
		SET
			status = 'succeeded',
			finished_at = now(),
			candidate_count = $3,
			new_count = $4,
			updated_count = $5,
			lease_owner = '',
			lease_until = NULL,
			last_error = ''
		WHERE id = $1 AND status = 'running' AND lease_owner = $2
	`, acquisition.ID, owner, len(candidates), newCount, updatedCount)
	if err != nil {
		return fmt.Errorf("complete acquisition in batch: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("complete acquisition in batch: lease is not owned by %q", owner)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit entry batch: %w", err)
	}
	return nil
}

type identifiedCandidate struct {
	identityKey string
	candidate   ingestion.Candidate
	features    story.Features
}
