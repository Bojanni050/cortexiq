-- +goose Up
-- A goal row says where an intention stands, not how it got there: the
-- consolidator may have inferred progress from new facts, an agent may have
-- closed the goal at the end of a session, a human may have abandoned it in the
-- dashboard. Until now the only place for that was the notes column, which mixed
-- the reader's inference into the user's own text and could not be queried,
-- counted or undone.
--
-- Append-only on purpose: nothing here is updated, and the only delete is the
-- cascade when the goal itself is removed for good.
CREATE TABLE goal_events (
    id           BIGSERIAL PRIMARY KEY,
    goal_id      BIGINT NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
    namespace_id BIGINT NOT NULL,
    -- event is the vocabulary a reader sees: created, progress,
    -- suggested_complete, contradicted, completed, abandoned.
    event        TEXT NOT NULL,
    actor        TEXT NOT NULL,
    status_from  TEXT,
    status_to    TEXT,
    note         TEXT,
    confidence   REAL,
    -- fact_id is the newest fact the assessment rested on; it is what keeps the
    -- consolidator from writing the same conclusion twice for the same batch.
    fact_id      BIGINT,
    -- episode_id is the evidence for a completion an agent reported while
    -- remembering: the memory that closed the goal.
    episode_id   BIGINT,
    detail       JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Reading the log is almost always "the newest N in these namespaces", so the
-- index is ordered the way the query sorts.
CREATE INDEX goal_events_namespace_idx ON goal_events (namespace_id, created_at DESC);
CREATE INDEX goal_events_goal_idx ON goal_events (goal_id, id);

-- +goose Down
DROP TABLE IF EXISTS goal_events;
