# Design: Fact Reflection (Stage 9)

> Status: proposal — not implemented. Written 2026-10-31, after studying how
> EverOS ([EverMind-AI/EverOS](https://github.com/EverMind-AI/EverOS)) solves the
> same problem in its Offline Memory Engine. This document ports the durable
> ideas onto CortexIQ's model; it does not copy EverOS's lossy narrative merges.

## Problem

CortexIQ deduplicates *identical* facts (consolidation dedup, `cortexiq dedupe`)
and resolves *contradicting* facts (auto-resolve + `retract`). What it never does
is merge *the same fact at different points in time*. A namespace accumulates:

```
fact 101  "MelodIQ supports PoYo v6 inspiration"        (observed 2026-09)
fact 412  "MelodIQ supports PoYo v6-family inspiration"  (observed 2026-10)
fact 553  "MelodIQ supports v5.5 and v6-family"          (observed 2026-10)
```

None of these are duplicates in the dedup sense, none are contradictions — they
are the *same truth maturing*. Recall returns all three; the answer picks one by
rank, not by correctness. The gap between the newest fact and the noise beneath
it grows with every consolidation run.

EverOS calls this problem Reflection and solves it offline: cluster → merge →
re-extract → deprecate, max 10 clusters per run, weekly by default, because
"each run is a lossy LLM merge" (docs/reflection.md in EverOS). The design below
adopts the *discipline* (small batches, soft-archive, provenance) and rejects the
*output shape* (a merged narrative episode). CortexIQ merges into a structured
fact, not a summary.

## Core idea

A new consolidation stage, running **after** the existing stages (it consumes
their output) and **offline** (never on the recall path):

```
existing stages 1–8
        │
        ▼
9. Fact Reflection: for each candidate cluster of facts about the same
   (entity, property), LLM-merge them into one current fact; the new fact
   cites all merged facts; the merged facts are soft-archived, not deleted.
```

Three rules carried over from EverOS, each mapped onto an existing CortexIQ
mechanism:

| EverOS rule | CortexIQ equivalent |
|---|---|
| Merged product carries `parent_type: cluster` | New fact's `episode_ids` = union of merged facts' `episode_ids`; a new `merged_from` column records the merged fact IDs |
| Originals are soft-archived, kept out of search, never deleted | Set `valid_until = now` on merged facts (existing mechanism, already excluded from recall) — identical to contradiction auto-resolve |
| Max 10 clusters per run, weekly, because every merge is lossy | `ReflectionClusterLimit` (default 10) and `ReflectionMinAge` — a fact is only eligible once it is at least N days old, so reflection never races fresh consolidation |

## What reflection is *not*

- **Not dedup.** Dedup fires on near-identical text within a store operation.
  Reflection fires on semantically related facts *across time*, after the fact
  was already accepted as distinct. Overlap is expected; reflection is the
  slower, deeper pass.
- **Not contradiction resolution.** Contradictions (A vs ¬A) are already handled
  with retraction semantics. Reflection handles A-then-A′ where A′ refines A.
  The LLM prompt must refuse to merge a pair it judges contradictory and emit
  `contradiction` instead, routing it to the existing resolve path.
- **Not a summary.** The merged fact keeps the structured fields (entity,
  property, canonical_type, confidence) of the newest member; older members
  only contribute evidence and wording.

## Mechanism

### Candidate selection (pure SQL, no LLM)

Group facts per namespace by `(entity, property)` where:

- `valid_until IS NULL` (still believed) and `known_until IS NULL` (not withdrawn);
- all members ≥ `ReflectionMinAge` days old (`created_at`);
- group size ≥ 2.

Order groups by the spread between oldest and newest member, largest spread
first — the groups aging worst are reflected first. Take at most
`ReflectionClusterLimit` groups per run. This mirrors EverOS "largest first" and
keeps one run cheap even on a large namespace.

### Merge (one LLM call per group)

`ReasonFactMerge(ctx, facts)` — a new reasoner method alongside
`ReasonStructured` / `ReasonRelationshipsBatch`, with its own prompt:

- Input: the group's facts as `[id, content, observed_at, episode excerpt?]` —
  episode excerpts only when the fact has one surviving source episode, capped
  at a few hundred chars, to bound the prompt.
- Output, JSON: either
  - `{"verdict": "merge", "content": "...", "basis_fact_id": <newest id>}`, or
  - `{"verdict": "distinct"}` (the facts are about different things after all), or
  - `{"verdict": "contradiction", "left": id, "right": id}` → hand off to the
    existing contradiction path and leave the group alone.
- The merged `content` must be grounded: it may only state what the member facts
    state. The same guard applies as in `groundedReflect` (mental_model.go:241):
    reject a merge whose output is not supported by its inputs.

### Commit (transactional, per group)

In one transaction:

1. Insert the new fact: content = merged content, structured fields copied from
   the basis (newest) fact, `episode_ids` = union of all members' episode_ids,
   `confidence = max(member confidences) − ε` (a merge is one observation short
   of certainty; ε ≈ 0.05), `valid_from = now`, plus new column `merged_from
   int[]` with the member fact IDs.
2. For every member: `valid_until = now`, `known_until = now` — the same soft
   archival that contradiction auto-resolve already performs. Nothing is
   deleted; `purge`/`retract`/`forget` on any member still works, and an
   `as_of` recall before the merge instant still returns the originals.

If the LLM call fails or the output fails the grounding check, the group is
skipped and logged — a reflection run degrades to "no reflection", never to
wrong memory.

### Observability

- `ConsolidationResult` gains: `FactsReflected`, `ReflectionGroupsSkipped`,
  `ReflectionContradictions` (merges refused because the pair contradicts).
- `consolidation_runs` error list already carries per-stage errors; reflection
  errors use the same channel with a `reflect:` prefix.
- CLI: `cortexiq reflect-facts <namespace> --dry-run` (report the candidate
  groups and what the merge would produce, store nothing) and the same without
  `--dry-run`. Same shape as `cortexiq dedupe`: report first, apply on request,
  and never run automatically until the stage has proven itself on live data —
  the same rollout lesson as the canonical_type guard (v0.1.16).

## Schema

One migration (`00042_add_fact_reflection.sql`):

```sql
ALTER TABLE facts ADD COLUMN merged_from BIGINT[] NULL;

COMMENT ON COLUMN facts.merged_from IS
'Fact IDs this fact was LLM-merged from during fact reflection. Members keep
their rows with valid_until/known_until set at the merge instant; this column
is the provenance link back to them.';
```

No new table: the audit story is the facts themselves plus `merged_from`, the
same append-only philosophy as `retractions` but without a separate ledger,
because the merged facts remain queryable. If reflection ever becomes
user-facing in the UI, a `fact_merge_events` table can be added then; it is not
needed to prove the stage works.

## Config

```
CORTEXIQ_REFLECTION_ENABLED          default false   (stage off until proven)
CORTEXIQ_REFLECTION_CLUSTER_LIMIT    default 10      (EverOS discipline: small runs)
CORTEXIQ_REFLECTION_MIN_AGE_DAYS     default 14      (never race fresh consolidation)
CORTEXIQ_REFLECTION_MIN_GROUP_SIZE   default 2
CORTEXIQ_REFLECTION_MIN_SPREAD_DAYS  default 7      (only groups that actually aged)
```

Namespace overrides via the existing `namespace_config` chain
(migration 00035) — a busy project namespace can reflect weekly while `/self`
stays monthly.

## Scheduling

No new daemon. Reflection runs as the tail of the normal consolidation pipeline
when enabled — same process, same LLM budget counter
(`MaxLLMCallsPerRun` counts its calls like every other stage). EverOS runs it
on cron weekly and warns against more; the CortexIQ equivalent of "weekly" is
simply that the schedule already ticks and the cluster limit caps the work per
tick. A dedicated cron can come later via `internal/schedule` if the tail
position proves too coupled.

## Test plan

1. **Selection (SQL, table-driven):** groups by (entity, property); respects
   min-age, min-size, min-spread; excludes valid_until/known_until-stamped
   facts; orders by spread; caps at cluster limit.
2. **Merge verdicts (reasoner fake):** merge / distinct / contradiction routes
   correctly; contradiction verdict leaves the group untouched and is routed to
   the existing contradiction path.
3. **Grounding guard:** a merge whose content states something absent from the
   members is rejected, group skipped, error logged.
4. **Commit (DB test):** new fact carries union episode_ids, copied structured
   fields, merged_from; members stamped valid_until/known_until; `as_of` recall
   before the merge still returns members; recall after returns only the merged
   fact.
5. **Failure isolation:** one bad group does not abort the run; checkpoint and
   other stages unaffected.
6. **Model eval:** add reflection merge to `internal/reasoner/modeleval_test.go`
   (`-tags modeleval`) — same harness that convicted Mistral-Small-24B. Gemma
   must merge true refinements and refuse true non-merges on a hand-built set,
   before `CORTEXIQ_REFLECTION_ENABLED` goes to true in production.

## What we deliberately do not take from EverOS

- **Lossy narrative output.** Their merged episode is a story ("Andrew's pet
  adoption journey…"). CortexIQ merges into structured facts with confidence
  and validity — recall must be able to answer "which PoYo versions" exactly.
- **Weekly hard-coded cadence.** Their cron is a workaround for having no other
  trigger. CortexIQ already has a scheduler and per-namespace config; cadence
  becomes a knob, not a constant.
- **Their markdown source of truth.** A separate, larger discussion (export/
  import as portable truth) — out of scope here; this stage works entirely on
  existing Postgres structures.
