/*
 * Reader-state access paths on stories.
 *
 * Migration 000017 moved reader state from entries to stories and dropped
 * entries.read_at/hidden_at; PostgreSQL cascaded that drop onto every index
 * referencing those columns (entries_inbox_idx, entries_unread_count_idx,
 * entries_search_idx, entries_display_title_trgm_idx), and no equivalent was
 * rebuilt on stories. Unread-scoped queries then degraded to walking the
 * whole stories table (digest previews, source unread counts, reader lists).
 *
 * These indexes restore state-scoped access on stories, the state root.
 */

-- Unread backlog, oldest first. The digest snapshot reads it forward
-- (order=oldest) and backward (order=newest), and unread counts drive from
-- it. The index only contains unread rows, so it stays proportional to the
-- backlog and filtering never needs heap verification.
CREATE INDEX stories_unread_sort_idx
    ON stories (sort_time ASC, id ASC)
    WHERE read_at IS NULL AND hidden_at IS NULL;

-- Default reader list ordering: unread first, newest first, stable id
-- tiebreak. Serving the ORDER BY lets the list stream in index order with a
-- LIMIT stop instead of sorting every page load.
CREATE INDEX stories_reader_order_idx
    ON stories ((read_at IS NULL) DESC, sort_time DESC, id DESC);

-- Starred and later views. Predicates match the reader state filters the
-- story search applies for those states (no hidden_at restriction).
CREATE INDEX stories_starred_idx
    ON stories (sort_time DESC, id DESC)
    WHERE starred_at IS NOT NULL;
CREATE INDEX stories_later_idx
    ON stories (sort_time DESC, id DESC)
    WHERE later_at IS NOT NULL;
