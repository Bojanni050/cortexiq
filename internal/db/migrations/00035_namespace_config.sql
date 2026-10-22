-- +goose Up
-- Per-namespace configuration overrides. Every key is optional; a namespace
-- without a row (or with an empty overrides object) inherits the global
-- default. A namespace on the path to the target also contributes: the nearest
-- ancestor that sets a key wins, so /clients can override for every client
-- while /clients/white-vision overrides just that one.
CREATE TABLE namespace_configs (
    namespace_id BIGINT      PRIMARY KEY REFERENCES namespaces(id) ON DELETE CASCADE,
    overrides    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS namespace_configs;
