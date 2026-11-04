-- +goose Up
-- A forget, and an invalidate without a reason, withdraw a memory without saying
-- whether it was true. They left no record at all: nobody could read back who
-- did it or when. They now write a retraction row too, with judged = false: an
-- audit entry, not a judgement.
--
-- Only a judged retraction is a rejection. An unjudged row has no value_key and
-- so cannot block a value from coming back; a forget says "stop using this", not
-- "this was wrong". The CHECK makes that a property of the table, not of the
-- code that happens to write it.
ALTER TABLE retractions ADD COLUMN judged BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE retractions
    ADD CONSTRAINT retractions_unjudged_has_no_key CHECK (judged OR value_key IS NULL);

-- +goose Down
ALTER TABLE retractions DROP CONSTRAINT IF EXISTS retractions_unjudged_has_no_key;
ALTER TABLE retractions DROP COLUMN IF EXISTS judged;
