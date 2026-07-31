ALTER TABLE stories
    ADD COLUMN read_at timestamptz,
    ADD COLUMN starred_at timestamptz,
    ADD COLUMN hidden_at timestamptz,
    ADD COLUMN later_at timestamptz;

UPDATE stories AS story
SET
    read_at = entry.read_at,
    starred_at = entry.starred_at,
    hidden_at = entry.hidden_at,
    later_at = entry.later_at
FROM entries AS entry
WHERE entry.id = story.representative_entry_id;
