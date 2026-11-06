-- +goose Up
-- A hypothesis states what might be true. Its counter states what might be true
-- *instead*, so that testing can tell the two apart instead of only looking for
-- agreement. The link is the whole point: without it a counter is just another
-- unrelated belief sitting next to the original.
ALTER TABLE hypotheses ADD COLUMN counter_of BIGINT REFERENCES hypotheses(id) ON DELETE SET NULL;

-- One counter per hypothesis. A second critic run would otherwise keep adding
-- alternatives to the same belief, and the log would fill with near-duplicates.
CREATE UNIQUE INDEX hypotheses_counter_unique ON hypotheses (counter_of) WHERE counter_of IS NOT NULL;

-- The critic is a fifth place a model can be chosen, so the binding table has to
-- allow it. Postgres has no "alter check", so it is dropped and recreated; the
-- name is the one Postgres generated for the inline check in 00039.
ALTER TABLE llm_bindings DROP CONSTRAINT llm_bindings_purpose_check;
ALTER TABLE llm_bindings ADD CONSTRAINT llm_bindings_purpose_check
    CHECK (purpose IN ('reasoner', 'triage', 'embedding', 'reranker', 'critic'));

-- +goose Down
-- Remove critic bindings first: the old constraint would reject them, and the
-- down migration has to leave a schema the previous version can run against.
DELETE FROM llm_bindings WHERE purpose = 'critic';
ALTER TABLE llm_bindings DROP CONSTRAINT llm_bindings_purpose_check;
ALTER TABLE llm_bindings ADD CONSTRAINT llm_bindings_purpose_check
    CHECK (purpose IN ('reasoner', 'triage', 'embedding', 'reranker'));

DROP INDEX IF EXISTS hypotheses_counter_unique;
ALTER TABLE hypotheses DROP COLUMN IF EXISTS counter_of;
