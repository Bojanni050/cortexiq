-- +goose Up
-- A memory that a client handed over must not be lost because the process died
-- between the request and the write. This table is the queue: the row exists
-- before the caller is told "accepted", so a crash leaves recoverable work
-- instead of a promise nobody kept.
--
-- It is a queue, not a log: finished rows are pruned after a retention window.
-- The payload is the full remember request as asked (already passed through the
-- noise filter and secret redaction), so the worker runs exactly what the caller
-- meant, and a restart does not need the client to ask again.
CREATE TABLE write_jobs (
    id              BIGSERIAL PRIMARY KEY,
    namespace_id    BIGINT,
    namespace_slug  TEXT NOT NULL,
    -- actor is the principal that asked, so provenance and the audit trail still
    -- name a person (or a token) when the write lands minutes later.
    actor           TEXT NOT NULL,
    payload         JSONB NOT NULL,
    -- state is queued, running, done, skipped or failed. skipped is a refusal
    -- the gates would have given the caller too (noise, social closer), not an
    -- error.
    state           TEXT NOT NULL DEFAULT 'queued',
    attempts        INT NOT NULL DEFAULT 0,
    -- next_attempt_at postpones a retry, so a failing job cannot spin.
    next_attempt_at TIMESTAMPTZ,
    episode_id      BIGINT,
    error           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ
);

-- The claim walks queued work in id order; the partial index keeps that cheap as
-- finished rows pile up.
CREATE INDEX write_jobs_queue_idx ON write_jobs (id) WHERE state IN ('queued', 'running');
-- Serializing per namespace needs "who is running in this namespace" fast.
CREATE INDEX write_jobs_running_idx ON write_jobs (namespace_id) WHERE state = 'running';
-- Listing and pruning finished work reads newest first.
CREATE INDEX write_jobs_created_idx ON write_jobs (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS write_jobs;
