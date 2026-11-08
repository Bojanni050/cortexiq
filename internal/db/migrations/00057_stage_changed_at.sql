-- +goose Up
-- A checkpoint that stops moving looks exactly like a stage that has nothing to
-- do: both leave the same number behind. This records when each checkpoint last
-- changed, so "there is work waiting and nothing has moved for six hours" becomes
-- a query instead of a guess — which is how a batch the model keeps failing on
-- starves every fact behind it without anyone noticing.
--
-- One JSONB map of stage name to timestamp, updated only for the stages whose
-- checkpoint actually moved. Empty for a namespace that has never run.
ALTER TABLE consolidation_progress ADD COLUMN stage_changed_at JSONB NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE consolidation_progress DROP COLUMN IF EXISTS stage_changed_at;
