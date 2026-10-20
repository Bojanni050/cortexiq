-- +goose Up
-- known_until is the end of a fact's knowledge (transaction-time) interval: the
-- moment the system stopped holding this belief (superseded by a newer fact, or
-- expired by decay). created_at is the start of that interval. NULL means the
-- belief is still held. It is separate from valid_until, which describes when
-- the fact was true in the world and can be supplied by the caller.
ALTER TABLE facts ADD COLUMN known_until TIMESTAMPTZ NULL;

-- Backfill only rows the system itself ended. Both write paths (auto-supersede
-- and decay expiry) stamp valid_until and updated_at with the same instant, and
-- auto-supersede also records resolved_at with it. A caller-supplied
-- valid_until (a validity date, not a withdrawn belief) matches neither
-- signature and stays NULL, which is exact: that belief was never withdrawn.
UPDATE facts f SET known_until = f.valid_until
WHERE f.valid_until IS NOT NULL
  AND f.valid_until >= f.created_at
  AND (f.valid_until = f.updated_at
       OR EXISTS (SELECT 1 FROM contradictions c
                  WHERE c.old_fact_id = f.id
                    AND c.resolution = 'superseded'
                    AND c.resolved_at = f.valid_until));

CREATE INDEX ON facts (namespace_id, created_at, known_until);

-- +goose Down
DROP INDEX IF EXISTS facts_namespace_id_created_at_known_until_idx;
ALTER TABLE facts DROP COLUMN known_until;
