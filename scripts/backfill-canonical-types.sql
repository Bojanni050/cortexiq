-- Backfill relationships.canonical_type for the STRICT layer of the semantic mapping
-- (embedding similarity >= 0.85 with a clear margin to the runner-up; no negation,
-- no inverse). 190 relation types -> 35 canonical types. The original
-- relation_type is never touched; types that are not listed stay NULL.
--
-- Deliberately left out after a manual review (ambiguous role/direction/meaning, so they
-- stay NULL): adopts, avoids_taking, based_in, communicates_in, decided, develops_using, develops_with, has_value, ignored_in, invoked_in, made_decision, migrated_from, off_prevents, present_for, reading_requires, rejected, renamed_to, replaced, replaced_with, replacing, runs through, runs_during, runs_over, runs_through, selects, serves_at, triggers_on, works_as, works_on, writes.
--
-- Generated from the 2026-10-15 dry-run (bge-m3 over the prod relation types).
-- Idempotent: only rows with canonical_type IS NULL are filled.
--
-- Usage (from /opt/cortexiq):
--   1. dry run (changes nothing):
--        docker compose -f docker-compose.prod.yml exec -T postgres psql -U cortexiq -d cortexiq -v apply=0 < scripts/backfill-canonical-types.sql
--   2. apply (single transaction, prints before/after counts):
--        docker compose -f docker-compose.prod.yml exec -T postgres psql -U cortexiq -d cortexiq -v apply=1 < scripts/backfill-canonical-types.sql
\set ON_ERROR_STOP on
\pset format aligned

BEGIN;

CREATE TEMP TABLE canon_map (relation_type TEXT PRIMARY KEY, canonical_type TEXT NOT NULL) ON COMMIT DROP;
INSERT INTO canon_map (relation_type, canonical_type) VALUES
    ('accepts', 'accepts'),
    ('consumes', 'accepts'),
    ('received', 'accepts'),
    ('receives', 'accepts'),
    ('caused', 'affects'),
    ('causes', 'affects'),
    ('results_in', 'affects'),
    ('breaks', 'affects'),
    ('leads_to', 'affects'),
    ('impacted', 'affects'),
    ('result_in', 'affects'),
    ('launches', 'calls'),
    ('invokes', 'calls'),
    ('calls', 'calls'),
    ('triggers', 'calls'),
    ('invokes_via', 'calls'),
    ('triggers_action', 'calls'),
    ('configured_with', 'configures'),
    ('configured_in', 'configures'),
    ('configures', 'configures'),
    ('set_up_with', 'configures'),
    ('works_with', 'connects_to'),
    ('connects_to', 'connects_to'),
    ('connects', 'connects_to'),
    ('connects_via', 'connects_to'),
    ('integrates_with', 'connects_to'),
    ('connected_via', 'connects_to'),
    ('connects_through', 'connects_to'),
    ('integrated_into', 'connects_to'),
    ('integrates_via', 'connects_to'),
    ('works_alongside', 'connects_to'),
    ('communicates', 'connects_to'),
    ('connected_to', 'connects_to'),
    ('connects_over', 'connects_to'),
    ('contains', 'contains'),
    ('includes', 'contains'),
    ('consists_of', 'contains'),
    ('contain', 'contains'),
    ('contains_field', 'contains'),
    ('contains_option', 'contains'),
    ('contains_memory', 'contains'),
    ('holds', 'contains'),
    ('creates', 'creates'),
    ('develops', 'creates'),
    ('builds', 'creates'),
    ('build', 'creates'),
    ('creates_with', 'creates'),
    ('made', 'creates'),
    ('makes', 'creates'),
    ('deletes', 'deletes'),
    ('deletes_from', 'deletes'),
    ('clears_field', 'deletes'),
    ('deletes_data_from', 'deletes'),
    ('requires', 'depends_on'),
    ('required', 'depends_on'),
    ('depends_on', 'depends_on'),
    ('needed', 'depends_on'),
    ('required_for', 'depends_on'),
    ('requires_env', 'depends_on'),
    ('derived_from', 'derived_from'),
    ('originates_from', 'derived_from'),
    ('comes_from', 'derived_from'),
    ('derived from', 'derived_from'),
    ('defines', 'describes'),
    ('specifies', 'describes'),
    ('explains', 'describes'),
    ('displays', 'displays'),
    ('displays_in', 'displays'),
    ('has', 'has'),
    ('has_field', 'has'),
    ('has_property', 'has'),
    ('has-property', 'has'),
    ('has_attribute', 'has'),
    ('possess', 'has'),
    ('has_status', 'has_status'),
    ('status', 'has_status'),
    ('has_version', 'has_status'),
    ('had_version', 'has_status'),
    ('has_state', 'has_status'),
    ('hosts', 'hosts'),
    ('serves', 'hosts'),
    ('implemented_in', 'implements'),
    ('implemented in', 'implements'),
    ('is_a', 'is_a'),
    ('is', 'is_a'),
    ('instance_of', 'is_a'),
    ('located_in', 'located_in'),
    ('located_at', 'located_in'),
    ('located_on', 'located_in'),
    ('lives_in', 'located_in'),
    ('is at', 'located_in'),
    ('lives_at', 'located_in'),
    ('located-in', 'located_in'),
    ('resides_at', 'located_in'),
    ('resides_in', 'located_in'),
    ('logs', 'logs'),
    ('captured', 'logs'),
    ('captures', 'logs'),
    ('captures_via', 'logs'),
    ('logs_for', 'logs'),
    ('monitors', 'logs'),
    ('tracks', 'logs'),
    ('handles', 'manages'),
    ('handle', 'manages'),
    ('manages', 'manages'),
    ('owns', 'owns'),
    ('maintains', 'owns'),
    ('part_of', 'part_of'),
    ('belongs_to', 'part_of'),
    ('ignores', 'prevents'),
    ('avoids', 'prevents'),
    ('excludes', 'prevents'),
    ('prevents', 'prevents'),
    ('blocks', 'prevents'),
    ('ignored', 'prevents'),
    ('returns', 'produces'),
    ('generates', 'produces'),
    ('produces', 'produces'),
    ('returned', 'produces'),
    ('yields', 'produces'),
    ('outputs', 'produces'),
    ('produce', 'produces'),
    ('return', 'produces'),
    ('reads', 'reads'),
    ('read_in', 'reads'),
    ('fetches', 'reads'),
    ('loads', 'reads'),
    ('read', 'reads'),
    ('reads_from', 'reads'),
    ('retrieves', 'reads'),
    ('reads_using', 'reads'),
    ('references', 'references'),
    ('points_to', 'references'),
    ('links', 'references'),
    ('points_at', 'references'),
    ('replaces', 'replaces'),
    ('runs_on', 'runs_on'),
    ('hosted_on', 'runs_on'),
    ('runs_in', 'runs_on'),
    ('runs_under', 'runs_on'),
    ('deployed_on', 'runs_on'),
    ('ran_under', 'runs_on'),
    ('run_under', 'runs_on'),
    ('runs_at', 'runs_on'),
    ('deployed_in', 'runs_on'),
    ('is deployed at', 'runs_on'),
    ('is deployed on', 'runs_on'),
    ('is_hosted_in', 'runs_on'),
    ('ran_on', 'runs_on'),
    ('run_in', 'runs_on'),
    ('run_on', 'runs_on'),
    ('running_on', 'runs_on'),
    ('was_deployed_on', 'runs_on'),
    ('sends', 'sends'),
    ('forwards', 'sends'),
    ('posts_to', 'sends'),
    ('transmits_to', 'sends'),
    ('sends_through', 'sends'),
    ('sends_via', 'sends'),
    ('transmits', 'sends'),
    ('posts_at', 'sends'),
    ('sends_at', 'sends'),
    ('sends_using', 'sends'),
    ('transmits_at', 'sends'),
    ('exposes', 'supports'),
    ('provides', 'supports'),
    ('enables', 'supports'),
    ('offers', 'supports'),
    ('supports', 'supports'),
    ('allows', 'supports'),
    ('pursues', 'targets'),
    ('targets', 'targets'),
    ('is_for', 'targets'),
    ('intended_for', 'targets'),
    ('target', 'targets'),
    ('mapped_to', 'transforms'),
    ('maps_to', 'transforms'),
    ('parses', 'transforms'),
    ('converts', 'transforms'),
    ('uses', 'uses'),
    ('used_alongside', 'uses'),
    ('used_with', 'uses'),
    ('used_in', 'uses'),
    ('uses_for', 'uses'),
    ('verified_on', 'validates'),
    ('checks', 'validates'),
    ('ensures', 'validates'),
    ('verifies', 'validates'),
    ('sets', 'writes'),
    ('updated', 'writes');

