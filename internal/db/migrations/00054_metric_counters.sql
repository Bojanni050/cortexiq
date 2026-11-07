-- +goose Up
-- Prometheus counters live in the process and start at zero after every
-- restart, so a deploy reads as a drop in usage on the dashboard. The process
-- that records the consolidation and reasoner counters snapshots its totals
-- here and restores them at start-up, which keeps the numbers cumulative.
--
-- Only counter series are stored (see internal/observability/persist.go);
-- gauges and the consolidation duration histogram are left to the process.
CREATE TABLE metric_counters (
    name       TEXT             NOT NULL,
    labels     TEXT             NOT NULL,
    value      DOUBLE PRECISION NOT NULL,
    updated_at TIMESTAMPTZ      NOT NULL DEFAULT now(),
    PRIMARY KEY (name, labels)
);

-- +goose Down
DROP TABLE IF EXISTS metric_counters;
