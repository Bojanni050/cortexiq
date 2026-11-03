-- +goose Up
-- external_key lets a writer address "its" memory by an id of its own
-- ("gaia:hypothesis:42", "jira:ABC-123") instead of by content. A second write
-- with the same key is then either a no-op (same content) or a superseding
-- version (see Brain.RememberWithOutcome), never a duplicate. Only a live
-- episode holds a key: when it is superseded or withdrawn it is soft-deleted,
-- which frees the key for its successor.
ALTER TABLE episodes ADD COLUMN external_key TEXT NULL;

-- external_refs are pointers to the thing a memory came from that lives outside
-- CortexIQ ("commit:bf575f8", "foundation:9f3a", a URL). They are opaque: stored
-- and returned verbatim, never resolved. Not to be confused with fact_sources,
-- which links a fact to the episodes it was consolidated from.
ALTER TABLE episodes ADD COLUMN external_refs TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE facts    ADD COLUMN external_refs TEXT[] NOT NULL DEFAULT '{}';

-- The episode that replaced this one under the same external_key. NULL for an
-- episode that was never superseded (including one that was merely forgotten
-- or retracted).
ALTER TABLE episodes ADD COLUMN superseded_by BIGINT NULL REFERENCES episodes(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX episodes_external_key_live
    ON episodes (namespace_id, external_key)
    WHERE external_key IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS episodes_external_key_live;
ALTER TABLE episodes DROP COLUMN IF EXISTS superseded_by;
ALTER TABLE facts    DROP COLUMN IF EXISTS external_refs;
ALTER TABLE episodes DROP COLUMN IF EXISTS external_refs;
ALTER TABLE episodes DROP COLUMN IF EXISTS external_key;
