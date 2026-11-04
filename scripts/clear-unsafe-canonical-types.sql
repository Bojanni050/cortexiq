-- Clear canonical_type on relationships whose type was suggested by the extraction model
-- and that the guard in internal/reasoner/relvocab.go (SuggestionAllowed) would now refuse:
-- negation ("lacks", "does_not_include"), passive or reachability wording ("used_by",
-- "accessible_at") and a reversed direction ("included_in" filed as contains).
--
-- Only rows whose relation_type is NOT in the hand-reviewed mapping and is not itself a
-- vocabulary word are touched; relation_type is never changed. Idempotent. The three
-- patterns below are copied from the Go constants (TestGuardPatternsMatchCleanupScript
-- fails when they drift apart).
--
-- Usage (from /opt/cortexiq):
--   1. dry run (changes nothing):
--        docker compose -f docker-compose.prod.yml exec -T postgres psql -U cortexiq -d cortexiq -v apply=0 < scripts/clear-unsafe-canonical-types.sql
--   2. apply (single transaction, prints before/after counts):
--        docker compose -f docker-compose.prod.yml exec -T postgres psql -U cortexiq -d cortexiq -v apply=1 < scripts/clear-unsafe-canonical-types.sql
\set ON_ERROR_STOP on
\pset format aligned

BEGIN;

CREATE TEMP TABLE known_types (relation_type TEXT PRIMARY KEY) ON COMMIT DROP;
INSERT INTO known_types (relation_type) VALUES
    ('accepts'),
    ('allows'),
    ('avoids'),
    ('belongs_to'),
    ('blocks'),
    ('breaks'),
    ('build'),
    ('builds'),
    ('calls'),
    ('captured'),
    ('captures'),
    ('captures_via'),
    ('caused'),
    ('causes'),
    ('checks'),
    ('clears_field'),
    ('comes_from'),
    ('communicates'),
    ('configured_in'),
    ('configured_with'),
    ('configures'),
    ('connected_to'),
    ('connected_via'),
    ('connects'),
    ('connects_over'),
    ('connects_through'),
    ('connects_to'),
    ('connects_via'),
    ('consists_of'),
    ('consumes'),
    ('contain'),
    ('contains'),
    ('contains_field'),
    ('contains_memory'),
    ('contains_option'),
    ('converts'),
    ('creates'),
    ('creates_with'),
    ('defines'),
    ('deletes'),
    ('deletes_data_from'),
    ('deletes_from'),
    ('depends_on'),
    ('deployed_in'),
    ('deployed_on'),
    ('derived_from'),
    ('develops'),
    ('displays'),
    ('displays_in'),
    ('enables'),
    ('ensures'),
    ('excludes'),
    ('explains'),
    ('exposes'),
    ('fetches'),
    ('forwards'),
    ('generates'),
    ('had_version'),
    ('handle'),
    ('handles'),
    ('has'),
    ('has_attribute'),
    ('has_field'),
    ('has_property'),
    ('has_state'),
    ('has_status'),
    ('has_version'),
    ('holds'),
    ('hosted_on'),
    ('hosts'),
    ('ignored'),
    ('ignores'),
    ('impacted'),
    ('implemented_in'),
    ('includes'),
    ('instance_of'),
    ('integrated_into'),
    ('integrates_via'),
    ('integrates_with'),
    ('intended_for'),
    ('invokes'),
    ('invokes_via'),
    ('is'),
    ('is_a'),
    ('is_at'),
    ('is_deployed_at'),
    ('is_deployed_on'),
    ('is_for'),
    ('is_hosted_in'),
    ('launches'),
    ('leads_to'),
    ('links'),
    ('lives_at'),
    ('lives_in'),
    ('loads'),
    ('located_at'),
    ('located_in'),
    ('located_on'),
    ('logs'),
    ('logs_for'),
    ('made'),
    ('maintains'),
    ('makes'),
    ('manages'),
    ('mapped_to'),
    ('maps_to'),
    ('monitors'),
    ('needed'),
    ('offers'),
    ('originates_from'),
    ('outputs'),
    ('owns'),
    ('parses'),
    ('part_of'),
    ('points_at'),
    ('points_to'),
    ('possess'),
    ('posts_at'),
    ('posts_to'),
    ('prevents'),
    ('produce'),
    ('produces'),
    ('provides'),
    ('pursues'),
    ('ran_on'),
    ('ran_under'),
    ('read'),
    ('read_in'),
    ('reads'),
    ('reads_from'),
    ('reads_using'),
    ('received'),
    ('receives'),
    ('references'),
    ('replaces'),
    ('required'),
    ('required_for'),
    ('requires'),
    ('requires_env'),
    ('resides_at'),
    ('resides_in'),
    ('result_in'),
    ('results_in'),
    ('retrieves'),
    ('return'),
    ('returned'),
    ('returns'),
    ('run_in'),
    ('run_on'),
    ('run_under'),
    ('running_on'),
    ('runs_at'),
    ('runs_in'),
    ('runs_on'),
    ('runs_under'),
    ('sends'),
    ('sends_at'),
    ('sends_through'),
    ('sends_using'),
    ('sends_via'),
    ('serves'),
    ('set_up_with'),
    ('sets'),
    ('specifies'),
    ('status'),
    ('supports'),
    ('target'),
    ('targets'),
    ('tracks'),
    ('transmits'),
    ('transmits_at'),
    ('transmits_to'),
    ('triggers'),
    ('triggers_action'),
    ('updated'),
    ('used_alongside'),
    ('used_in'),
    ('used_with'),
    ('uses'),
    ('uses_for'),
    ('verified_on'),
    ('verifies'),
    ('was_deployed_on'),
    ('works_alongside'),
    ('works_with'),
    ('yields');

