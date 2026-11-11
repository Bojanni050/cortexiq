-- +goose Up
-- Keyword recall moves from ts_rank_cd over a stored tsvector to real BM25
-- (pg_textsearch). ts_rank_cd has no IDF and no length normalisation, and it
-- scores every matching row before the LIMIT, so its cost grows with the corpus:
-- on 500k synthetic rows a common term cost 160-230 ms against 4-6 ms for BM25.
--
-- The index is built on the same expression the tsvector used (everything that
-- is not a letter or digit becomes a space), so hostnames, URLs and paths still
-- split into searchable tokens, and the 'simple' configuration still does no
-- stemming or stop-word removal: it behaves the same for Dutch and English.
--
-- The recall queries must repeat this exact expression and the index name, or
-- the planner cannot use the index.
--
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'pg_textsearch') THEN
        RAISE EXCEPTION 'pg_textsearch is not installed: CortexIQ needs PostgreSQL 17+ with the pg_textsearch extension preloaded (see deploy/postgres/Dockerfile)';
    END IF;
END $$;
-- +goose StatementEnd

CREATE EXTENSION IF NOT EXISTS pg_textsearch;

CREATE INDEX facts_content_bm25 ON facts
    USING bm25 ((regexp_replace(content, '[^[:alnum:]]+', ' ', 'g')))
    WITH (text_config = 'simple');
CREATE INDEX episodes_content_bm25 ON episodes
    USING bm25 ((regexp_replace(content, '[^[:alnum:]]+', ' ', 'g')))
    WITH (text_config = 'simple');

-- The stored tsvector and its GIN index are no longer read by recall. Failures
-- and goals build theirs on the fly and never used these columns.
DROP INDEX IF EXISTS facts_content_tsv_idx;
DROP INDEX IF EXISTS episodes_content_tsv_idx;
ALTER TABLE facts DROP COLUMN content_tsv;
ALTER TABLE episodes DROP COLUMN content_tsv;

-- +goose Down
ALTER TABLE facts ADD COLUMN content_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', regexp_replace(content, '[^[:alnum:]]+', ' ', 'g'))) STORED;
CREATE INDEX facts_content_tsv_idx ON facts USING GIN (content_tsv);
ALTER TABLE episodes ADD COLUMN content_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', regexp_replace(content, '[^[:alnum:]]+', ' ', 'g'))) STORED;
CREATE INDEX episodes_content_tsv_idx ON episodes USING GIN (content_tsv);

DROP INDEX IF EXISTS facts_content_bm25;
DROP INDEX IF EXISTS episodes_content_bm25;
