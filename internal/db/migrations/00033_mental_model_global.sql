-- +goose Up
-- A mental model without a namespace is global: it is synthesized from every
-- namespace and is available to reflect from any namespace. Models created
-- before this change keep the namespace they were filed under and stay scoped
-- to that namespace's subtree.
ALTER TABLE mental_models ALTER COLUMN namespace_id DROP NOT NULL;

-- +goose Down
-- Global models did not exist before this migration, so there is no scoped
-- namespace to put them back into. Drop them rather than inventing one.
DELETE FROM mental_models WHERE namespace_id IS NULL;
ALTER TABLE mental_models ALTER COLUMN namespace_id SET NOT NULL;
