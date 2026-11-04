-- +goose Up
-- The query that produced the stored content. When the model is edited to a
-- different source query, the content no longer answers the current question;
-- comparing the two makes that visible as stale_reason "source_changed"
-- instead of leaving a summary that silently describes the wrong topic.
ALTER TABLE mental_models
    ADD COLUMN last_refreshed_source_query TEXT NOT NULL DEFAULT '';

UPDATE mental_models
   SET last_refreshed_source_query = source_query
 WHERE last_refreshed_at IS NOT NULL;

-- +goose Down
ALTER TABLE mental_models DROP COLUMN IF EXISTS last_refreshed_source_query;
