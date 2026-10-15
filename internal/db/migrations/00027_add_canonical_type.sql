-- +goose Up
-- canonical_type is a normalised relation type from a small fixed vocabulary
-- (uses, has, part_of, ...). relation_type keeps the extractor's own wording;
-- canonical_type stays NULL when no confident mapping exists, so nothing is lost
-- and filtering on it only ever sees reliable mappings.
ALTER TABLE relationships ADD COLUMN canonical_type TEXT NULL;
CREATE INDEX ON relationships (namespace_id, canonical_type) WHERE canonical_type IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS relationships_namespace_id_canonical_type_idx;
ALTER TABLE relationships DROP COLUMN canonical_type;
