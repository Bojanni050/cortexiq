-- +goose Up
-- A fact can be ended for several reasons: superseded by a newer value,
-- invalidated, or because nobody used it for a long time. Only the last one is
-- reversible by observation: the fact was never wrong, it just went quiet, so
-- seeing it again is the same fact coming back, not a new one.
--
-- expired_by_disuse_at marks that case, and only it. NULL on an ended fact means
-- it ended for another reason (or before this migration) and stays ended; seeing
-- the same claim again then creates a new fact, as it always did.
ALTER TABLE facts ADD COLUMN expired_by_disuse_at TIMESTAMPTZ NULL;

-- +goose Down
ALTER TABLE facts DROP COLUMN IF EXISTS expired_by_disuse_at;
