-- +goose Up
-- Per-model refresh and source settings. The defaults reproduce the behaviour
-- from before this migration: automatic refresh after consolidation at most once
-- an hour (NULL), grounded on 20 facts and episodes of the model's namespace.
ALTER TABLE mental_models
    ADD COLUMN auto_refresh                 BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN min_refresh_interval_seconds INT     NULL,
    ADD COLUMN source_tags                  TEXT[]  NOT NULL DEFAULT '{}',
    ADD COLUMN source_types                 TEXT[]  NOT NULL DEFAULT '{}',
    ADD COLUMN source_limit                 INT     NOT NULL DEFAULT 20;

-- Content a refresh or a clear replaced, so a rewrite that lost something can be
-- read back. based_on travels with it: a purge has to find the versions that
-- were written from the purged memory.
CREATE TABLE mental_model_versions (
    id              BIGSERIAL   PRIMARY KEY,
    mental_model_id BIGINT      NOT NULL REFERENCES mental_models(id) ON DELETE CASCADE,
    content         TEXT        NOT NULL,
    source_query    TEXT        NOT NULL,
    based_on        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    refreshed_at    TIMESTAMPTZ NULL,
    replaced_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    replaced_by     TEXT        NOT NULL
);

CREATE INDEX ON mental_model_versions (mental_model_id, id DESC);

-- +goose Down
DROP TABLE IF EXISTS mental_model_versions;
ALTER TABLE mental_models
    DROP COLUMN IF EXISTS source_limit,
    DROP COLUMN IF EXISTS source_types,
    DROP COLUMN IF EXISTS source_tags,
    DROP COLUMN IF EXISTS min_refresh_interval_seconds,
    DROP COLUMN IF EXISTS auto_refresh;
