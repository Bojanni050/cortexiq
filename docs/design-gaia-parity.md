# Design: Gaia-Cloud Parity

> Status: phases 3 and 4 are implemented on branch `feat/external-key-refs`
> (written without a local Go toolchain; verified only by CI). The rest is
> proposal. Written 2026-11-02, revised 2026-11-03
> after a review and after reading Gaia-Cloud's merge code. Source of the
> dependency list: Gaia-Cloud (`Bojanni050/Gaia-Cloud`,
> `services/gaia-api/src/hindsightClient.js` and `docs/memory-pipe.md`),
> checked against CortexIQ's current code. The earlier list of gaps (reranking,
> reflect, graph arm, temporal recall, MMR) is closed — those exist in
> `internal/reranker`, `internal/brain/recall.go`, `internal/brain/mmr.go`.
> What remains is infrastructure, not intelligence.

## Vocabulary

CortexIQ has **namespaces**, not banks. "Bank" is Hindsight's word and appears
in this document only when quoting Gaia's code. Everything is expressed in
CortexIQ's own concepts:

| Gaia / Hindsight says | CortexIQ says |
|---|---|
| bank (`bojan`, `gaia`, `gaia-logos`) | namespace path; a path includes its descendants |
| memory unit / item | episode or fact (`type` in a recall result) |
| retain | `remember` |
| recall across banks | `recall` over several `namespaces` |
| `sources: ['foundation:<id>']` | `external_refs` (new). `fact_sources` already exists and means fact → episode |
| invalidate / supersede a unit | `valid_until` on the old row, a new row, `superseded_by` (new) |
| `agent_identity` | principal on a write (see `internal/brain/authz.go`) |

Which namespace each Gaia bank maps to is decided when porting; the paths used
in the examples below (`/people/bojan`, `/self/gaia`, `/projects/gaia-logos`)
are illustrative. Today Gaia's own notes live under `/projects/gaia`.

## Problem

Gaia-Cloud cannot swap `hindsightClient.js` for CortexIQ until six dependencies
are covered. Each one is named from Gaia's own code, with the current CortexIQ
state.

| # | Gaia dependency | Where in Gaia-Cloud | CortexIQ today |
|---|---|---|---|
| 1 | Recall across several stores with a per-store quota | `createCrossBankRecallClient`, `mergeBankSlices` | `recall` over several namespaces fuses all candidates into one RRF list |
| 2 | Arbitrary tags per item, readable in recall output | `memoryKindOf` reads item tags; `gaia:*` tags | **done** (migration 00037): tags on episodes and facts, accepted by `remember`, returned by `recall`, AND-filter; only an ANY-filter is open |
| 3 | Superseding version writes + rebuildable mirror | `cognitionSync.js`, `reconcile-hindsight-tags.js` | `valid_until`, superseding facts and `forget` exist; no keyed upsert |
| 4 | Provenance `sources: ['foundation:<id>']` | `evidenceRecall.js` Dual Recall | no per-item pointer to something outside CortexIQ |
| 5 | Async fire-and-forget retain | `reflectOnTurn` (`async: true, timeout: null`) | `POST /api/v1/remember` is synchronous |
| 6 | Server-enforced `agent_identity` | `memory.js` (identity always Gaia) | no equivalent |

Phases 1–6 are ordered by how hard Gaia is blocked without them: 1 and 2 block
the read side, 3 and 4 block the write side, 5 and 6 are operational.

## Is any of this useful without Gaia?

Gaia is the reason these are on the list, not the only reason to build them.
Judged on what CortexIQ gains with no Gaia in the picture:

| Phase | Useful without Gaia? | Why |
|---|---|---|
| 1 Quota recall over namespaces | **Maybe — measure first** | A `recall` on `/` or a parent returns one top-k list. A large namespace can fill every slot while small ones (`/self`, `/users/...`) never surface. That is a real risk, but unmeasured here: RRF is rank-based, so the "incomparable scales" argument from Hindsight carries over only weakly. It is a *diversity guarantee*, not a scale fix. Run the fixture below against live data before building. |
| 2 Tags (done) | Already shipped | Only the optional ANY-filter is open; build it when a caller asks. |
| 3 Keyed upsert + supersede (`external_key`) | **Yes, clearly** | Any integration that mirrors external state (a Hermes plugin, a file or ticket sync, re-running an import, the Hindsight → CortexIQ migration) gets idempotent re-runs instead of duplicates, and a queryable history of how a statement changed (`superseded_by`). |
| 4 Pointers to outside sources (`external_refs`) | **Yes, cheap** | Any memory can name where it came from: a commit (`commit:bf575f8`), a file, a URL, a ticket. Today that only survives as prose in `content`. |
| 5 Async `remember` | **Only if measured** | Useful for hooks and plugins that must never block a reply. If `remember` is fast today it is unnecessary; if embedding makes it slow, it pays off for every client. Measure latency first. |
| 6 Server-enforced identity | **Weak** | CortexIQ already has principals (`authz.go`, `fact_feedback`). A second identity concept is only justified by a concrete failure, e.g. several agents sharing one store and needing to tell their writes apart. Optional, last. |

