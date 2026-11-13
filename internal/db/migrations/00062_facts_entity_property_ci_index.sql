-- +goose Up
-- Contradiction detection looks facts up by (entity, property) ignoring capitals and
-- surrounding spaces, so "CortexIQ" and "cortexiq" are one entity. The existing index
-- on the raw columns (migration 00013) cannot serve that lookup, so this one holds
-- the folded form. It covers exactly the rows the lookup reads: live facts that are
-- still valid.
CREATE INDEX facts_entity_property_ci
    ON facts (namespace_id, lower(btrim(entity)), lower(btrim(property)))
    WHERE entity IS NOT NULL AND property IS NOT NULL AND valid_until IS NULL AND deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS facts_entity_property_ci;
