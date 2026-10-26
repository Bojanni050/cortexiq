-- +goose Up
-- Providers and the model chosen for each purpose. The API key lives here rather
-- than in the environment because the dashboard can change it at runtime, and it
-- is write-only: no query in the application returns it to a caller.
CREATE TABLE llm_providers (
    id BIGSERIAL PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    base_url TEXT NOT NULL,
    api_key TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One row per purpose. A purpose without a row keeps whatever the environment
-- configured, so an empty table changes nothing.
CREATE TABLE llm_bindings (
    purpose TEXT PRIMARY KEY CHECK (purpose IN ('reasoner', 'triage', 'embedding', 'reranker')),
    provider_slug TEXT NOT NULL REFERENCES llm_providers(slug) ON DELETE CASCADE,
    model TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE llm_bindings;
DROP TABLE llm_providers;
