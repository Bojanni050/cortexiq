-- +goose Up
-- The shape sweep asks the model about one regularity candidate at a time:
-- a relation type pointing at the same target from several distinct sources.
-- This table is its cursor. A candidate is not asked about again until edges
-- were added to it, so a namespace with hundreds of stable clusters does not
-- pay for the same answer on every tick.
CREATE TABLE pattern_clusters (
    id            BIGSERIAL   PRIMARY KEY,
    namespace_id  BIGINT      NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    signature     TEXT        NOT NULL,
    relation_type TEXT        NOT NULL,
    target        TEXT        NOT NULL,
    sources_seen  INT         NOT NULL,
    max_rel_id    BIGINT      NOT NULL,
    verdict       TEXT        NOT NULL CHECK (verdict IN ('pattern', 'rejected', 'error')),
    last_run_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (namespace_id, signature)
);

CREATE INDEX pattern_clusters_stale ON pattern_clusters (namespace_id, max_rel_id);

-- +goose Down
DROP TABLE pattern_clusters;
