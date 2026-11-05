-- +goose Up
-- A fact now says which path wrote it and what that path rested on.
--
-- origin     the path: consolidation, hypothesis, failure_pattern, import, eval.
-- origin_ref what a non-consolidation path rested on: "hypothesis:42" or
--            "failure:7". Opaque text, never resolved. Facts that came from
--            episodes are linked in fact_sources as before.
--
-- NULL on both means the fact was written before this migration. It is not
-- backfilled: guessing the origin of an old fact would be a fabricated record.
ALTER TABLE facts ADD COLUMN origin     TEXT NULL;
ALTER TABLE facts ADD COLUMN origin_ref TEXT NULL;

-- +goose Down
ALTER TABLE facts DROP COLUMN IF EXISTS origin_ref;
ALTER TABLE facts DROP COLUMN IF EXISTS origin;