Net: **3 and 4 are worth building on their own merits**; 1 and 5 depend on a
measurement; 2 is done; 6 is optional. That ordering also matches the rollout
below.

## Phase 1 — Quota recall over namespaces

### Why

Gaia recalls from three stores and found that a flat global sort lets one loud
store drown the others (measured on 2026-10-18, quoted in
`hindsightClient.js`). Its contract: each store speaks before any store
repeats, the rest follows in global score order. CortexIQ's `recall` over
several namespaces ranks everything in one list. The port needs a mode that
guarantees every listed namespace a place.

### Design

`recall` already takes `namespaces` (comma-separated in MCP, a list in the HTTP
body; each path matches itself and its descendants). Add one optional
parameter, `quota`. When it is set, each listed path becomes a **slice** and the
results are merged by the algorithm below. When it is absent, behaviour is
unchanged.

```json
{
  "query": "wat weet je over de outages in maart",
  "namespaces": ["/people/bojan", "/self/gaia", "/projects/gaia-logos"],
  "quota": 3,
  "limit": 9
}
```

`quota` is either an integer or `"auto"`. With `"auto"` it is derived:
`max(1, ceil(limit / answering_slices))`, where a slice only counts as
answering if it returned at least one item. Three slices and `limit: 9` give 3
each; two give 5 and 4. This is Gaia's `quotaForBankCount(count,
MEMORY_MAX_LINES)` with `limit` standing in for the line budget (default 9).

A listed path that contains another listed path (`/projects` and
`/projects/cortexiq`) would return the same item twice. Assign each item to the
**most specific** listed path and drop duplicates.

Merge algorithm, **verified against `mergeBankSlices` and
`createCrossBankRecallClient`** (`services/gaia-api/src/hindsightClient.js`,
2026-11-03). An earlier draft of this document had steps 2–4 wrong:

1. Run the existing recall pipeline **once per slice**, in parallel. RRF,
   rerank, temporal and MMR all stay *within* a slice. Each slice is re-sorted
   best-first by its final score.
2. **Quota rounds, round-robin:** for round 1..`quota`, take the next-best item
   of each slice in the order the namespaces were listed — best of each, then
   second best of each, and so on. This is *interleaved*, not "all of slice A,
   then all of slice B": with a hard line cap the order decides who is cut. A
   slice that runs out skips its turn. Items without a finite score get no
   reserved place and wait in the tail.
3. **Tail:** everything not claimed follows sorted by **global score**. This does
   compare scores across slices, but only for items beyond the quota, where the
   damage a loud namespace can do is bounded. A single namespace therefore
   behaves exactly like today's recall.
4. **Best-effort per slice:** a slice that errors or times out contributes an
   empty list instead of failing the whole recall, and does not count toward the
   derived quota. The response says which slices were dropped.
5. Truncate to `limit` after merging (Gaia truncates later, at the prompt line
   budget).

CortexIQ's `Score` is an RRF value normalised to 0..1, which is comparable
across slices in rank terms but not in absolute relevance. Replicate the
global-score tail for parity and keep it behind one internal function so a
rank-interleaved tail (slice A's #4, slice B's #4, …) can replace it later if
the tail proves noisy. That alternative is *ours*, not Gaia's, and not the
default.

Every result item gains a `namespace` field (today only `namespace_id`), so the
caller can render the epistemic mark per namespace — Gaia renders items from the
`gaia-logos` store as "confirmed by the person", from `gaia` as her own memory.
The recall trace gains a `slices` section with per-slice duration, returned
count and error, mirroring the existing arm traces.

Validation: extend `internal/brain/recall_test.go` with a fixture where
namespace A would win a global sort 8-of-9 and assert (a) the round-robin order
— A1, B1, C1, A2, B2, C2, A3, B3, C3 — (b) the tail in global score order, (c)
one failing slice leaves the other two intact with a recomputed quota, (d) a
listed parent and child do not duplicate items. Port the cases of
`test/hindsightCrossBankRecall.test.js` rather than inventing new ones.

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

The existing recall filter is **AND** (an item must carry every listed tag). A
caller may need **OR** (e.g. `gaia:hypothesis` or `gaia:pattern`). Add an
optional `tags_any: []string` next to `tags`, same normalisation
(`NormalizeTags`), same GIN index (`&&` overlap operator). Both set means AND of
the two conditions. Build it only once a concrete call site needs it; until
then the read side works with `tags` alone.

**A bigger gap, found while implementing phase 3:** `NormalizeTags` only accepts
`^[a-z0-9][a-z0-9_-]*$` (`internal/brain/tags.go`), so Gaia's `gaia:hypothesis`
and `gaia:confirmed` are **rejected today** because of the colon. Either allow
`:` inside a tag (a change to the tag vocabulary for everyone) or let the Gaia
port map `gaia:x` to `gaia-x`. This must be decided before `gaia-logos` is
pointed at CortexIQ; it blocks the rollout more than the ANY-filter does.

## Phase 3 — Keyed upsert with superseding versions

### Why

Every Cognition state change in Gaia (`proposed → testing → corroborated →
confirmed | rejected`) must become a new version in memory with the previous one
invalidated, and the mirror must be rebuildable from scratch
(`npm run reconcile:hindsight`). CortexIQ has the primitives (`valid_until`,
superseding facts) but no idempotent, keyed write. The same shape serves any
integration that mirrors external state, so this phase stands on its own.

### Design

1. `external_key` — `text NULL` on episodes and facts, unique per namespace
   among rows that are still valid. Writers key their units
   (`gaia:hypothesis:42`), so a rebuild is an idempotent upsert, not a
   duplicate spray.
2. **The version write extends `remember`; there is no new write endpoint.**
   `POST /api/v1/remember` (and the MCP tool) gain `external_key` and
   `supersedes`. One write path keeps the noise gate, tags, async mode (phase
   5) and identity (phase 6) in one place. Pure invalidation (a rejection, no
   new content) is the one operation that does not fit `remember`; give it a
   small dedicated call, `POST /api/v1/invalidate {namespace, external_key}`. A
   standalone `POST /api/v1/sync` is only worth it if Gaia's `cognitionSync.js`
   needs several units written atomically — decide that when porting
   `gaia-logos`.

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

Semantics: upsert by `(namespace, external_key)`. When the key exists and
`supersedes` is true, the old row gets `valid_until = now` and a new
`superseded_by` column pointing at the new row, which inherits `external_key`.
With `supersedes: false` and unchanged content the call is a no-op. Rebuild =
replay all writes; `external_refs` (phase 4) makes each derived record
traceable, never hand-written.

**Implementation notes (2026-11-03).** Episodes are immutable in CortexIQ and
carry no `valid_until`; `remember` writes episodes, and facts only come from
consolidation. So the design above was adapted:

- The key lives on the **episode**. Superseding retires the old episode with
  `deleted_at` (the same soft delete forget and retract use) and sets
  `superseded_by`; the unique index
  `episodes_external_key_live (namespace_id, external_key)` covers live rows
  only, so the retired row frees the key for its successor.
- Facts derived from the old episode are closed (`valid_until` and
  `known_until` set) **only if** the retired episode was their last live source.
  A fact that also rests on another live episode stays open.
- Same key, same content: a no-op that returns the existing id (HTTP 200,
  `unchanged: true`), decided before the embedding call so a rebuild is cheap.
- Same key, different content, no `supersedes`: refused (HTTP 409,
  `ErrExternalKeyExists`) rather than silently rewriting history.
  `supersedes` without a key is a 400.
- `external_refs` is `TEXT[]` like `tags`, not jsonb.

**Invariant, tested:** invalidating the old row and inserting the new one
happen in **one transaction**. A failed write must leave the old row valid; it
must never leave a gap in which the key resolves to nothing. (The same rule
EverOS applies to its own merges: retire the old only once the new is complete —
see `docs/note-everos-lessons.md`.)

## Phase 4 — Pointers to outside sources: `external_refs`

### Why

Gaia's Dual Recall renders the literal Foundation observations beneath each
recalled derived item — the way back from interpretation to raw reality. In
Gaia's API the link is `sources: ['foundation:<id>']`. For any CortexIQ user the
same field records where a memory came from (`commit:bf575f8`, a file path, a
URL, a ticket id).

### Naming

CortexIQ already has a `fact_sources` table (fact → episode, internal
derivation) and `internal/brain/sources.go`. Reusing the word `sources` would
blur an internal link with an external pointer in code, traces and the
dashboard. CortexIQ stores the field as **`external_refs`**; the API accepts
`sources` as an alias on input and returns `external_refs`, so the Gaia port
stays a one-line mapping.

### Design

Migration `00042_add_external_key_refs.sql`: `external_refs TEXT[] NOT NULL
DEFAULT '{}'` on `episodes` and `facts`. `POST /api/v1/remember` and the MCP `remember` accept `external_refs:
[]string`; recall returns them verbatim. CortexIQ stores and returns the
pointer; resolving it (`foundation:<id>` against Gaia's Foundation store) is and
stays the caller's job — a one-way boundary.

Facts consolidated from several episodes carry the sorted union of their
episodes' `external_refs`, the way they already carry tags (`sourceRefs` in
`internal/brain/external.go`, used by `storeFact` and `reinforceFact`). Recall
results return `external_key` (episodes only) and `external_refs`, filled in one
query per table after the final cut (`attachProvenance`).

## Phase 5 — Async retain

### Why

`reflectOnTurn` writes with `async: true, timeout: null` so the deferred write
pipe can never block or break a delivered reply. CortexIQ's mental models
already have the shape (`202 Accepted`, background generation,
`internal/webui`); memories do not. Build it only after measuring how long a
synchronous `remember` takes: if it is fast, this phase is unnecessary.

### Design

`POST /api/v1/remember` accepts `"async": true` → responds `202 Accepted` +
`{"job_id": "…"}` immediately after validation and gate checks (the noise gate
must stay synchronous — refusing "dank je wel" must not cost a job). A single
background worker drains the queue with the same code path as the synchronous
call. One worker is a deliberate start: it keeps write order per namespace
trivially correct (supersede chains, phase 3), at the cost that one slow
embedding call delays every queued job. Make the worker count a setting only if
queue latency is measured to matter; if more than one, shard by namespace so
per-key order holds. `GET /api/v1/jobs/<id>` reports `queued | done | failed`.
Durability: enqueue = insert into a `write_jobs` table first, so a crash
mid-queue leaves recoverable work, not lost memory.

## Phase 6 — Server-enforced agent identity (optional)

### Why

Gaia's memory writes are always *hers*: `agent_identity` is fixed server-side
and cannot be overridden by caller metadata.

### Open question — is this needed?

This is the weakest-justified phase. CortexIQ already has principals
(`internal/brain/authz.go`; `fact_feedback` records a principal per vote). An
immutable per-namespace identity only guarantees *who writes*; it does not
distinguish "her own memory" from "confirmed by the person" — that is the
namespace-and-tag axis (phases 1–2). Before building, name the concrete failure
it prevents (e.g. a caller overriding Gaia's identity through request
metadata). If the only goal is rendering "her own memory", a namespace plus a
tag is enough and this phase can be dropped. Treat it as optional and last, and
prefer extending the existing principal over adding a second identity concept.

### Design (if kept)

A namespace gains an immutable `agent_identity` set at creation
(`create_namespace` / `POST /api/v1/namespaces`), defaulting to the creating
principal. Every write stamps it server-side; an `agent_identity` field in a
request body is accepted and ignored (logged at debug). Recall items carry
`agent_identity`. Existing namespaces backfill to `agent_identity = "cortexiq"`
in the migration.

## What this deliberately does not do

- **No answer step for LongMemEval.** Public benchmark parity is an eval
  harness, not an API port — see `docs/design-longmemeval.md`.
- **No score normalisation across slices.** Any future "smart" merge across
  namespaces must be opt-in and off the default path.
- **No interpretation of tags or `external_refs`.** Epistemic marks and pointer
  formats are the caller's vocabulary; CortexIQ keeps them opaque.

## Migration and rollout order

One migration adds `external_key` (+ unique index), `external_refs` and
`superseded_by` on episodes and facts, `agent_identity` only if phase 6 is
kept, and the `write_jobs` table only if phase 5 is built (`tags` already
shipped in 00037). Quota recall (phase 1) needs no migration.

Build order, by independent value and risk:

1. **Phases 3 + 4** — the write side. Useful without Gaia, testable without
   Gaia, and all that `gaia-logos` needs.
2. **Phase 1** — after the fixture has been run on live data and shown a real
   problem.
3. **Phase 5** — after `remember` latency has been measured.
4. **Phase 6** — only against a named failure.

Gaia's `hindsightClient.js` is then ported one store at a time: point
`gaia-logos` at CortexIQ first (smallest blast radius, only needs phases 3+4),
keep `bojan` and `gaia` on Hindsight until phases 1, 5 (and 6, if kept) land.
