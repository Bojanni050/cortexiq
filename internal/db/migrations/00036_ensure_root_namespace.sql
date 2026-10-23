-- +goose Up
-- The root namespace is the documented default target for a memory that was
-- filed nowhere in particular: the MCP remember tool defaults to "/" and its
-- description promises root "for truly uncategorized memories". No install ever
-- created that row, so every write without an explicit namespace died with
-- "namespace not found" — invisible in hosted clients such as Claude's
-- connector, which only reports a generic connector error.
--
-- Creating the row is safe: resolveNamespaceIDWithDescendants("/") keeps
-- answering all namespaces, and a "/" argument to mental models still means
-- global (NULL namespace).
INSERT INTO namespaces (slug, name, description)
VALUES ('/', 'Root', 'Memories that belong to no more specific namespace.')
ON CONFLICT (slug) DO NOTHING;

-- +goose Down
-- This is data, not schema: episodes may already be filed under the root
-- namespace, so a rollback leaves the row in place instead of orphaning them.
SELECT 1;
