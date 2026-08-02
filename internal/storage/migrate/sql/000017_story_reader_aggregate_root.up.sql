DO $$
DECLARE
    conflict_report text;
BEGIN
    SELECT format(
        'Story %s has conflicting Entry display_title values: %s',
        member.story_id,
        string_agg(format('%s=%L', entry.id, NULLIF(entry.display_title, '')), ', ' ORDER BY entry.id)
    )
    INTO conflict_report
    FROM story_entries AS member
    JOIN entries AS entry ON entry.id = member.entry_id
    WHERE NULLIF(entry.display_title, '') IS NOT NULL
    GROUP BY member.story_id
    HAVING count(DISTINCT NULLIF(entry.display_title, '')) > 1
    LIMIT 1;
    IF conflict_report IS NOT NULL THEN
        RAISE EXCEPTION USING MESSAGE = conflict_report;
    END IF;

    SELECT format(
        'Story %s has conflicting Entry note values: %s',
        member.story_id,
        string_agg(format('%s=%L', entry.id, NULLIF(entry.note, '')), ', ' ORDER BY entry.id)
    )
    INTO conflict_report
    FROM story_entries AS member
    JOIN entries AS entry ON entry.id = member.entry_id
    WHERE NULLIF(entry.note, '') IS NOT NULL
    GROUP BY member.story_id
    HAVING count(DISTINCT NULLIF(entry.note, '')) > 1
    LIMIT 1;
    IF conflict_report IS NOT NULL THEN
        RAISE EXCEPTION USING MESSAGE = conflict_report;
    END IF;
END
$$;

ALTER TABLE stories
    ADD COLUMN display_title text NOT NULL DEFAULT '',
    ADD COLUMN note text NOT NULL DEFAULT '';

/*
 * The conflict preflight above deliberately runs before any ownership data is
 * changed. The rest of this migration is transactional and therefore rolls
 * back together if a later invariant or constraint cannot be established.
 */

WITH metadata AS (
    SELECT
        member.story_id,
        max(NULLIF(entry.display_title, '')) AS display_title,
        max(NULLIF(entry.note, '')) AS note
    FROM story_entries AS member
    JOIN entries AS entry ON entry.id = member.entry_id
    GROUP BY member.story_id
)
UPDATE stories AS story
SET
    display_title = coalesce(metadata.display_title, story.display_title),
    note = coalesce(metadata.note, story.note)
FROM metadata
WHERE story.id = metadata.story_id;

WITH member_state AS (
    SELECT
        member.story_id,
        max(entry.read_at) AS read_at,
        max(entry.starred_at) AS starred_at,
        max(entry.hidden_at) AS hidden_at,
        max(entry.later_at) AS later_at
    FROM story_entries AS member
    JOIN entries AS entry ON entry.id = member.entry_id
    GROUP BY member.story_id
)
UPDATE stories AS story
SET
    read_at = CASE
        WHEN story.read_at IS NULL THEN member_state.read_at
        WHEN member_state.read_at IS NULL THEN story.read_at
        ELSE greatest(story.read_at, member_state.read_at)
    END,
    starred_at = CASE
        WHEN story.starred_at IS NULL THEN member_state.starred_at
        WHEN member_state.starred_at IS NULL THEN story.starred_at
        ELSE greatest(story.starred_at, member_state.starred_at)
    END,
    hidden_at = CASE
        WHEN story.hidden_at IS NULL THEN member_state.hidden_at
        WHEN member_state.hidden_at IS NULL THEN story.hidden_at
        ELSE greatest(story.hidden_at, member_state.hidden_at)
    END,
    later_at = CASE
        WHEN story.later_at IS NULL THEN member_state.later_at
        WHEN member_state.later_at IS NULL THEN story.later_at
        ELSE greatest(story.later_at, member_state.later_at)
    END,
    updated_at = now()
FROM member_state
WHERE story.id = member_state.story_id;

