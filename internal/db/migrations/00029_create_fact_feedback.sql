-- +goose Up
-- Feedback on a single fact: the one signal the evidence model cannot see.
-- Confidence only knows "observed again" and "time passed"; a fact that was
-- repeated but is wrong keeps its confidence forever, and recall_count knows a
-- fact surfaced, not whether it helped. Votes are stored per event for audit,
-- but only each principal's latest vote counts, so a caller that flaps between
-- helpful and wrong cannot outvote everyone else by voting more often.
CREATE TABLE fact_feedback (
    id           BIGSERIAL   PRIMARY KEY,
    fact_id      BIGINT      NOT NULL REFERENCES facts(id) ON DELETE CASCADE,
    namespace_id BIGINT      NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    principal    TEXT        NOT NULL,
    verdict      SMALLINT    NOT NULL CHECK (verdict IN (-1, 1)),
    note         TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON fact_feedback (fact_id, principal, created_at DESC);

-- +goose Down
DROP TABLE fact_feedback;
