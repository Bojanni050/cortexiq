-- +goose Up
-- A belief is only as strong as the number of independent things that point at
-- it. source_fact_ids records what the belief was BUILT from; it never grows,
-- so a hypothesis that was proposed on a hunch two months ago looks exactly
-- like one that has been corroborated ten times since.
--
-- supported_by is the union of everything that has ever argued for the belief:
-- the facts it was created from, plus the facts in every batch that came back
-- with a "supports" verdict. Cardinality is the proof count.
ALTER TABLE hypotheses ADD COLUMN supported_by BIGINT[] NOT NULL DEFAULT '{}';

-- Existing beliefs start from what they were built on, so the column is never
-- empty for a row that already had sources.
UPDATE hypotheses SET supported_by = source_fact_ids WHERE source_fact_ids <> '{}';

-- +goose Down
ALTER TABLE hypotheses DROP COLUMN IF EXISTS supported_by;
