-- Clean break: the annotation -> Entry pipeline is replaced by the Document
-- reading domain. Legacy annotation rows are migrated into document_notes as
-- unmatched imported notes (the highlight text lived in entries.summary),
-- the annotations sources and their feed entries are removed, and the
-- pipeline's table is dropped.
INSERT INTO document_notes (
    book_identifier, book_title, book_author, chapter_index, location,
    highlight, note, highlight_color, source, highlighted_at
)
SELECT
    a.book_identity,
    a.book_title,
    a.book_author,
    0,
    a.chapter,
    coalesce(nullif(e.summary, ''), regexp_replace(e.content_html, '<[^>]*>', '', 'g')),
    a.annotation_note,
    a.highlight_color,
    'import',
    a.highlighted_at
FROM entry_annotations a
JOIN entries e ON e.id = a.entry_id;

-- Stories fed only by annotations disappear with their entries.
DELETE FROM stories s
WHERE NOT EXISTS (
    SELECT 1
    FROM story_entries se
    JOIN entries e ON e.id = se.entry_id
    WHERE se.story_id = s.id
      AND e.source_id NOT IN (SELECT id FROM sources WHERE driver_kind = 'annotations')
);

-- Mixed stories keep a remaining member as their representative.
UPDATE stories s
SET representative_entry_id = keeper.entry_id
FROM (
    SELECT DISTINCT ON (se.story_id) se.story_id, se.entry_id
    FROM story_entries se
    JOIN entries e ON e.id = se.entry_id
    WHERE e.source_id NOT IN (SELECT id FROM sources WHERE driver_kind = 'annotations')
) keeper
WHERE s.representative_entry_id IN (
    SELECT e.id FROM entries e JOIN sources src ON src.id = e.source_id
    WHERE src.driver_kind = 'annotations'
)
AND keeper.story_id = s.id;

DELETE FROM entries
WHERE source_id IN (SELECT id FROM sources WHERE driver_kind = 'annotations');

DELETE FROM sources WHERE driver_kind = 'annotations';

DROP TABLE entry_annotations;
