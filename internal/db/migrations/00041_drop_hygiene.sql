-- +goose Up
-- Removes the hygiene subsystem that 00040 introduced: the audit log, the
-- archive markers and the flag columns on episodes and facts.
--
-- Why it goes: the store already answers the question hygiene was built for.
-- Confidence decay lowers facts nobody observes, reinforcement raises facts
-- that keep coming back, and purge/retract handle removal. The archive half was
-- also half-wired — nothing ever read hygiene_archived, so an "archived" memory
-- kept scoring exactly as before. What survives is the deterministic scorer,
-- now only as the detector behind the write filter (internal/noise).
--
-- Dropping a column drops its indexes, so there is nothing to name here. On a
-- database that never applied 00040 every statement is a no-op.
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

-- +goose Down
-- Intentionally empty. Recreating the tables and columns would produce empty
-- shells: the archive markers and audit rows they held are gone, and the code
-- that wrote them is gone with them. A down migration here would only pretend
-- to restore state that no longer exists.
