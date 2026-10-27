-- +goose Up
-- Hygiene finds noise in memory: terminal output, heartbeat pings, stack
-- traces, trivial acknowledgements and leaked secrets. The audit is read-only
-- and deterministic (regex and arithmetic, no LLM). Cleaning archives
-- (importance 0, reversible) or flags (metadata only); delete is a separate
-- explicit action via purge.

-- The audit log records every candidate the cleaner looked at, including the
-- ones it left alone, so a run is reproducible and reviewable afterwards.
CREATE TABLE hygiene_audit_log (
    id                      BIGSERIAL   PRIMARY KEY,
    namespace_id            BIGINT      NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    target_table            TEXT        NOT NULL CHECK (target_table IN ('episodes', 'facts')),
    target_id               BIGINT      NOT NULL,
    action                  TEXT        NOT NULL CHECK (action IN ('archived', 'flagged', 'kept')),
    reasons                 TEXT[]      NOT NULL DEFAULT '{}',
    noise_score             REAL        NOT NULL CHECK (noise_score >= 0 AND noise_score <= 1),
    secret_labels           TEXT[]      NOT NULL DEFAULT '{}',
    content_preview         TEXT        NOT NULL DEFAULT '',
    original_importance     REAL        NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON hygiene_audit_log (namespace_id, created_at DESC);
CREATE INDEX ON hygiene_audit_log (target_table, target_id);
CREATE INDEX ON hygiene_audit_log (created_at DESC);

-- Archive markers live on the memory row itself so recall can exclude archived
-- rows without joining the audit log. Archived rows keep their content and stay
-- searchable; they only drop out of importance-weighted ranking.
ALTER TABLE episodes ADD COLUMN hygiene_archived BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE episodes ADD COLUMN hygiene_archived_at TIMESTAMPTZ NULL;
ALTER TABLE episodes ADD COLUMN hygiene_archive_reason TEXT NULL;
ALTER TABLE episodes ADD COLUMN hygiene_original_importance REAL NULL;

ALTER TABLE facts ADD COLUMN hygiene_archived BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE facts ADD COLUMN hygiene_archived_at TIMESTAMPTZ NULL;
ALTER TABLE facts ADD COLUMN hygiene_archive_reason TEXT NULL;
ALTER TABLE facts ADD COLUMN hygiene_original_importance REAL NULL;

-- A flag is metadata only: the row is unchanged, but the label is visible so a
-- human can review it (typically because a secret was detected).
ALTER TABLE episodes ADD COLUMN hygiene_flagged BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE episodes ADD COLUMN hygiene_flag_reason TEXT NULL;
ALTER TABLE facts ADD COLUMN hygiene_flagged BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE facts ADD COLUMN hygiene_flag_reason TEXT NULL;

CREATE INDEX ON episodes (hygiene_archived) WHERE hygiene_archived = TRUE;
CREATE INDEX ON facts (hygiene_archived) WHERE hygiene_archived = TRUE;
CREATE INDEX ON episodes (hygiene_flagged) WHERE hygiene_flagged = TRUE;
CREATE INDEX ON facts (hygiene_flagged) WHERE hygiene_flagged = TRUE;

-- +goose Down
DROP INDEX IF EXISTS facts_hygiene_flagged;
DROP INDEX IF EXISTS episodes_hygiene_flagged;
DROP INDEX IF EXISTS facts_hygiene_archived;
DROP INDEX IF EXISTS episodes_hygiene_archived;

ALTER TABLE facts DROP COLUMN IF EXISTS hygiene_flag_reason;
ALTER TABLE facts DROP COLUMN IF EXISTS hygiene_flagged;
ALTER TABLE facts DROP COLUMN IF EXISTS hygiene_original_importance;
ALTER TABLE facts DROP COLUMN IF EXISTS hygiene_archive_reason;
ALTER TABLE facts DROP COLUMN IF EXISTS hygiene_archived_at;
ALTER TABLE facts DROP COLUMN IF EXISTS hygiene_archived;

ALTER TABLE episodes DROP COLUMN IF EXISTS hygiene_flag_reason;
ALTER TABLE episodes DROP COLUMN IF EXISTS hygiene_flagged;
ALTER TABLE episodes DROP COLUMN IF EXISTS hygiene_original_importance;
ALTER TABLE episodes DROP COLUMN IF EXISTS hygiene_archive_reason;
ALTER TABLE episodes DROP COLUMN IF EXISTS hygiene_archived_at;
ALTER TABLE episodes DROP COLUMN IF EXISTS hygiene_archived;

DROP TABLE IF EXISTS hygiene_audit_log;
