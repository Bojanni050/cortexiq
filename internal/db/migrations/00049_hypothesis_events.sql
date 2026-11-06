-- +goose Up
-- A hypothesis row shows where a belief ended up, not how it got there: the
-- consolidator may have confirmed it on evidence, an agent may have proposed it,
-- or a human may have rejected it in the dashboard. Without that trail a
-- hypothesis is unfalsifiable after the fact — you cannot tell a belief that
-- survived scrutiny from one that was never looked at.
--
-- Append-only on purpose: nothing here is updated, and the only delete is the
-- cascade when the hypothesis itself is removed for good.
CREATE TABLE hypothesis_events (
    id              BIGSERIAL PRIMARY KEY,
    hypothesis_id   BIGINT NOT NULL REFERENCES hypotheses(id) ON DELETE CASCADE,
    namespace_id    BIGINT NOT NULL,
    event           TEXT NOT NULL,
    actor           TEXT NOT NULL,
    status_from     TEXT,
    status_to       TEXT NOT NULL,
    confidence_from REAL,
    confidence_to   REAL,
    reason          TEXT,
    fact_id         BIGINT,
    detail          JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Reading the log is almost always "the newest N in these namespaces", so the
-- index is ordered the way the query sorts.
CREATE INDEX hypothesis_events_namespace_idx ON hypothesis_events (namespace_id, created_at DESC);
CREATE INDEX hypothesis_events_hypothesis_idx ON hypothesis_events (hypothesis_id, id);

-- +goose Down
DROP TABLE IF EXISTS hypothesis_events;
