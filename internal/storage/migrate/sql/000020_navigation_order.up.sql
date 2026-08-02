ALTER TABLE sources ADD COLUMN navigation_position integer;

WITH ranked AS (
    SELECT
        id,
        (row_number() OVER (ORDER BY lower(name), id) - 1)::integer AS navigation_position
    FROM sources
)
UPDATE sources
SET navigation_position = ranked.navigation_position
FROM ranked
WHERE sources.id = ranked.id;

ALTER TABLE sources ALTER COLUMN navigation_position SET NOT NULL;

CREATE INDEX sources_navigation_position_idx
    ON sources (navigation_position, id)
    WHERE archived_at IS NULL;

ALTER TABLE folders ADD COLUMN navigation_position integer;

WITH ranked AS (
    SELECT
        id,
        (row_number() OVER (ORDER BY lower(name), id) - 1)::integer AS navigation_position
    FROM folders
)
UPDATE folders
SET navigation_position = ranked.navigation_position
FROM ranked
WHERE folders.id = ranked.id;

ALTER TABLE folders ALTER COLUMN navigation_position SET NOT NULL;

CREATE INDEX folders_navigation_position_idx
    ON folders (navigation_position, id);

ALTER TABLE source_folders ADD COLUMN navigation_position integer;

WITH ranked AS (
    SELECT
        membership.source_id,
        membership.folder_id,
        (row_number() OVER (
            PARTITION BY membership.folder_id
            ORDER BY lower(source.name), source.id
        ) - 1)::integer AS navigation_position
    FROM source_folders AS membership
    JOIN sources AS source ON source.id = membership.source_id
)
UPDATE source_folders AS membership
SET navigation_position = ranked.navigation_position
FROM ranked
WHERE membership.source_id = ranked.source_id
  AND membership.folder_id = ranked.folder_id;

ALTER TABLE source_folders ALTER COLUMN navigation_position SET NOT NULL;

CREATE INDEX source_folders_navigation_position_idx
    ON source_folders (folder_id, navigation_position, source_id);
