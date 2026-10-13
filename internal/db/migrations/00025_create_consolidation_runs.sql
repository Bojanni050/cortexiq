-- +goose Up
CREATE TABLE consolidation_runs (
    id                          BIGSERIAL   PRIMARY KEY,
    namespace_id                BIGINT      NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    namespace_slug              TEXT        NOT NULL DEFAULT '',
    started_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration_ms                 BIGINT      NOT NULL DEFAULT 0,
    episodes_read               INT         NOT NULL DEFAULT 0,
    facts_created               INT         NOT NULL DEFAULT 0,
    facts_deduplicated          INT         NOT NULL DEFAULT 0,
    relationships_found         INT         NOT NULL DEFAULT 0,
    causal_links_found          INT         NOT NULL DEFAULT 0,
    patterns_found              INT         NOT NULL DEFAULT 0,
    contradictions_found        INT         NOT NULL DEFAULT 0,
    goals_annotated             INT         NOT NULL DEFAULT 0,
    goals_suggested_complete    INT         NOT NULL DEFAULT 0,
    failure_repeats_detected    INT         NOT NULL DEFAULT 0,
    failure_patterns_found      INT         NOT NULL DEFAULT 0,
    hypotheses_auto_confirmed   INT         NOT NULL DEFAULT 0,
    hypotheses_auto_rejected    INT         NOT NULL DEFAULT 0,
    hypotheses_updated          INT         NOT NULL DEFAULT 0,
    facts_decayed               INT         NOT NULL DEFAULT 0,
    facts_expired               INT         NOT NULL DEFAULT 0,
    llm_calls                   INT         NOT NULL DEFAULT 0,
    llm_calls_limited           BOOLEAN     NOT NULL DEFAULT false,
    errors                      INT         NOT NULL DEFAULT 0,
    error_sample                TEXT        NOT NULL DEFAULT '',
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX consolidation_runs_namespace_started_idx ON consolidation_runs (namespace_id, started_at DESC);
CREATE INDEX consolidation_runs_started_idx ON consolidation_runs (started_at DESC);
