-- Backfill relationships.canonical_type for the STRICT layer of the semantic mapping
-- (embedding similarity >= 0.85 with a clear margin to the runner-up; no negation,
-- no inverse). The original relation_type is never touched; types that are not
-- listed stay NULL.
--
-- 2026-11-09: keys are now compared NORMALISED (lowercase, whitespace and hyphens as
-- underscores). The same predicate written "pushed to" and "pushed_to" is one
-- identity, not two, which is what made the sweep see four and five sources for one
-- regularity instead of nine. Normalisation is mechanical: it carries no judgement,
-- so it can run over every type without a review.
--
-- Deliberately left out after a manual review (ambiguous role/direction/meaning, so they
-- stay NULL): adopts, avoids_taking, based_in, communicates_in, decided, develops_using, develops_with, has_value, ignored_in, invoked_in, made_decision, migrated_from, off_prevents, present_for, reading_requires, rejected, renamed_to, replaced, replaced_with, replacing, runs through, runs_during, runs_over, runs_through, selects, serves_at, triggers_on, works_as, works_on, writes.
-- The 2026-11-09 additions are listed with their reason in canon_left_out below, so
-- the decision is visible where it is applied instead of in a comment nobody reads.
--
-- Generated from the 2026-10-15 dry-run (bge-m3 over the prod relation types),
-- extended by hand on 2026-11-09 (the families that carry three or more sources).
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
    ('updated', 'writes'),
    -- 2026-11-09: families that carry three or more sources and were still NULL,
    -- so their cluster identity depended on what the model happened to suggest.
    -- Rule used: a word that names the OBJECT ("editor", "package manager") folds
    -- into the predicate it belongs to, because the object is already to_entity.
    ('used', 'uses'),
    ('used_as', 'uses'),
    ('uses_editor', 'uses'),
    ('uses_as_editor', 'uses'),
    ('uses_for_editing', 'uses'),
    ('uses_package_manager', 'uses'),
    ('uses_as_package_manager', 'uses'),
    ('uses_for_package_management', 'uses'),
    ('uses_transport', 'uses'),
    ('uses_flag', 'uses'),
    ('uses_via', 'uses'),
    ('broke', 'affects'),
    ('broken', 'affects'),
    ('involved', 'affects'),
    ('implements', 'implements'),
    ('implemented_via', 'implements'),
    ('include', 'contains'),
    ('had', 'has'),

    ('stored_in', 'part_of'),
    ('defined_in', 'part_of'),
    ('documented_in', 'part_of'),
    ('taken_from', 'derived_from'),
    ('built_on', 'depends_on'),
    ('sent_to', 'sends'),
    ('deployed', 'runs_on'),
    ('running', 'runs_on'),
    ('runs', 'runs_on');

-- Reviewed and deliberately left out. A predicate that says nothing about the
-- object, a value or a date, an observation about the research itself, or a wording
-- whose meaning depends on the object. The sweep must never build a candidate from
-- one of these, and the reason belongs next to the decision.
CREATE TEMP TABLE canon_left_out (relation_type TEXT PRIMARY KEY, reason TEXT NOT NULL) ON COMMIT DROP;
INSERT INTO canon_left_out (relation_type, reason) VALUES
    ('for',              'not a predicate: "X for Y" says nothing about the relation (71 sources, all noise)'),
    ('on',               'preposition; meaning depends on the object ("code on a branch" is not runs_on)'),
    ('to',               'empty preposition'),
    ('of',               'empty preposition'),
    ('via',              'empty preposition'),
    ('null',             'the extractor had no predicate at all'),
    ('was',              'meta: about the wording of a memory, not about the world'),
    ('observed',         'meta: what the agent noticed, not a property of the object'),
    ('applies',          'meta: whether a rule is in force'),
    ('performs',         'means "executes"; the vocabulary has no word for that, and every edge would become a false "manages"'),
    ('in',               'empty preposition: the existing rows already split between part_of and located_in'),
    ('reported',         'meta: who said it, not what is true'),
    ('lacks',            'negation: the edge says the relation is absent'),
    ('added',            'ambiguous: a field, a user or a file'),
    ('updated_to',       'ambiguous: a value or a state'),
    ('named',            'ambiguous: naming is not a relation between two things'),
    ('followed_by',      'ambiguous direction'),
    ('committed_as',     'ambiguous: a commit hash, a branch or an alias'),
    ('pushed_to',        'no canonical word fits; "sends" would be false for code pushed to a branch'),
    ('competitor_for',   'a relation between two products, not a property of one'),
    ('has_editor',       'possession read backwards; the direction is not fixed'),
    ('min_width',        'value, not a predicate'),
    ('max_width',        'value, not a predicate'),
    ('default_width',    'value, not a predicate'),
    ('released_on',      'date, not a predicate'),
    ('dated',            'date, not a predicate'),
    ('retrievable_once_within',       'value: a retention window'),
    ('retrievable_only_once_within',  'value: a retention window'),
    ('retrievable_through',           'value: a transport detail'),
    ('accessible_via',   'meaning depends on the object: a port, a URL or a tool'),
    ('accessible_through', 'meaning depends on the object'),
    ('available_through',  'meaning depends on the object'),
    ('accessible_only_via', 'meaning depends on the object'),
    ('added_to',           'ambiguous: added to a list, a file or a config'),
    ('passed',             'ambiguous: a test, a value or a message');

