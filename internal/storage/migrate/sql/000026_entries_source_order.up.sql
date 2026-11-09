/*
 * Source browsing streams page by (source_id, discovered_at DESC, id DESC)
 * with a row-wise cursor predicate (see sourceEntriesSearchSQL). Migration
 * 000017 cascaded away the entries inbox-order index and 000025 only rebuilt
 * the Stories side, so Source pages had no matching index at all and every
 * page sorted the whole Source. This index lets each page flow out of the
 * index boundary directly, mirroring stories_reader_order_idx.
 */
CREATE INDEX entries_source_order_idx
    ON entries (source_id, discovered_at DESC, id DESC);
