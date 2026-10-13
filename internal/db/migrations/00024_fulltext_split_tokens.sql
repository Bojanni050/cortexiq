-- +goose Up
-- The 'simple' parser keeps hostnames, URLs and paths as single tokens
-- ("server.example.ts.net:8445/ui", "GUI/dashboard"), so searching for "8445" or
-- "dashboard" missed them. Split on everything that is not a letter or digit
-- before indexing, which matches how the query side tokenizes.
ALTER TABLE facts DROP COLUMN content_tsv;
ALTER TABLE facts ADD COLUMN content_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', regexp_replace(content, '[^[:alnum:]]+', ' ', 'g'))) STORED;
CREATE INDEX facts_content_tsv_idx ON facts USING GIN (content_tsv);

ALTER TABLE episodes DROP COLUMN content_tsv;
ALTER TABLE episodes ADD COLUMN content_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', regexp_replace(content, '[^[:alnum:]]+', ' ', 'g'))) STORED;
CREATE INDEX episodes_content_tsv_idx ON episodes USING GIN (content_tsv);
