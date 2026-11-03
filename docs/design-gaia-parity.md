# Design: Gaia-Cloud Parity

> Status: proposal — not implemented. Written 2026-11-02, after auditing what
> Gaia-Cloud (`Bojanni050/Gaia-Cloud`, `services/gaia-api/src/hindsightClient.js`
> and `docs/memory-pipe.md`) actually depends on in Hindsight, and checking that
> dependency list against CortexIQ's current code. The earlier list of gaps
> (reranking, reflect, graph arm, temporal recall, MMR) is closed — those exist
> in `internal/reranker`, `internal/brain/recall.go`, `internal/brain/mmr.go`.
> What remains is infrastructure, not intelligence, and every item below is a
> port of an API shape, not an algorithm.

## Problem

Gaia-Cloud cannot swap `hindsightClient.js` for CortexIQ until six dependencies
are covered. Each one is named from Gaia's own code, with the current CortexIQ
state and the proposed port. Dependency 2 (tags) was found already built when
this document was reviewed on 2026-11-03; Phase 2 below is reduced to the one
remaining gap.

| # | Gaia dependency | Where in Gaia-Cloud | CortexIQ today |
|---|---|---|---|
| 1 | Banks + cross-bank recall with per-bank quota | `createCrossBankRecallClient`, `CROSS_BANK_QUOTA = 3` | namespaces fuse into one RRF list on one scale |
| 2 | Arbitrary tags per item, readable in recall output | `memoryKindOf` reads item tags; `gaia:*` tags | **done** (migration 00037): tags on episodes and facts, accepted by `remember`, returned by `recall`, AND-filter; only an ANY-filter is open |
| 3 | Superseding version writes + rebuildable mirror | `cognitionSync.js`, `reconcile-hindsight-tags.js` | superseding facts and `forget` exist; no sync operation |
| 4 | Provenance `sources: ['foundation:<id>']` | `evidenceRecall.js` Dual Recall | no per-item external pointer (`fact_sources` links fact→episode only; stored here as `external_refs`, see Phase 4) |
| 5 | Async fire-and-forget retain | `reflectOnTurn` (`async: true, timeout: null`) | `POST /api/v1/remember` is synchronous |
| 6 | Server-enforced `agent_identity` | `memory.js` (identity always Gaia) | no equivalent |

Phases 1–6 are ordered by how hard Gaia is blocked without them: 1 and 2 block
the read side entirely, 3 and 4 block the write side, 5 and 6 are operational.

## Phase 1 — Banks: cross-bank recall with per-bank quota

### Why

Gaia recalls from three banks (`bojan`, `gaia`, `gaia-logos`). The banks' scores
are *not on one scale*, so a global sort lets one loud bank drown the others.
Gaia's contract: each bank speaks first (quota), the remainder follows in
global score order. CortexIQ's recall runs one query across namespaces and
fuses everything with RRF — by design one scale. That is a different retrieval
behavior, not a missing parameter.

### Design

A **bank is a namespace with its own score scale and a quota**. No new table:
banks are an addressing mode on top of namespaces, so existing data migrates
for free.

`POST /api/v1/recall` gains a `banks` field:

```json
{
  "query": "wat weet je over de outages in maart",
  "banks": [
    {"namespace": "/people/bojan",   "quota": 3},
    {"namespace": "/self/gaia",      "quota": 3},
    {"namespace": "/projects/gaia-logos", "quota": 3}
  ],
  "limit": 9
}
```

Merge algorithm. Steps 1, 2 and 4 are the contract Gaia relies on. Step 3
(interleave by rank position) is **our assumption**, not a verified port: read
`createCrossBankRecallClient` in Gaia-Cloud and align step 3 with what it
actually does before building.

1. Run the existing per-namespace recall pipeline **once per bank**, in
   parallel. RRF, rerank, temporal and MMR all stay *within* a bank — that is
   what makes bank scores incomparable, and why they must not be globally
   sorted up front.
2. **Quota round:** emit up to `quota` items from each bank, in each bank's own
   rank order. This guarantees "each bank speaks before any bank repeats,"
   independent of absolute scores.