CREATE TABLE story_tags (
    story_id uuid NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    tag_id uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    origin text NOT NULL DEFAULT 'user' CHECK (origin IN ('user', 'rule')),
    rule_id uuid REFERENCES rules(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (story_id, tag_id, origin)
);

CREATE INDEX story_tags_tag_story_idx ON story_tags(tag_id, story_id);

INSERT INTO story_tags (story_id, tag_id, origin, rule_id, created_at)
SELECT DISTINCT ON (member.story_id, tags.entry_tag_id, tags.origin)
    member.story_id,
    tags.entry_tag_id,
    tags.origin,
    tags.rule_id,
    tags.created_at
FROM story_entries AS member
JOIN (
    SELECT entry_id, tag_id AS entry_tag_id, origin, rule_id, created_at
    FROM entry_tags
) AS tags ON tags.entry_id = member.entry_id
ORDER BY member.story_id, tags.entry_tag_id, tags.origin, tags.created_at;

CREATE TABLE story_rule_tags (
    rule_id uuid NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    rule_version integer NOT NULL,
    entry_id uuid NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    story_id uuid NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    tag_id uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (rule_id, entry_id, tag_id)
);

INSERT INTO story_rule_tags (rule_id, rule_version, entry_id, story_id, tag_id, created_at)
SELECT
    rule_tag.rule_id,
    rule_tag.rule_version,
    rule_tag.entry_id,
    member.story_id,
    rule_tag.tag_id,
    rule_tag.created_at
FROM rule_entry_tags AS rule_tag
JOIN story_entries AS member ON member.entry_id = rule_tag.entry_id;

CREATE INDEX story_rule_tags_story_idx ON story_rule_tags(story_id, tag_id);

CREATE TABLE story_aliases (
    alias_id uuid PRIMARY KEY,
    canonical_story_id uuid NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (alias_id <> canonical_story_id)
);

CREATE INDEX story_aliases_canonical_idx ON story_aliases(canonical_story_id);

ALTER TABLE stories
    DROP CONSTRAINT stories_representative_entry_id_fkey;

ALTER TABLE stories
    ADD CONSTRAINT stories_representative_entry_id_fkey
        FOREIGN KEY (representative_entry_id) REFERENCES entries(id) ON DELETE RESTRICT,
    ALTER COLUMN representative_entry_id SET NOT NULL;

CREATE OR REPLACE FUNCTION pulse_validate_story(story_uuid uuid)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
    representative_uuid uuid;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM stories WHERE id = story_uuid) THEN
        RETURN;
    END IF;

    SELECT representative_entry_id
    INTO representative_uuid
    FROM stories
    WHERE id = story_uuid;

    IF representative_uuid IS NULL
       OR NOT EXISTS (
           SELECT 1
           FROM story_entries
           WHERE story_id = story_uuid AND entry_id = representative_uuid
       ) THEN
        RAISE EXCEPTION 'Story % must be non-empty with a member representative', story_uuid;
    END IF;
END
$$;

CREATE OR REPLACE FUNCTION pulse_validate_story_row()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM pulse_validate_story(NEW.id);
    RETURN NEW;
END
$$;

CREATE OR REPLACE FUNCTION pulse_validate_story_membership()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP IN ('UPDATE', 'DELETE') THEN
        PERFORM pulse_validate_story(OLD.story_id);
    END IF;
    IF TG_OP <> 'DELETE' THEN
        PERFORM pulse_validate_story(NEW.story_id);
    END IF;
    RETURN NEW;
END
$$;

CREATE CONSTRAINT TRIGGER stories_invariant_trigger
AFTER INSERT OR UPDATE ON stories
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION pulse_validate_story_row();

CREATE CONSTRAINT TRIGGER story_entries_invariant_trigger
AFTER INSERT OR UPDATE OR DELETE ON story_entries
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION pulse_validate_story_membership();

ALTER TABLE entries
    DROP COLUMN display_title,
    DROP COLUMN note,
    DROP COLUMN read_at,
    DROP COLUMN starred_at,
    DROP COLUMN hidden_at,
    DROP COLUMN later_at;

DROP TABLE rule_entry_tags;
DROP TABLE entry_tags;
