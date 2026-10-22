-- +goose Up
-- Mental models move from "regenerated on a timer" to "refreshed when their
-- ground truth changed". last_memory_seen_at is the snapshot watermark: the
-- database clock taken before the reflect that produced the current content.
-- A model is stale when any memory in its namespace scope changed after it.
-- based_on records which memories the synthesis actually read, so a fact that
-- is later deleted marks the model stale even when nothing new arrives.
ALTER TABLE mental_models
    ADD COLUMN last_memory_seen_at    TIMESTAMPTZ NULL,
    ADD COLUMN last_refresh_failed_at TIMESTAMPTZ NULL,
    ADD COLUMN last_refresh_error     TEXT        NULL,
    ADD COLUMN based_on               JSONB       NOT NULL DEFAULT '{}'::jsonb;

-- Existing models predate the watermark. Stamping them with their last refresh
-- keeps the first consolidation after the upgrade from re-synthesizing every
-- model at once; a model only refreshes once something actually changed.
UPDATE mental_models
   SET last_memory_seen_at = COALESCE(last_refreshed_at, created_at)
 WHERE last_memory_seen_at IS NULL;

CREATE INDEX idx_mental_models_namespace_watermark
    ON mental_models (namespace_id, last_memory_seen_at)
    WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_mental_models_namespace_watermark;
ALTER TABLE mental_models
    DROP COLUMN IF EXISTS based_on,
    DROP COLUMN IF EXISTS last_refresh_error,
    DROP COLUMN IF EXISTS last_refresh_failed_at,
    DROP COLUMN IF EXISTS last_memory_seen_at;
