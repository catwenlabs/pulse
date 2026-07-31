package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wenpengfei/pulse/internal/entry"
	"github.com/wenpengfei/pulse/internal/story"
)

type StoryStore struct {
	pool *pgxpool.Pool
}

func NewStoryStore(pool *pgxpool.Pool) *StoryStore {
	return &StoryStore{pool: pool}
}

func (store *StoryStore) Search(ctx context.Context, query story.Query) ([]story.Story, error) {
	if query.Limit <= 0 || query.Limit > 200 {
		query.Limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	rows, err := store.pool.Query(ctx, `
		SELECT
			story.id, story.entry_count, story.source_count,
			story.first_published_at, story.last_published_at,
			story.read_at, story.starred_at, story.hidden_at, story.later_at,
			entry.id, entry.source_id, entry.identity_key, entry.external_id, entry.canonical_url,
			entry.source_title, entry.display_title, entry.author, entry.summary, entry.content_html,
			entry.published_at, entry.discovered_at, entry.read_at, entry.starred_at,
			entry.hidden_at, entry.later_at, entry.note,
			to_jsonb(entry_annotation) - 'entry_id' - 'imported_at'
		FROM stories AS story
		JOIN entries AS entry ON entry.id = story.representative_entry_id
		LEFT JOIN entry_annotations AS entry_annotation ON entry_annotation.entry_id = entry.id
		WHERE
			($2 = '' OR (
				entry.source_title ILIKE '%' || $2 || '%'
				OR entry.display_title ILIKE '%' || $2 || '%'
				OR entry.summary ILIKE '%' || $2 || '%'
				OR word_similarity(lower($2), lower(entry.source_title)) >= 0.45
			))
			AND (
				$3 = ''
				OR ($3 = 'inbox' AND story.hidden_at IS NULL)
				OR ($3 = 'unread' AND story.hidden_at IS NULL AND story.read_at IS NULL)
				OR ($3 = 'starred' AND story.starred_at IS NOT NULL)
				OR ($3 = 'later' AND story.later_at IS NOT NULL)
				OR ($3 = 'hidden' AND story.hidden_at IS NOT NULL)
			)
			AND (
				$4 = ''
				OR EXISTS (
					SELECT 1
					FROM story_entries AS member
					JOIN entry_tags ON entry_tags.entry_id = member.entry_id
					JOIN tags ON tags.id = entry_tags.tag_id
					WHERE member.story_id = story.id
					  AND tags.normalized_name = lower($4)
				)
			)
			AND (
				$5 = ''
				OR EXISTS (
					SELECT 1
					FROM story_entries AS member
					JOIN entries AS source_entry ON source_entry.id = member.entry_id
					WHERE member.story_id = story.id
					  AND source_entry.source_id = $5::uuid
				)
			)
		ORDER BY story.last_published_at DESC NULLS LAST, story.id DESC
		LIMIT $1 OFFSET $6
	`,
		query.Limit,
		strings.TrimSpace(query.Search),
		strings.TrimSpace(query.State),
		strings.TrimSpace(query.Tag),
		strings.TrimSpace(query.SourceID),
		query.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("search Stories: %w", err)
	}
	defer rows.Close()

	result := make([]story.Story, 0, query.Limit)
	for rows.Next() {
		item, err := scanStory(rows)
		if err != nil {
			return nil, fmt.Errorf("scan Story: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search Stories: %w", err)
	}
	return result, nil
}

func (store *StoryStore) Get(ctx context.Context, id story.ID) (story.Story, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT
			entry.id, entry.source_id, entry.identity_key, entry.external_id, entry.canonical_url,
			entry.source_title, entry.display_title, entry.author, entry.summary, entry.content_html,
			entry.published_at, entry.discovered_at, entry.read_at, entry.starred_at,
			entry.hidden_at, entry.later_at, entry.note,
			to_jsonb(entry_annotation) - 'entry_id' - 'imported_at'
		FROM story_entries AS member
		JOIN entries AS entry ON entry.id = member.entry_id
		LEFT JOIN entry_annotations AS entry_annotation ON entry_annotation.entry_id = entry.id
		WHERE member.story_id = $1
		ORDER BY coalesce(entry.published_at, entry.discovered_at) DESC, entry.id DESC
	`, id)
	if err != nil {
		return story.Story{}, fmt.Errorf("get Story %s: %w", id, err)
	}
	defer rows.Close()
	var entries []entry.Entry
	for rows.Next() {
		item, err := scanEntry(rows)
		if err != nil {
			return story.Story{}, fmt.Errorf("scan Story Entry: %w", err)
		}
		entries = append(entries, item)
	}
	if err := rows.Err(); err != nil {
		return story.Story{}, fmt.Errorf("get Story %s: %w", id, err)
	}
	if len(entries) == 0 {
		return story.Story{}, fmt.Errorf("%w: %s", entry.ErrNotFound, id)
	}
	var item story.Story
	var representativeID entry.ID
	err = store.pool.QueryRow(ctx, `
		SELECT
			id, representative_entry_id, entry_count, source_count, first_published_at, last_published_at,
			read_at, starred_at, hidden_at, later_at
		FROM stories
		WHERE id = $1
	`, id).Scan(
		&item.ID,
		&representativeID,
		&item.EntryCount,
		&item.SourceCount,
		&item.FirstPublishedAt,
		&item.LastPublishedAt,
		&item.ReadAt,
		&item.StarredAt,
		&item.HiddenAt,
		&item.LaterAt,
	)
	if err != nil {
		return story.Story{}, fmt.Errorf("get Story metadata %s: %w", id, err)
	}
	item.Entries = entries
	item.Representative = entries[0]
	for _, candidate := range entries {
		if candidate.ID == representativeID {
			item.Representative = candidate
			break
		}
	}
	return item, nil
}

func (store *StoryStore) Pending(ctx context.Context, limit int, model string) ([]story.Candidate, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := store.pool.Query(ctx, `
		SELECT
			story.id, story.clustered_at,
			entry.id, entry.source_id, entry.identity_key, entry.external_id, entry.canonical_url,
			entry.source_title, entry.display_title, entry.author, entry.summary, entry.content_html,
			entry.published_at, entry.discovered_at, entry.read_at, entry.starred_at,
			entry.hidden_at, entry.later_at, entry.note,
			to_jsonb(entry_annotation) - 'entry_id' - 'imported_at',
			entry.normalized_title, entry.content_hash, entry.content_simhash,
			coalesce(entry.embedding::text, ''), entry.embedding_model
		FROM stories AS story
		JOIN entries AS entry ON entry.id = story.representative_entry_id
		LEFT JOIN entry_annotations AS entry_annotation ON entry_annotation.entry_id = entry.id
		WHERE (
			story.clustered_at IS NULL
			OR (
				$2 <> ''
				AND (
					(entry.embedding IS NOT NULL AND entry.embedding_model <> $2)
					OR (
						entry.embedding IS NULL
						AND entry.embedding_attempted_at IS NOT NULL
						AND entry.embedding_attempted_at < now() - interval '1 day'
					)
				)
			)
		  )
		ORDER BY
			(story.clustered_at IS NULL) DESC,
			coalesce(entry.embedding_attempted_at, '-infinity') ASC,
			story.created_at ASC
		LIMIT $1
	`, limit, model)
	if err != nil {
		return nil, fmt.Errorf("list pending Stories: %w", err)
	}
	defer rows.Close()
	return scanCandidates(rows)
}

func (store *StoryStore) Candidates(
	ctx context.Context,
	item story.Candidate,
	limit int,
) ([]story.Candidate, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var embeddingVector any
	if len(item.Features.Embedding) > 0 && item.Features.EmbeddingModel != "" {
		embeddingVector = vectorLiteral(item.Features.Embedding)
	}
	rows, err := store.pool.Query(ctx, `
		WITH lexical_candidates AS (
			SELECT story.id
			FROM stories AS story
			JOIN entries AS entry ON entry.id = story.representative_entry_id
			WHERE story.id <> $1
			  AND coalesce(entry.published_at, entry.discovered_at)
			      BETWEEN $2::timestamptz - interval '7 days'
			          AND $2::timestamptz + interval '7 days'
			ORDER BY
				CASE WHEN entry.content_hash <> '' AND entry.content_hash = $3 THEN 1 ELSE 0 END DESC,
				word_similarity(entry.normalized_title, $4) DESC,
				coalesce(entry.published_at, entry.discovered_at) DESC
			LIMIT greatest(1, $7 / 2)
		), semantic_candidates AS (
			SELECT story.id
			FROM stories AS story
			JOIN entries AS entry ON entry.id = story.representative_entry_id
			WHERE story.id <> $1
			  AND $5::vector IS NOT NULL
			  AND entry.embedding IS NOT NULL
			  AND entry.embedding_model = $6
			  AND vector_dims(entry.embedding) = vector_dims($5::vector)
			  AND coalesce(entry.published_at, entry.discovered_at)
			      BETWEEN $2::timestamptz - interval '7 days'
			          AND $2::timestamptz + interval '7 days'
			ORDER BY entry.embedding <=> $5::vector
			LIMIT greatest(1, $7 / 2)
		), selected AS (
			SELECT id FROM lexical_candidates
			UNION
			SELECT id FROM semantic_candidates
		)
		SELECT
			story.id, story.clustered_at,
			entry.id, entry.source_id, entry.identity_key, entry.external_id, entry.canonical_url,
			entry.source_title, entry.display_title, entry.author, entry.summary, entry.content_html,
			entry.published_at, entry.discovered_at, entry.read_at, entry.starred_at,
			entry.hidden_at, entry.later_at, entry.note,
			to_jsonb(entry_annotation) - 'entry_id' - 'imported_at',
			entry.normalized_title, entry.content_hash, entry.content_simhash,
			coalesce(entry.embedding::text, ''), entry.embedding_model
		FROM selected
		JOIN stories AS story ON story.id = selected.id
		JOIN entries AS entry ON entry.id = story.representative_entry_id
		LEFT JOIN entry_annotations AS entry_annotation ON entry_annotation.entry_id = entry.id
	`,
		item.StoryID,
		entryTimeForStore(item.Entry),
		item.Features.ContentHash,
		item.Features.NormalizedTitle,
		embeddingVector,
		item.Features.EmbeddingModel,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list Story candidates: %w", err)
	}
	defer rows.Close()
	return scanCandidates(rows)
}

func (store *StoryStore) SaveEmbedding(
	ctx context.Context,
	id entry.ID,
	model string,
	vector []float32,
) error {
	_, err := store.pool.Exec(ctx, `
		UPDATE entries
		SET
			embedding = $2::vector,
			embedding_model = $3,
			embedding_updated_at = now()
		WHERE id = $1
	`, id, vectorLiteral(vector), model)
	if err != nil {
		return fmt.Errorf("save Entry embedding %s: %w", id, err)
	}
	return nil
}

func (store *StoryStore) SaveFeatures(
	ctx context.Context,
	id entry.ID,
	features story.Features,
) error {
	_, err := store.pool.Exec(ctx, `
		UPDATE entries
		SET
			normalized_title = $2,
			content_hash = $3,
			content_simhash = $4
		WHERE id = $1
	`, id, features.NormalizedTitle, features.ContentHash, int64(features.ContentSimHash))
	if err != nil {
		return fmt.Errorf("save Entry Story features %s: %w", id, err)
	}
	return nil
}

func (store *StoryStore) MarkEmbeddingAttempted(ctx context.Context, id entry.ID) error {
	_, err := store.pool.Exec(ctx, `
		UPDATE entries SET embedding_attempted_at = now() WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("mark Entry embedding attempted %s: %w", id, err)
	}
	return nil
}

func (store *StoryStore) Merge(
	ctx context.Context,
	from story.ID,
	into story.ID,
	match story.Result,
) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Story merge: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		SELECT id
		FROM stories
		WHERE id = ANY($1::uuid[])
		ORDER BY id
		FOR UPDATE
	`, []string{string(from), string(into)}); err != nil {
		return fmt.Errorf("lock Stories: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE story_entries
		SET
			story_id = $2,
			match_method = $3,
			final_score = $4,
			embedding_score = $5,
			title_score = $6,
			content_score = $7,
			time_score = $8,
			critical_conflict = $9,
			algorithm_version = 1,
			joined_at = now()
		WHERE story_id = $1
	`, from, into, match.Method, match.FinalScore, match.EmbeddingScore,
		match.TitleScore, match.ContentScore, match.TimeScore, match.CriticalConflict)
	if err != nil {
		return fmt.Errorf("move Story entries: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE stories AS story
		SET
			entry_count = aggregate.entry_count,
			source_count = aggregate.source_count,
			first_published_at = aggregate.first_published_at,
			last_published_at = aggregate.last_published_at,
			read_at = coalesce(story.read_at, source_story.read_at),
			starred_at = coalesce(story.starred_at, source_story.starred_at),
			hidden_at = coalesce(story.hidden_at, source_story.hidden_at),
			later_at = coalesce(story.later_at, source_story.later_at),
			clustered_at = now(),
			updated_at = now()
		FROM (
			SELECT
				count(*)::integer AS entry_count,
				count(DISTINCT entry.source_id)::integer AS source_count,
				min(coalesce(entry.published_at, entry.discovered_at)) AS first_published_at,
				max(coalesce(entry.published_at, entry.discovered_at)) AS last_published_at
			FROM story_entries AS member
			JOIN entries AS entry ON entry.id = member.entry_id
			WHERE member.story_id = $1
		) AS aggregate,
		stories AS source_story
		WHERE story.id = $1 AND source_story.id = $2
	`, into, from); err != nil {
		return fmt.Errorf("refresh target Story: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE entries AS entry
		SET
			read_at = story.read_at,
			starred_at = story.starred_at,
			hidden_at = story.hidden_at,
			later_at = story.later_at
		FROM story_entries AS member
		JOIN stories AS story ON story.id = member.story_id
		WHERE member.story_id = $1 AND member.entry_id = entry.id
	`, into); err != nil {
		return fmt.Errorf("synchronize target Story state: %w", err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM stories WHERE id = $1", from); err != nil {
		return fmt.Errorf("delete empty Story: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Story merge: %w", err)
	}
	return nil
}

func (store *StoryStore) MergeManual(ctx context.Context, from story.ID, into story.ID) error {
	if from == into {
		return story.ErrSelfMerge
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin manual Story merge: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		SELECT id
		FROM stories
		WHERE id = ANY($1::uuid[])
		ORDER BY id
		FOR UPDATE
	`, []string{string(from), string(into)}); err != nil {
		return fmt.Errorf("lock Stories: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE story_entries
		SET
			story_id = $2,
			match_method = $3,
			final_score = NULL,
			embedding_score = NULL,
			title_score = NULL,
			content_score = NULL,
			time_score = NULL,
			critical_conflict = false,
			algorithm_version = 1,
			joined_at = now()
		WHERE story_id = $1
	`, from, into, story.MatchManual)
	if err != nil {
		return fmt.Errorf("move Story entries: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE stories AS story
		SET
			entry_count = aggregate.entry_count,
			source_count = aggregate.source_count,
			first_published_at = aggregate.first_published_at,
			last_published_at = aggregate.last_published_at,
			read_at = coalesce(story.read_at, source_story.read_at),
			starred_at = coalesce(story.starred_at, source_story.starred_at),
			hidden_at = coalesce(story.hidden_at, source_story.hidden_at),
			later_at = coalesce(story.later_at, source_story.later_at),
			clustered_at = now(),
			updated_at = now()
		FROM (
			SELECT
				count(*)::integer AS entry_count,
				count(DISTINCT entry.source_id)::integer AS source_count,
				min(coalesce(entry.published_at, entry.discovered_at)) AS first_published_at,
				max(coalesce(entry.published_at, entry.discovered_at)) AS last_published_at
			FROM story_entries AS member
			JOIN entries AS entry ON entry.id = member.entry_id
			WHERE member.story_id = $1
		) AS aggregate,
		stories AS source_story
		WHERE story.id = $1 AND source_story.id = $2
	`, into, from); err != nil {
		return fmt.Errorf("refresh target Story: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE entries AS entry
		SET
			read_at = story.read_at,
			starred_at = story.starred_at,
			hidden_at = story.hidden_at,
			later_at = story.later_at
		FROM story_entries AS member
		JOIN stories AS story ON story.id = member.story_id
		WHERE member.story_id = $1 AND member.entry_id = entry.id
	`, into); err != nil {
		return fmt.Errorf("synchronize target Story state: %w", err)
	}

	if _, err := tx.Exec(ctx, "DELETE FROM stories WHERE id = $1", from); err != nil {
		return fmt.Errorf("delete empty Story: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit manual Story merge: %w", err)
	}
	return nil
}

func (store *StoryStore) Split(ctx context.Context, storyID story.ID, entryID entry.ID) (story.ID, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin Story split: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		SELECT id FROM stories WHERE id = $1 FOR UPDATE
	`, storyID); err != nil {
		return "", fmt.Errorf("lock Story: %w", err)
	}

	var newID story.ID
	err = tx.QueryRow(ctx, `
		INSERT INTO stories (
			representative_entry_id,
			entry_count,
			source_count,
			first_published_at,
			last_published_at,
			read_at,
			starred_at,
			hidden_at,
			later_at,
			clustered_at
		)
		SELECT
			entry.id,
			1,
			1,
			coalesce(entry.published_at, entry.discovered_at),
			coalesce(entry.published_at, entry.discovered_at),
			entry.read_at,
			entry.starred_at,
			entry.hidden_at,
			entry.later_at,
			now()
		FROM entries AS entry
		WHERE entry.id = $2
		  AND EXISTS (SELECT 1 FROM story_entries WHERE story_id = $1 AND entry_id = $2)
		RETURNING stories.id
	`, storyID, entryID).Scan(&newID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("%w: entry %s in story %s", entry.ErrNotFound, entryID, storyID)
		}
		return "", fmt.Errorf("create split Story: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE story_entries
		SET
			story_id = $3,
			match_method = $4,
			final_score = NULL,
			embedding_score = NULL,
			title_score = NULL,
			content_score = NULL,
			time_score = NULL,
			critical_conflict = false,
			algorithm_version = 1,
			joined_at = now()
		WHERE story_id = $1 AND entry_id = $2
	`, storyID, entryID, newID, story.MatchManual); err != nil {
		return "", fmt.Errorf("move split entry: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE stories AS story
		SET
			representative_entry_id = (
				SELECT member.entry_id
				FROM story_entries AS member
				WHERE member.story_id = story.id
				ORDER BY member.joined_at ASC
				LIMIT 1
			),
			entry_count = aggregate.entry_count,
			source_count = aggregate.source_count,
			first_published_at = aggregate.first_published_at,
			last_published_at = aggregate.last_published_at,
			clustered_at = now(),
			updated_at = now()
		FROM (
			SELECT
				count(*)::integer AS entry_count,
				count(DISTINCT entry.source_id)::integer AS source_count,
				min(coalesce(entry.published_at, entry.discovered_at)) AS first_published_at,
				max(coalesce(entry.published_at, entry.discovered_at)) AS last_published_at
			FROM story_entries AS member
			JOIN entries AS entry ON entry.id = member.entry_id
			WHERE member.story_id = $1
		) AS aggregate
		WHERE story.id = $1
	`, storyID); err != nil {
		return "", fmt.Errorf("refresh original Story: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit Story split: %w", err)
	}
	return newID, nil
}

func (store *StoryStore) MarkClustered(ctx context.Context, id story.ID) error {
	_, err := store.pool.Exec(ctx, `
		UPDATE stories SET clustered_at = now(), updated_at = now() WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("mark Story clustered %s: %w", id, err)
	}
	return nil
}

func (store *StoryStore) Update(
	ctx context.Context,
	id story.ID,
	patch story.Patch,
) (story.Story, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return story.Story{}, fmt.Errorf("begin Story update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE stories
		SET
			read_at = CASE WHEN $2 THEN CASE WHEN $3 THEN coalesce(read_at, now()) ELSE NULL END ELSE read_at END,
			starred_at = CASE WHEN $4 THEN CASE WHEN $5 THEN coalesce(starred_at, now()) ELSE NULL END ELSE starred_at END,
			hidden_at = CASE WHEN $6 THEN CASE WHEN $7 THEN coalesce(hidden_at, now()) ELSE NULL END ELSE hidden_at END,
			later_at = CASE WHEN $8 THEN CASE WHEN $9 THEN coalesce(later_at, now()) ELSE NULL END ELSE later_at END,
			updated_at = now()
		WHERE id = $1
	`, id,
		patch.Read != nil, boolValue(patch.Read),
		patch.Starred != nil, boolValue(patch.Starred),
		patch.Hidden != nil, boolValue(patch.Hidden),
		patch.Later != nil, boolValue(patch.Later),
	)
	if err != nil {
		return story.Story{}, fmt.Errorf("update Story %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return story.Story{}, fmt.Errorf("%w: %s", entry.ErrNotFound, id)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE entries AS entry
		SET
			read_at = CASE WHEN $2 THEN CASE WHEN $3 THEN coalesce(entry.read_at, now()) ELSE NULL END ELSE entry.read_at END,
			starred_at = CASE WHEN $4 THEN CASE WHEN $5 THEN coalesce(entry.starred_at, now()) ELSE NULL END ELSE entry.starred_at END,
			hidden_at = CASE WHEN $6 THEN CASE WHEN $7 THEN coalesce(entry.hidden_at, now()) ELSE NULL END ELSE entry.hidden_at END,
			later_at = CASE WHEN $8 THEN CASE WHEN $9 THEN coalesce(entry.later_at, now()) ELSE NULL END ELSE entry.later_at END
		FROM story_entries AS member
		WHERE member.story_id = $1 AND member.entry_id = entry.id
	`, id,
		patch.Read != nil, boolValue(patch.Read),
		patch.Starred != nil, boolValue(patch.Starred),
		patch.Hidden != nil, boolValue(patch.Hidden),
		patch.Later != nil, boolValue(patch.Later),
	); err != nil {
		return story.Story{}, fmt.Errorf("update Story Entries %s: %w", id, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return story.Story{}, fmt.Errorf("commit Story update %s: %w", id, err)
	}
	return store.Get(ctx, id)
}

func (store *StoryStore) MarkRead(ctx context.Context, sourceID string) (int64, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin mark Stories read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `
		UPDATE stories AS story
		SET read_at = now(), updated_at = now()
		WHERE story.read_at IS NULL
		  AND (
			$1 = ''
			OR EXISTS (
				SELECT 1
				FROM story_entries AS member
				JOIN entries AS entry ON entry.id = member.entry_id
				WHERE member.story_id = story.id AND entry.source_id = $1::uuid
			)
		  )
		RETURNING story.id
	`, sourceID)
	if err != nil {
		return 0, fmt.Errorf("mark Stories read: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan marked Story: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("mark Stories read: %w", err)
	}
	if len(ids) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE entries AS entry
			SET read_at = coalesce(entry.read_at, now())
			FROM story_entries AS member
			WHERE member.story_id = ANY($1::uuid[]) AND member.entry_id = entry.id
		`, ids); err != nil {
			return 0, fmt.Errorf("mark Story Entries read: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit mark Stories read: %w", err)
	}
	return int64(len(ids)), nil
}

type storyRow interface {
	Scan(...any) error
}

func scanStory(row storyRow) (story.Story, error) {
	var item story.Story
	var representative entry.Entry
	var annotationJSON []byte
	err := row.Scan(
		&item.ID,
		&item.EntryCount,
		&item.SourceCount,
		&item.FirstPublishedAt,
		&item.LastPublishedAt,
		&item.ReadAt,
		&item.StarredAt,
		&item.HiddenAt,
		&item.LaterAt,
		&representative.ID,
		&representative.SourceID,
		&representative.IdentityKey,
		&representative.ExternalID,
		&representative.CanonicalURL,
		&representative.SourceTitle,
		&representative.DisplayTitle,
		&representative.Author,
		&representative.Summary,
		&representative.ContentHTML,
		&representative.PublishedAt,
		&representative.DiscoveredAt,
		&representative.ReadAt,
		&representative.StarredAt,
		&representative.HiddenAt,
		&representative.LaterAt,
		&representative.Note,
		&annotationJSON,
	)
	if err != nil {
		return story.Story{}, err
	}
	if err := decodeAnnotation(annotationJSON, &representative); err != nil {
		return story.Story{}, err
	}
	item.Representative = representative
	return item, nil
}

func scanCandidates(rows pgx.Rows) ([]story.Candidate, error) {
	var result []story.Candidate
	for rows.Next() {
		var item story.Candidate
		var annotationJSON []byte
		var simHash int64
		var vector string
		err := rows.Scan(
			&item.StoryID,
			&item.ClusteredAt,
			&item.Entry.ID,
			&item.Entry.SourceID,
			&item.Entry.IdentityKey,
			&item.Entry.ExternalID,
			&item.Entry.CanonicalURL,
			&item.Entry.SourceTitle,
			&item.Entry.DisplayTitle,
			&item.Entry.Author,
			&item.Entry.Summary,
			&item.Entry.ContentHTML,
			&item.Entry.PublishedAt,
			&item.Entry.DiscoveredAt,
			&item.Entry.ReadAt,
			&item.Entry.StarredAt,
			&item.Entry.HiddenAt,
			&item.Entry.LaterAt,
			&item.Entry.Note,
			&annotationJSON,
			&item.Features.NormalizedTitle,
			&item.Features.ContentHash,
			&simHash,
			&vector,
			&item.Features.EmbeddingModel,
		)
		if err != nil {
			return nil, err
		}
		item.Features.ContentSimHash = uint64(simHash)
		item.Features.Embedding, err = parseVector(vector)
		if err != nil {
			return nil, err
		}
		item.Features.CanonicalURL = item.Entry.CanonicalURL
		if err := decodeAnnotation(annotationJSON, &item.Entry); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func vectorLiteral(vector []float32) string {
	var result strings.Builder
	result.WriteByte('[')
	for index, value := range vector {
		if index > 0 {
			result.WriteByte(',')
		}
		result.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	result.WriteByte(']')
	return result.String()
}

func parseVector(value string) ([]float32, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	value = strings.TrimPrefix(strings.TrimSuffix(value, "]"), "[")
	parts := strings.Split(value, ",")
	result := make([]float32, len(parts))
	for index, part := range parts {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(part), 32)
		if err != nil {
			return nil, fmt.Errorf("parse embedding vector: %w", err)
		}
		result[index] = float32(parsed)
	}
	return result, nil
}

func entryTimeForStore(item entry.Entry) time.Time {
	if item.PublishedAt != nil {
		return *item.PublishedAt
	}
	return item.DiscoveredAt
}

func decodeAnnotation(value []byte, item *entry.Entry) error {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	if err := json.Unmarshal(value, &item.Annotation); err != nil {
		return err
	}
	return nil
}
