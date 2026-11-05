-- +goose Up
-- Confidence and reachability shared one number. Disuse multiplied confidence
-- down, and a fact whose confidence fell under the expiry threshold was ended:
-- a true fact that nobody asked about for a while looked, to the store, exactly
-- like a fact the evidence had turned against.
--
-- strength is the reachability half: 1.0 for a fact that is in use, falling when
-- nobody observes or recalls it. It is the only thing disuse touches once
-- CORTEXIQ_DECAY_SPLIT is on. Confidence keeps meaning "how well is this
-- supported" and moves on evidence alone.
--
-- Every existing fact starts at 1.0. With the split off nothing reads or writes
-- strength except reinforcement, which resets it to 1.0.
ALTER TABLE facts ADD COLUMN strength REAL NOT NULL DEFAULT 1.0;

-- +goose Down
ALTER TABLE facts DROP COLUMN IF EXISTS strength;
