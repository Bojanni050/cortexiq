-- +goose Up
-- A retraction was keyed on the row it withdrew, so a value that was rejected
-- could be asserted again in a new write and come back as a fresh, active fact.
-- value_key is the key on the value instead: the normalised text of the
-- withdrawn claim, hashed (see valueKey in internal/brain). insertFact refuses
-- a fact whose key matches an active retraction in the same namespace.
--
-- NULL means no key: a retraction made before this migration (filled in by a
-- backfill at start-up), or a claim whose normalised text is empty. A claim
-- with nothing left to match must not tombstone everything else that is empty.
ALTER TABLE retractions ADD COLUMN value_key TEXT NULL;

-- A tombstone is lifted, not deleted: the retraction stays as the record of what
-- was withdrawn and why, and lifted_* record who took the block away and why.
-- A retraction is an active tombstone while lifted_at IS NULL.
ALTER TABLE retractions ADD COLUMN lifted_at  TIMESTAMPTZ NULL;
ALTER TABLE retractions ADD COLUMN lifted_by  TEXT        NULL;
ALTER TABLE retractions ADD COLUMN lift_reason TEXT       NULL;

CREATE INDEX retractions_value_key_active
    ON retractions (namespace_id, value_key)
    WHERE lifted_at IS NULL AND value_key IS NOT NULL;

-- A fact the tombstone refused is not stored, so without a count a tombstone
-- that fires looks exactly like a run that found nothing.
ALTER TABLE consolidation_runs ADD COLUMN facts_refused INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE consolidation_runs DROP COLUMN IF EXISTS facts_refused;
DROP INDEX IF EXISTS retractions_value_key_active;
ALTER TABLE retractions DROP COLUMN IF EXISTS lift_reason;
ALTER TABLE retractions DROP COLUMN IF EXISTS lifted_by;
ALTER TABLE retractions DROP COLUMN IF EXISTS lifted_at;
ALTER TABLE retractions DROP COLUMN IF EXISTS value_key;
