-- +goose Up
ALTER TABLE consolidation_progress ADD COLUMN last_causal_fact_id BIGINT NOT NULL DEFAULT 0;
