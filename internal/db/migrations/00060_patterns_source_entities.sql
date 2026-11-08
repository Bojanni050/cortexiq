-- +goose Up
-- Which things a pattern is about. The sweep derives a statement from a cluster
-- of edges, and two clusters over the same set of sources can say the same thing
-- in different words; the content key only catches identical text. Storing the
-- sources makes "the same evidence in another wording" recognisable.
ALTER TABLE patterns ADD COLUMN source_entities TEXT[] NOT NULL DEFAULT '{}';

-- Existing rows carry the edges they cite, so their sources are recoverable and
-- the merge rule works on what is already in the table.
UPDATE patterns p
SET source_entities = COALESCE((
    SELECT array_agg(DISTINCT r.from_entity)
    FROM relationships r
    WHERE r.id = ANY(p.source_rel_ids)
), '{}');

CREATE INDEX patterns_source_entities_gin ON patterns USING GIN (source_entities);

-- +goose Down
DROP INDEX IF EXISTS patterns_source_entities_gin;
ALTER TABLE patterns DROP COLUMN source_entities;
