-- +goose Up

-- A retraction says a stored claim was wrong. The claim itself is kept in the
-- row (content) on purpose: the point of a retraction is that you can still
-- see what was withdrawn, and why. This is the opposite of a purge, which
-- leaves no content behind at all.
CREATE TABLE retractions (
    id           BIGSERIAL   PRIMARY KEY,
    namespace_id BIGINT      NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    target_type  TEXT        NOT NULL CHECK (target_type IN ('episode', 'fact')),
    target_id    BIGINT      NOT NULL,
    content      TEXT        NOT NULL,
    reason       TEXT        NOT NULL,
    retracted_by TEXT        NOT NULL DEFAULT 'unknown',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON retractions (namespace_id, created_at DESC);
CREATE INDEX ON retractions (target_type, target_id);

-- A purge is a hard delete, so the row it describes no longer exists. The log
-- records that something was removed, by whom and why, without keeping the
-- content: that is the whole difference with retractions.
CREATE TABLE purge_log (
    id           BIGSERIAL   PRIMARY KEY,
    namespace_id BIGINT      NULL REFERENCES namespaces(id) ON DELETE SET NULL,
    target_table TEXT        NOT NULL,
    target_id    BIGINT      NOT NULL,
    reason       TEXT        NOT NULL,
    requested_by TEXT        NOT NULL DEFAULT 'unknown',
    details      JSONB       NULL,
    purged_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON purge_log (purged_at DESC);
CREATE INDEX ON purge_log (target_table, target_id);

-- Purging a fact that confirmed a hypothesis failed with a foreign-key
-- violation: the reference had no ON DELETE rule, so it defaulted to NO
-- ACTION. A purged fact must take its confirmation link with it, not block the
-- purge.
ALTER TABLE hypotheses DROP CONSTRAINT hypotheses_confirmed_fact_id_fkey;
ALTER TABLE hypotheses ADD CONSTRAINT hypotheses_confirmed_fact_id_fkey
    FOREIGN KEY (confirmed_fact_id) REFERENCES facts(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE hypotheses DROP CONSTRAINT hypotheses_confirmed_fact_id_fkey;
ALTER TABLE hypotheses ADD CONSTRAINT hypotheses_confirmed_fact_id_fkey
    FOREIGN KEY (confirmed_fact_id) REFERENCES facts(id);
DROP TABLE IF EXISTS purge_log;
DROP TABLE IF EXISTS retractions;