CREATE TEMP TABLE to_clear ON COMMIT DROP AS
SELECT id, relation_type, canonical_type FROM (
  SELECT id, relation_type, canonical_type,
         regexp_replace(lower(btrim(relation_type)), '[[:space:]-]+', '_', 'g') AS norm
  FROM relationships
  WHERE deleted_at IS NULL AND canonical_type IS NOT NULL
) r
WHERE norm NOT IN (SELECT relation_type FROM known_types)
  AND norm NOT IN ('accepts', 'affects', 'calls', 'configures', 'connects_to', 'contains', 'creates', 'deletes', 'depends_on', 'derived_from', 'describes', 'displays', 'has', 'has_status', 'hosts', 'implements', 'is_a', 'located_in', 'logs', 'manages', 'owns', 'part_of', 'prefers', 'prevents', 'produces', 'reads', 'references', 'replaces', 'runs_on', 'sends', 'supports', 'targets', 'transforms', 'uses', 'validates', 'writes')
  AND (
       norm ~ '(^|_)(not|no|never|without|cannot|cant|non|nothing|none|null|lack|lacks|lacking|missing|unable|fail|fails|failed)(_|$)'
    OR norm ~ '(_by|_under)$|^(accessible|reachable)'
    OR (norm ~ '_(in|within)$' AND canonical_type IN ('accepts', 'affects', 'calls', 'configures', 'contains', 'creates', 'deletes', 'describes', 'displays', 'has', 'hosts', 'implements', 'logs', 'manages', 'owns', 'prefers', 'prevents', 'produces', 'reads', 'replaces', 'sends', 'supports', 'targets', 'transforms', 'uses', 'validates', 'writes'))
  );

\echo '== rows that would lose their canonical_type (model suggestions the guard refuses)'
SELECT count(*) AS rows_to_clear,
       (SELECT count(*) FROM relationships WHERE deleted_at IS NULL AND canonical_type IS NOT NULL) AS currently_mapped
FROM to_clear;

\echo '== per relation_type -> canonical_type'
SELECT relation_type, canonical_type, count(*) AS rows FROM to_clear GROUP BY 1, 2 ORDER BY 3 DESC, 1;

\if :apply
  \echo '== APPLY'
  UPDATE relationships r SET canonical_type = NULL FROM to_clear c WHERE r.id = c.id;

  \echo '== after'
  SELECT count(*) FILTER (WHERE canonical_type IS NOT NULL) AS mapped,
         count(*) FILTER (WHERE canonical_type IS NULL) AS unmapped,
         count(*) AS total
  FROM relationships WHERE deleted_at IS NULL;
  COMMIT;
\else
  \echo '== DRY RUN: nothing changed (rolled back). Re-run with -v apply=1 to apply.'
  ROLLBACK;
\endif
