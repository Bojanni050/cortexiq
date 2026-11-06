-- +goose Up
-- The run history answers "what did that consolidation actually do?". Now that a
-- run can propose hypotheses of its own, a run that quietly invented two beliefs
-- must be visible there and not only in the hypothesis log.
ALTER TABLE consolidation_runs ADD COLUMN hypotheses_proposed INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE consolidation_runs DROP COLUMN IF EXISTS hypotheses_proposed;