\echo '== mapping rows / canonical types in the map / left out with a reason'
SELECT (SELECT count(*) FROM canon_map) AS mapped_types,
       (SELECT count(DISTINCT canonical_type) FROM canon_map) AS canonical_types,
       (SELECT count(*) FROM canon_left_out) AS left_out;

\echo '== types in the map that do not occur in relationships (stale entries, harmless)'
SELECT count(*) AS unused FROM canon_map m
WHERE NOT EXISTS (
  SELECT 1 FROM relationships r
  WHERE lower(regexp_replace(btrim(r.relation_type), '[[:space:]-]+', '_', 'g'))
      = lower(regexp_replace(btrim(m.relation_type), '[[:space:]-]+', '_', 'g'))
);

-- One row per normalised type: how much evidence it carries and what we propose to
-- do with it. This is the review surface: a bucket per type, and every type that is
-- neither mapped nor left out shows up as UNMAPPED so it cannot hide.
\echo '== per type: edges, distinct sources, proposed bucket'
WITH live AS (
  SELECT id, from_entity,
         lower(regexp_replace(btrim(relation_type), '[[:space:]-]+', '_', 'g')) AS norm_type
  FROM relationships
  WHERE deleted_at IS NULL AND canonical_type IS NULL
)
SELECT l.norm_type,
       count(*) AS edges,
       count(DISTINCT l.from_entity) AS sources,
       coalesce(
         (SELECT string_agg(m.canonical_type, '/') FROM canon_map m
           WHERE lower(regexp_replace(btrim(m.relation_type), '[[:space:]-]+', '_', 'g')) = l.norm_type),
         'LEFT OUT: ' || (SELECT lo.reason FROM canon_left_out lo WHERE lo.relation_type = l.norm_type),
         'UNMAPPED'
       ) AS bucket
FROM live l
GROUP BY l.norm_type
ORDER BY count(DISTINCT l.from_entity) DESC, count(*) DESC, l.norm_type
LIMIT 60;

\echo '== how many live rows would be filled, and how many stay NULL'
WITH live AS (
  SELECT canonical_type,
         lower(regexp_replace(btrim(relation_type), '[[:space:]-]+', '_', 'g')) AS norm_type
  FROM relationships WHERE deleted_at IS NULL
)
SELECT count(*) FILTER (WHERE canonical_type IS NULL
          AND norm_type IN (SELECT lower(regexp_replace(btrim(m.relation_type), '[[:space:]-]+', '_', 'g')) FROM canon_map m)) AS would_fill,
       count(*) FILTER (WHERE canonical_type IS NULL) AS still_null,
       count(*) AS live_rows
FROM live;

\if :apply
  \echo '== APPLY'
  -- The hand-map beats the model's suggestion, exactly as CanonicalRelationType
  -- does at extraction time: a mapped type whose rows disagree with the map would
  -- otherwise stay half old and half new, which freezes the coin toss instead of
  -- removing it.
  UPDATE relationships r SET canonical_type = m.canonical_type
  FROM canon_map m
  WHERE r.canonical_type IS DISTINCT FROM m.canonical_type
    AND lower(regexp_replace(btrim(r.relation_type), '[[:space:]-]+', '_', 'g'))
      = lower(regexp_replace(btrim(m.relation_type), '[[:space:]-]+', '_', 'g'));

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
