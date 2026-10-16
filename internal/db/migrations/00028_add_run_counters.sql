-- +goose Up
-- Two counters that only lived in the result of a run and never reached the
-- history: episodes the extractor skipped because they state nothing, and facts
-- stored as new while a near-twin already sat just below the dedup threshold.
-- The second one is the band where duplicates are born, so it is the number the
-- dedup dimension of the quality evaluation needs to argue with.
ALTER TABLE consolidation_runs ADD COLUMN episodes_skipped INT NOT NULL DEFAULT 0;
ALTER TABLE consolidation_runs ADD COLUMN dedup_near_misses INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE consolidation_runs DROP COLUMN dedup_near_misses;
ALTER TABLE consolidation_runs DROP COLUMN episodes_skipped;