\echo '== mapping rows / canonical types in the map'
SELECT count(*) AS mapped_types, count(DISTINCT canonical_type) AS canonical_types FROM canon_map;

\echo '== types in the map that do not occur in relationships (stale entries, harmless)'
SELECT count(*) AS unused FROM canon_map m
WHERE NOT EXISTS (SELECT 1 FROM relationships r WHERE r.relation_type = m.relation_type);

\echo '== rows that WOULD be filled (live rows, canonical_type still NULL)'
SELECT count(*) AS rows_to_fill,
       (SELECT count(*) FROM relationships WHERE deleted_at IS NULL) AS live_rows
FROM relationships r JOIN canon_map m USING (relation_type)
WHERE r.deleted_at IS NULL AND r.canonical_type IS NULL;

\echo '== per canonical type: rows and distinct source types'
SELECT m.canonical_type, count(*) AS rows, count(DISTINCT r.relation_type) AS source_types
FROM relationships r JOIN canon_map m USING (relation_type)
WHERE r.deleted_at IS NULL AND r.canonical_type IS NULL
GROUP BY 1 ORDER BY 2 DESC;

\if :apply
  \echo '== APPLY'
  UPDATE relationships r SET canonical_type = m.canonical_type
  FROM canon_map m
  WHERE r.relation_type = m.relation_type AND r.canonical_type IS NULL;

  \echo '== after'
  SELECT count(*) FILTER (WHERE canonical_type IS NOT NULL) AS filled,
         count(*) FILTER (WHERE canonical_type IS NULL) AS still_null,
         count(*) AS total
  FROM relationships WHERE deleted_at IS NULL;
  COMMIT;
\else
  \echo '== DRY RUN: nothing changed (rolled back). Re-run with -v apply=1 to apply.'
  ROLLBACK;
\endif
