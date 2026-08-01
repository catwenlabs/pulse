CREATE INDEX entries_unread_count_idx
    ON entries (source_id)
    WHERE read_at IS NULL AND hidden_at IS NULL;
