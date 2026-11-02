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
state and the proposed port.

| # | Gaia dependency | Where in Gaia-Cloud | CortexIQ today |
|---|---|---|---|
| 1 | Banks + cross-bank recall with per-bank quota | `createCrossBankRecallClient`, `CROSS_BANK_QUOTA = 3` | namespaces fuse into one RRF list on one scale |
| 2 | Arbitrary tags per item, readable in recall output | `memoryKindOf` reads item tags; `gaia:*` tags | tags exist only on mental models |
| 3 | Superseding version writes + rebuildable mirror | `cognitionSync.js`, `reconcile-hindsight-tags.js` | superseding facts and `forget` exist; no sync operation |
| 4 | Provenance `sources: ['foundation:<id>']` | `evidenceRecall.js` Dual Recall | no per-item sources field |
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

Merge algorithm (the exact contract Gaia relies on, ported verbatim):

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

## Phase 2 — Tags on episodes and facts

### Why

Gaia renders each recalled item with an epistemic mark read from its tags:
"confirmed by the person", "your own hypothesis, never confirmed", "a pattern
you formed". In CortexIQ only mental models carry tags
(`internal/brain/mental_model.go`), and recall cannot filter or return them.

### Design

Migration: `tags text NOT NULL DEFAULT '{}'` on `episodes` and `facts`
(`[]string`, same storage as mental models).

- `POST /api/v1/remember` and the MCP `remember` tool accept `tags: []string`.
  Consolidation carries the union (capped) of source-episode tags onto extracted
  facts so `gaia:hypothesis` survives extraction.
- `POST /api/v1/recall` accepts `tags: []string` (ANY-match) and returns each
  item's `tags`. MCP recall mirrors both.
- Epistemic marks themselves stay client-side (Gaia's `memoryKindOf`): CortexIQ
  stores and returns tags, it does not interpret them.

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
2. `POST /api/v1/sync` — the atomic version write:

```json
{
  "namespace": "/projects/gaia-logos",
  "external_key": "gaia:hypothesis:42",
  "type": "fact",
  "content": "Confirmed: hypotheses need human confirmation only",
  "tags": ["gaia:hypothesis", "gaia:confirmed"],
  "sources": ["foundation:9f3a…"],
  "supersedes": true
}
```

Semantics: upsert by `(namespace, external_key)`. When the key exists, the old
row gets `valid_until = now` plus a new `superseded_by` column; the new row
inherits `external_key`. `supersedes: false` is a plain upsert (state unchanged
→ no-op version). A rejection is `supersedes: true` with no content — a pure
invalidation. Rebuild = replay all syncs; provenance (phase 4) makes each
derived record traceable, never hand-written.

## Phase 4 — Provenance: `sources` on episodes and facts

### Why

Gaia's Dual Recall renders the literal Foundation observations beneath each
recalled derived item — the way back from interpretation to raw reality. The
link is `sources: ['foundation:<id>']` on the item itself.

### Design

Migration: `sources jsonb NOT NULL DEFAULT '[]'` on `episodes` and `facts`.
`POST /api/v1/remember`, `/v1/sync` and MCP `remember` accept `sources:
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
code path as the synchronous call. `GET /api/v1/jobs/<id>` reports
`queued | done | failed`. Durability: enqueue = insert into a `write_jobs`
table first, so a crash mid-queue leaves recoverable work, not lost memory.

## Phase 6 — Server-enforced agent identity

### Why

Gaia's memory writes are always *hers*: `agent_identity` is fixed server-side
and cannot be overridden by caller metadata. CortexIQ has no equivalent; the
`principal` on `fact_feedback` is a caller, not an identity guarantee.

### Design

A namespace gains an immutable `agent_identity` set at creation
(`create_namespace` / `POST /api/v1/namespaces`), defaulting to the creating
principal. Every write stamps it server-side; an `agent_identity` field in a
remember/sync request body is accepted and ignored (logged at debug). Recall
items carry `agent_identity` so Gaia's read side can render "her own memory"
from data rather than convention. Existing namespaces backfill to
`agent_identity = "cortexiq"` in the migration.

## What this deliberately does not do

- **No answer step for LongMemEval.** Public benchmark parity is the
  seventh gap, but it is an eval harness, not an API port — separate proposal.
- **No bank-level scores normalization.** The whole point of banks is that
  scores are incomparable; any future "smart" cross-bank merge must be opt-in
  and off the default path.
- **No interpretation of tags.** Epistemic marks are the caller's vocabulary;
  CortexIQ keeps them opaque.

## Migration and rollout order

One migration adds: `tags`, `sources`, `external_key` (+ unique index), and
`agent_identity` on episodes and facts, plus the `write_jobs` table. The
`banks` mode of recall needs no migration. Ship phases in pairs —
(1, 2) read side, (3, 4) write side, (5, 6) operational — each pair is
independently shippable and leaves recall behavior untouched unless the new
fields are used. Gaia's `hindsightClient.js` can then be ported bank by bank:
point `gaia-logos` at CortexIQ first (smallest blast radius, only needs
phases 3+4), keep `bojan` and `gaia` on Hindsight until phases 1+2+5+6 land.
