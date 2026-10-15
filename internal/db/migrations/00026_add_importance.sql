-- +goose Up
-- Importance (0..1) is how much a memory matters, set by the caller at remember
-- time and used as an optional recall ranking factor. 0.5 is neutral, so existing
-- rows and callers that say nothing keep ranking exactly as before.
ALTER TABLE episodes ADD COLUMN importance REAL NOT NULL DEFAULT 0.5 CHECK (importance >= 0 AND importance <= 1);
ALTER TABLE facts ADD COLUMN importance REAL NOT NULL DEFAULT 0.5 CHECK (importance >= 0 AND importance <= 1);

-- +goose Down
ALTER TABLE facts DROP COLUMN importance;
ALTER TABLE episodes DROP COLUMN importance;
