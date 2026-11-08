-- +goose Up
-- A pattern is a statement about several facts, so the same statement must not
-- exist twice in one namespace. Until now it could: a consolidation run that
-- failed after this stage never saved its checkpoint, so the next tick derived
-- the same patterns from the same facts and inserted them again. One namespace
-- accumulated 1260 copies of 200 statements that way.
--
-- content_key is the comparison form: trimmed, lowercased, whitespace collapsed,
-- so a rephrasing with different spacing still collides.
ALTER TABLE patterns ADD COLUMN content_key TEXT
    GENERATED ALWAYS AS (regexp_replace(lower(btrim(content)), '\s+', ' ', 'g')) STORED;

-- Clean up what the fan-out left behind before the index can go on: keep the row
-- with the most evidence (then the highest confidence, then the newest), and
-- retire the rest as tombstones so the history is not destroyed.
WITH ranked AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY namespace_id, content_key
               ORDER BY cardinality(source_fact_ids) DESC, confidence DESC, id DESC
           ) AS rn
    FROM patterns
    WHERE deleted_at IS NULL
)
UPDATE patterns p
SET deleted_at = now(), updated_at = now()
FROM ranked r
WHERE p.id = r.id AND r.rn > 1;

-- From here on the database itself refuses a duplicate, so a stage that runs
-- twice (or two workers racing) cannot recreate the mess.
CREATE UNIQUE INDEX patterns_content_key_uniq
    ON patterns (namespace_id, content_key)
    WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS patterns_content_key_uniq;
ALTER TABLE patterns DROP COLUMN IF EXISTS content_key;
