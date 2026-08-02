ALTER TABLE stories
    ADD COLUMN sort_time timestamptz;

WITH aggregates AS (
    SELECT
        member.story_id,
        min(least(coalesce(entry.published_at, entry.discovered_at), entry.discovered_at)) AS sort_time
    FROM story_entries AS member
    JOIN entries AS entry ON entry.id = member.entry_id
    GROUP BY member.story_id
)
UPDATE stories AS story
SET
    sort_time = aggregates.sort_time
FROM aggregates
WHERE story.id = aggregates.story_id;

UPDATE stories
SET sort_time = created_at
WHERE sort_time IS NULL;

ALTER TABLE stories
    ALTER COLUMN sort_time SET NOT NULL;

DROP INDEX IF EXISTS stories_activity_idx;

ALTER TABLE stories
    DROP COLUMN entry_count,
    DROP COLUMN source_count,
    DROP COLUMN first_published_at,
    DROP COLUMN last_published_at;

CREATE INDEX stories_activity_idx
    ON stories (sort_time DESC, id DESC);