3. **Remainder round:** after all banks spent their quota, remaining items
   follow in global order *within* each bank's score scale, interleaved by
   rank position (bank A's #4, bank B's #4, ...), until `limit` is reached.
4. Never compare raw scores across banks at any point.

Every result item gains a `bank` field so the caller can render the epistemic
mark per bank (Gaia renders `gaia-logos` items as "confirmed by the person",
`gaia` items as her own memory). The trace gains a `banks` section with
per-bank duration and returned count, mirroring the existing arm traces.

Validation: extend `internal/brain/recall_test.go` with a fixture where bank A
would win a global sort 8-of-9 and assert the quota contract (3/3/3 first).

## Phase 2 — Tags on episodes and facts (mostly done)

### Why

Gaia renders each recalled item with an epistemic mark read from its tags:
"confirmed by the person", "your own hypothesis, never confirmed", "a pattern
you formed".

### Already built

Migration `00037_add_tags.sql` adds `tags TEXT[]` (with GIN indexes) to
`episodes` and `facts`. `POST /api/v1/remember` and the MCP `remember` tool
accept `tags`; consolidation carries the union of source-episode tags onto
extracted facts (`internal/brain/consolidate.go`); `POST /api/v1/recall` and MCP
recall accept `tags` and return each item's `tags`. Epistemic marks stay
client-side (Gaia's `memoryKindOf`): CortexIQ stores and returns tags, it does
not interpret them.

### Remaining gap

The existing recall filter is **AND** (an item must carry every listed tag). Gaia
may need **OR** (e.g. `gaia:hypothesis` or `gaia:pattern`). Add an optional
`tags_any: []string` next to `tags`, same normalisation (`NormalizeTags`), same
GIN index (`&&` overlap operator). Both set means AND of the two conditions.
Only build this once a concrete Gaia call site needs it; until then the
read side works with `tags` alone.

## Phase 3 — Superseding version sync (the Cognition mirror)

### Why

Every Cognition state change (`proposed → testing → corroborated → confirmed |
rejected`) must become a new superseding version in memory with the previous
unit invalidated, and the mirror must be rebuildable from scratch
(`npm run reconcile:hindsight`). CortexIQ has the primitive (`valid_until`,
superseding facts) but no idempotent sync operation.

### Design

Two additions:

1. `external_key` — `text NULL` on episodes and facts, unique per namespace.
   Writers like `cognitionSync.js` key their units (`gaia:hypothesis:42`), so a
   mirror rebuild is an idempotent upsert, not a duplicate spray.
2. The atomic version write. **Recommended shape: extend `remember`, no new
   endpoint.** `POST /api/v1/remember` (and the MCP tool) gain `external_key`
   and `supersedes`; one write path keeps the noise gate, tags, async mode
   (phase 5) and identity stamping (phase 6) in one place instead of two.
   Pure invalidation (a rejection, no new content) is the only operation that
   does not fit `remember`; give it a small dedicated call,
   `POST /api/v1/invalidate {namespace, external_key}`, rather than a whole
   second write endpoint. A standalone `POST /api/v1/sync` is only worth it if
   Gaia's `cognitionSync.js` needs several units written atomically in one
   transaction — decide that when porting `gaia-logos`.

```json
{
  "namespace": "/projects/gaia-logos",
  "external_key": "gaia:hypothesis:42",
  "content": "Confirmed: hypotheses need human confirmation only",
  "tags": ["gaia:hypothesis", "gaia:confirmed"],
  "external_refs": ["foundation:9f3a…"],
  "supersedes": true
}
```

Semantics: upsert by `(namespace, external_key)`. When the key exists, the old
row gets `valid_until = now` plus a new `superseded_by` column; the new row
inherits `external_key`. `supersedes: false` is a plain upsert (state unchanged
→ no-op version). Rebuild = replay all writes; provenance (phase 4) makes each
derived record traceable, never hand-written.

## Phase 4 — Provenance: `external_refs` on episodes and facts

### Why

Gaia's Dual Recall renders the literal Foundation observations beneath each
recalled derived item — the way back from interpretation to raw reality. The
link is `sources: ['foundation:<id>']` on the item itself in Gaia's API.

### Naming

Gaia calls the field `sources`; CortexIQ already has a `fact_sources` table
(fact → episode, internal derivation) and `internal/brain/sources.go`. Reusing
the word would blur an internal link with an external pointer in code, traces
and the dashboard. CortexIQ stores it as **`external_refs`**; the API accepts
`sources` as an alias on input and returns `external_refs`, so the client port
stays a one-line mapping.

### Design

Migration: `external_refs jsonb NOT NULL DEFAULT '[]'` on `episodes` and
`facts`. `POST /api/v1/remember` and MCP `remember` accept `external_refs:
[]string`; recall returns them verbatim. CortexIQ stores and returns the
pointer; resolving `foundation:<id>` against the Foundation store is and stays
Gaia's job (one-way boundary, her own rule).

## Phase 5 — Async retain

### Why

`reflectOnTurn` writes with `async: true, timeout: null` so the deferred write
pipe can never block or break a delivered reply. CortexIQ's mental models
already have the shape (`202 Accepted`, background generation,
`internal/webui`); memories do not.

### Design

`POST /api/v1/remember` accepts `"async": true` → responds
`202 Accepted` + `{"job_id": "…"}` immediately after validation and
gate checks (the noise gate must stay synchronous — refusing "dank je wel"
must not cost a job). A single background worker drains the queue with the same
code path as the synchronous call. One worker is a deliberate start: it keeps
write order per namespace trivially correct (supersede chains, phase 3), at the
cost that one slow embedding call delays every queued job. Make the worker count
a setting only if queue latency is measured to matter; if more than one, shard
by namespace so per-key order holds. `GET /api/v1/jobs/<id>` reports
`queued | done | failed`. Durability: enqueue = insert into a `write_jobs`
table first, so a crash mid-queue leaves recoverable work, not lost memory.

## Phase 6 — Server-enforced agent identity

### Why

Gaia's memory writes are always *hers*: `agent_identity` is fixed server-side
and cannot be overridden by caller metadata. CortexIQ has no equivalent; the
`principal` on `fact_feedback` is a caller, not an identity guarantee.

### Open question — is this needed?

This is the weakest-justified phase. An immutable per-namespace identity only
guarantees *who writes*; Gaia writes to three banks under one identity, so it
does not distinguish "her own memory" from "confirmed by the person" — that is
the tag/bank axis (phases 1–2). Before building, name the concrete failure it
prevents (e.g. a caller overriding Gaia's identity through metadata). If the
only goal is rendering "her own memory", a bank-level convention plus a tag is
enough and this phase can be dropped. Treat it as optional and last.

### Design (if kept)

A namespace gains an immutable `agent_identity` set at creation
(`create_namespace` / `POST /api/v1/namespaces`), defaulting to the creating
principal. Every write stamps it server-side; an `agent_identity` field in a
remember request body is accepted and ignored (logged at debug). Recall
items carry `agent_identity` so Gaia's read side can render "her own memory"
from data rather than convention. Existing namespaces backfill to
`agent_identity = "cortexiq"` in the migration.

## What this deliberately does not do

- **No answer step for LongMemEval.** Public benchmark parity is the
  seventh gap, but it is an eval harness, not an API port — written up
  separately in `docs/design-longmemeval.md`.
- **No bank-level scores normalization.** The whole point of banks is that
  scores are incomparable; any future "smart" cross-bank merge must be opt-in
  and off the default path.
- **No interpretation of tags.** Epistemic marks are the caller's vocabulary;
  CortexIQ keeps them opaque.

## Migration and rollout order

One migration adds: `external_refs`, `external_key` (+ unique index), and
`agent_identity` (only if phase 6 is kept) on episodes and facts, plus the
`write_jobs` table (`tags`
already shipped in 00037). The
`banks` mode of recall needs no migration. Ship phases in pairs —
(1, 2) read side, (3, 4) write side, (5, 6) operational — each pair is
independently shippable and leaves recall behavior untouched unless the new
fields are used. Gaia's `hindsightClient.js` can then be ported bank by bank:
point `gaia-logos` at CortexIQ first (smallest blast radius, only needs
phases 3+4), keep `bojan` and `gaia` on Hindsight until phases 1+2+5+6 land.
