-- +goose Up
-- Tags are a second, orthogonal axis next to namespaces: a namespace says
-- where a memory belongs (it scopes recall and consolidation), a tag says what
-- it is about. Agents attach them on write; facts inherit the union of the tags
-- of the episodes they were consolidated from, so filtering keeps working after
-- the raw episode is out of the way.
ALTER TABLE episodes ADD COLUMN tags TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE facts    ADD COLUMN tags TEXT[] NOT NULL DEFAULT '{}';

CREATE INDEX episodes_tags_gin ON episodes USING gin (tags) WHERE deleted_at IS NULL;
CREATE INDEX facts_tags_gin    ON facts    USING gin (tags) WHERE deleted_at IS NULL;

-- Listing the vocabulary an agent may reuse must not scan both tables fully.
CREATE INDEX episodes_tagged_ns ON episodes (namespace_id) WHERE deleted_at IS NULL AND cardinality(tags) > 0;
CREATE INDEX facts_tagged_ns    ON facts    (namespace_id) WHERE deleted_at IS NULL AND cardinality(tags) > 0;

-- +goose Down
DROP INDEX IF EXISTS facts_tagged_ns;
DROP INDEX IF EXISTS episodes_tagged_ns;
DROP INDEX IF EXISTS facts_tags_gin;
DROP INDEX IF EXISTS episodes_tags_gin;
ALTER TABLE facts    DROP COLUMN IF EXISTS tags;
ALTER TABLE episodes DROP COLUMN IF EXISTS tags;
