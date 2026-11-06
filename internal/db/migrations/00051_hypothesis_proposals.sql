-- +goose Up
-- Proposals need their own checkpoint. The evidence stage advances
-- last_hypothesis_fact_id every run, so sharing it would leave the proposal
-- stage with nothing new to look at: it would read the same facts forever or
-- none at all.
ALTER TABLE consolidation_progress
    ADD COLUMN last_hypothesis_proposal_fact_id BIGINT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE consolidation_progress DROP COLUMN IF EXISTS last_hypothesis_proposal_fact_id;
