-- +goose Up
-- Full-text search for hybrid recall. The 'simple' configuration is used on
-- purpose: it does no stemming or stop-word removal, so it behaves the same for
-- Dutch and English content (and for names, versions and ports).
ALTER TABLE facts ADD COLUMN content_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED;
CREATE INDEX facts_content_tsv_idx ON facts USING GIN (content_tsv);

ALTER TABLE episodes ADD COLUMN content_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED;
CREATE INDEX episodes_content_tsv_idx ON episodes USING GIN (content_tsv);
