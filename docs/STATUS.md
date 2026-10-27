# Status

Living page: what this build is, what is switched on, and what is still open.
For the dated snapshot it replaces, see [handoff-2026-10-18](handoff-2026-10-18.md) —
that file is history and is deliberately not updated.

Last updated: 2026-10-27, after the `v0.1.9` deploy.

## Build

| | |
|---|---|
| Latest release | `v0.1.9`, 2026-10-27 |
| Release commit | `ec43c43` |
| How to check | `cortexiq version` (works without a database) |

A binary built from a working tree reports `cortexiq dev (built from source)`.
Release and Docker builds inject the tag, commit and build date; the values live
in `internal/version`.

## What runs where

- Production: the `cortexiq-cortexiq-1` and `cortexiq-cortexiq-ui-1` containers on
  `strato-vps`, in `/opt/cortexiq`, running `v0.1.9` (`cortexiq version` inside the
  container is the source of truth; the binary lives at `/cortexiq` and the image
  has no shell, so use `docker compose exec -T cortexiq /cortexiq version`).
  Both containers run image `sha256:80f56b92e9a6…`. Migrations `00038`
  (retractions and the purge log) and `00039` (providers and model bindings)
  applied on the 2026-10-26 and 2026-10-27 restarts, and `00040` (hygiene) on
  the 2026-10-27 restart. `00041` applied on the `v0.1.9` restart: it drops
  twelve `hygiene_*` columns and the `hygiene_audit_log` table, so the schema is
  back to what it was before `00040` — verified after the deploy (0 matching
  columns, table gone, data intact). All of them are idempotent.
- The MCP server hands its prime directive to a client at connect time, in the
  `initialize` result's `instructions` field (6445 characters on `v0.1.4`). A
  client reads that once per connection, so it has to reconnect after an upgrade
  to pick up a change. Hermes does not depend on it: its `cortexiq` memory
  provider recalls before a turn and captures after it, and writes only when a
  session actually runs — which is why the silence from 24 to 27 October was
  usage, not a broken integration.
- The dashboard (`/ui`) is three columns: navigation, the content, and a context
  column that lists the facts an episode produced or the episodes behind a fact.
  It collapses from the topbar or its own × and becomes a drawer below 1360px;
  the choice is kept in that browser, not on the server. Since `v0.1.8` it
  wears the logo's palette (teal-green accent, olive second colour) and the
  brain mark is served from `/ui/brand/mark-96.png`.
- Deploying: production pulls the image CI built —
  `ghcr.io/bojanni050/cortexiq:${CORTEXIQ_IMAGE_TAG}`. To deploy, bump
  `CORTEXIQ_IMAGE_TAG` in `/opt/cortexiq/.env` to the new release and run
  `docker compose -f docker-compose.prod.yml up -d`; nothing is compiled on the
  server, so the running binary is byte-for-byte the one the release workflow
  produced (compare `.Image` with the tag's digest). Rolling back is setting the
  previous tag. Migrations run on startup.
- `build: .` is no longer in the prod compose. The old locally built
  `cortexiq:local` image and `0.1.0`/`0.1.1` stay on the server as rollback
  artifacts, about 90 MB together; nothing pulls them. The local `latest` tag was
  untagged, so nothing on the server suggests a moving tag.
- Registry tags: exactly one tag per release (`0.1.2`, `0.1.3`, …) and nothing
  else. Checked at the registry after `v0.1.9`: `0.1.2` through `0.1.9`, eight
  tags for eight releases, no `latest` and no floating `0.1`. Past releases are
  kept rather than deleted, so a rollback can still pull the tag it names;
  production pulls `0.1.9` today. With a moving tag, "which version runs here"
  becomes a guess. `CORTEXIQ_IMAGE_TAG` is a required variable in the compose
  file, so a missing value fails the deploy instead of silently pulling
  something old.
- Getting there took one correction. `v0.1.1` was published with a `latest` tag
  even after the explicit `type=raw,value=latest` was removed, because
  `docker/metadata-action` defaults to `flavor: latest=auto` and adds `latest`
  itself. `v0.1.2` sets `flavor: latest=false`, and the result was checked at
  the registry (`/v2/bojanni050/cortexiq/tags/list` with an anonymous pull
  token) rather than in the GitHub UI. The old versions were then deleted; a tag
  cannot be removed on its own, only the version it belongs to.
- Untagged manifests still in the GHCR versions list are buildkit attestations,
  not images; they are harmless.
- This page may be one documentation-only commit ahead of the deployed binary;
  compare with `cortexiq version` rather than assuming.
- Storage: Postgres + pgvector. At the `v0.1.9` deploy: 1,150 live facts,
  325 episodes, 24 namespaces, 70 MB.

## Switched off, on purpose

| Setting | Production | Why |
|---|---|---|
| `CORTEXIQ_GRAPH_RECALL` | `false` | Off by default since the 37-query measurement: on, it costs topical questions more than it gains on relational ones. Any call can still ask for it with `"graph": true`; the capability is intact, only the default changed. See below. |
| `CORTEXIQ_RERANKER_MODEL` | `jina-reranker-v3.5` | On. It re-scores the top ~100 fused candidates and costs roughly 300 ms of a ~525 ms recall, so it owns the top of the list. |
| `CORTEXIQ_RECALL_MIN_SIMILARITY` | `0.40` | Drops weak vector-only hits. Model dependent. |
| `CORTEXIQ_CONSOLIDATION_CANDIDATE_CAP` | `0` | Measured on 272 real episodes (2026-10-26): the lexical arm would add a member to 271 of them, but the near-threshold pairs are mostly state changes (v0.1.2 → v0.1.3, bug found → bug fixed, volume not shared → shared), not restatements. Left at 0. |
| `CORTEXIQ_CONSOLIDATION_DEDUP_THRESHOLD` | `0.98` | Auto-merge only near-identical facts. On 932 real facts, every pair between 0.85 and 0.98 differed in a number, an entity or a state. |
| `CORTEXIQ_CONSOLIDATION_DEDUP_REVIEW_THRESHOLD` | `0.85` | Below this, an LLM verdict was always "different"; above it the pair is worth asking. Narrowed from 0.75, which sent a third of all facts through the model. |
| `CORTEXIQ_TRIAGE_MODEL` | unset | Optional cheap classifier that runs before extraction and can refuse a cluster of acknowledgements. Unset sends everything to the reasoner. It fails open (unusable answer = durable), so enabling it risks cost, not memories. |
| `CORTEXIQ_WRITE_FILTER` | `off` | Since `v0.1.8`, opt-in: `warn` logs what it would refuse, `strict` refuses secrets and noise. Left off until the false-positive rate on real writes is known. The hygiene cleaner that shipped beside it was removed in `v0.1.9`: it audited noise the store's own decay already handles, and its archive flag was read by nothing. |
| `CORTEXIQ_ROUTE_RECALL` | `false` | Only widens recall; never measured in production. |
| `CORTEXIQ_RECALL_MMR_LAMBDA` | `0` | Available per call; off by default so ranking stays plain fusion. |

These are the values read from `/opt/cortexiq/.env` on 2026-10-26, not defaults
from the code. Two of them used to be accidents rather than decisions: the
candidate cap looked like ordinary configuration nobody had measured, and the
dedup thresholds were in the file twice, so the effective values came from
whichever line happened to be last (see below).

### Consolidation: what merges, and what does not (272 episodes, 932 facts)

Both consolidation decisions were measured on production data, read-only, rather
than argued from the synthetic corpus:

- **Episode clusters.** At the 0.85 similarity threshold only **9 pairs** of the
  272 active episodes reach each other at all, so clusters are effectively single
  episodes today. Turning on the lexical arm (candidate cap > 0) would add a
  member to **271 of 272** episodes, but the pairs just under the threshold are
  mostly state changes — a plugin at `v0.1.2` that broke versus `v0.1.3` that
  fixed it, a bug found versus the same bug fixed, "no shared volume" versus
  "shared volume is live". Merging those would erase the sequence. The synthetic
  result that promised 20/20 on rewordings did not transfer, because its "same"
  pairs were restatements. Cap stays `0`.
- **Fact dedup.** Of 91,290 same-namespace fact pairs, **0** reach 0.98, 33 reach
  0.85, 99 reach 0.80 and 267 reach 0.75. Read by hand, the 33 near-identical
  pairs are mostly not duplicates either: hit@1 0.75 next to hit@3 0.97, port
  8445 (UI) next to port 8444 (MCP), Hindsight cold-load next to CortexIQ
  cold-load. Cosine on this corpus says "same subject", not "same fact".

The `.env` had `CORTEXIQ_CONSOLIDATION_DEDUP_THRESHOLD` twice — `0.85` and then
`0.98` — and the container took the last one, while the review floor sat at
`0.75`. Net effect: nothing ever auto-merged (no pair reaches 0.98 anyway) and
336 facts, 36% of the corpus, were each sent to the model for a verdict whose
answer was almost always "different". The duplicate line is gone; the values are
now `0.98` and `0.85`, so a review happens only where it can change something.

Extraction itself had no such gate: every episode that survived the fragment
filter went to the reasoner, including sessions that are mostly "ok" and
"thanks". `CORTEXIQ_TRIAGE_MODEL` adds Hindsight's triage idea — a cheaper model
classifies the cluster first and may refuse it. It is off here, because there is
no cheap model configured and because the honest saving is unmeasured; a triage
answer that cannot be parsed counts as durable, so the worst case is a wasted
call rather than a lost memory.

### What the graph arm actually does (37 real queries, 2026-10-26)

| Set | n | graph off (MRR) | graph on (MRR) | better / worse |
|---|---|---|---|---|
| Topic questions | 19 | 0.496 | 0.423 | 2 / 4 |
| Entity and relation questions | 18 | 0.257 | 0.425 | 9 / 2 |
| All | 37 | 0.380 | 0.424 | 11 / 6 |

Measured twice, before and after the neighbour floor (v0.1.3). The floor did its
job — results reachable only through the graph arm fell from 9 to 1 — and no
rank moved, because the neighbours it removed were already ranked below the fold
at the limits people use. The topical loss has a different cause, and it is not
junk: a fact that shares an entity with the question gets both a vector rank and
a graph rank, so equal-weight fusion counts it twice and a plausible neighbour
outranks a better answer.

So the arm is not wrong, its default was. `CORTEXIQ_GRAPH_RECALL` is `false` in
production since 2026-10-26, and callers that need the arm pass `"graph": true`
per request — which works regardless of the global default, verified against the
same instance: a plain recall now matches graph-off exactly (no graph stage in
the trace at all, MRR 0.496 on topic questions), while the 18 relational
questions still reach MRR 0.425 when the caller asks for the arm.

The floor stays. Not because it moved a rank in this measurement, but because
production runs `CORTEXIQ_MAX_RESULT_SIZE=10000`: at a large limit those
neighbours do surface, and there the floor is the difference between a hub list
and a pile.

Gating the arm on question shape — only run it when the question names two or
more entities — remains open. Entity counts did not separate the two sets
cleanly (on the topic set, gating at three or more entities scored 0.538 against
0.554 for always-off), and thresholds tuned on 37 hand-written labels prove
little.

Fixed alongside it: a recall that asked for facts *and* episodes returned facts
only. Episodes were handed whatever slots the facts left over, and facts fill the
limit. Each type now searches with its own budget and the merged list is sorted
by score, so the better answer wins regardless of type.


## Tests and CI

| | |
|---|---|
| Test functions | 307 across 57 files |
| Unit tests | run everywhere, no database needed |
| Integration tests | `internal/brain`, need `CORTEXIQ_TEST_DSN`; CI runs them against `pgvector/pgvector:pg16` and fails if any was skipped |
| Race detector | CI job `race`, `go test -race ./...` — green since its first run on 2026-10-26 |
| Coverage | 43.5% overall, gated by `.github/coverage-baseline.txt` (ratchet, cannot drop) |

Coverage is measured with the database attached. Without it `internal/brain`
reports 26% instead of 67%, so a coverage number from a bare `go test ./...` is
not comparable.

Thin spots, in order of how much it hurts: `internal/bootstrap` (12%),
`internal/db` (11%), `internal/webui` (15%), `internal/reasoner` (21%). Those are
the wiring layers — a bug there breaks startup or the dashboard, not recall.

## Removed memory: three verbs

- `forget` — soft delete, no judgement. `episodes.deleted_at` is set.
- `retract` — soft delete plus a row in `retractions` with the claim and the
  reason. For "that was wrong".
- `purge` — hard delete. Also removes facts sourced only by the purged episode,
  the matching `embedding_cache` text, retractions of the target, and writes a
  `purge_log` row without content. `purge namespace` refuses to run with child
  namespaces and needs the slug twice.

Drop the dashboard in and all three are reachable there since `v0.1.1`: `retract`
sits next to `purge` on every memory row (with the same "a reason is required"
rule as the API), and the Lifecycle tab shows retractions and purges in one
timeline. Retraction records follow the namespace selector; purge records are
global, because a purge can remove the namespace itself.

There is no undo for a purge, and there is no soft-delete sweep: tombstones stay
in place.

- A model chosen in the dashboard is stored in `llm_bindings`, and from `v0.1.5`
  the database wins over the environment **at startup**, not only in the process
  that accepted the change. Before that, a binding looked enabled and then did
  nothing in the next consolidation run, because that runs in its own process.
- Triage was tried on `/test` after `v0.1.5`: two fresh episodes, one durable and
  one chatter, gave `episodes_read: 2`, `episodes_skipped: 1` — the chatter was
  refused before extraction. `llm_calls` went from 5 to 6, because the triage call
  is also a call; the saving is in tokens, not in calls. One skipped episode out of
  two proves the wire is live, not the saving; that needs weeks of real sessions.

## Models and providers

The four models consolidation uses (reasoner, triage, embedding, reranker) are
visible in **Settings → Models**, with the environment value next to whatever was
chosen in the dashboard, so an override never hides the deployment's own setting.

- Providers live in `llm_providers` (migration 39), alongside the model chosen per
  purpose in `llm_bindings`. A purpose without a row keeps the environment value,
  which is why an empty table changes nothing.
- API keys are **write-only**: they are stored and never returned by the API. The
  dashboard sees `…` and the last four characters, enough to tell which key is
  loaded. An empty key on an update means "keep the stored one", so editing a
  name cannot silently erase a credential.
- **Fetch models** asks the provider itself, so pricing, context length and
  capabilities are theirs and not our guesses: DeepInfra's public catalogue
  (`/models/list`) returned 397 models, 230 of them chat-completions with prices.
  Mistral's `/v1/models` needs the key; a rejected key is reported as such and the
  key is never echoed.
- Reasoner and triage take effect immediately. The **embedding model is
  read-only**: stored vectors carry its dimensions, so switching it is a re-embed
  job, and the API refuses with `409` rather than letting recall degrade quietly.
  The reranker is changeable but needs a restart.

## Open decisions

Closed on 2026-10-26 by measurement: the candidate cap stays `0`, and fact dedup
runs at `0.98` with reviews from `0.85` (see the consolidation section above).
What is left:

1. Graph recall: decided — off by default, opt-in per call, on the evidence
   above. Still open is whether to gate it on question shape, which needs labels
   from questions you actually could not answer, not more invented ones.
2. RRF arm weights: the sweep says the graph arm at any weight above 0 is worse
   before reranking, and with the arm off by default the point is moot. The
   reranker decides the top anyway, so tuning weights would be work with no
   measurable effect.
3. Cross-encoder reranker: a new external dependency, not just code.

## Known rough edges

- No semantic versioning history before `v0.1.0`, so nothing can depend on
  "the API as of v0.x".
- A row without an embedding stops the whole namespace run: scanning it into a
  non-pointer vector fails with `unsupported data type: <nil>`, logged per query
  rather than per row. Production writes always embed, so this reaches
  hand-seeded or imported rows, not normal traffic.
- Windows: `go test ./...` can fail with `unlinkat ... test.exe` when a test
  binary is still running. Retry; it is not a test failure.
- The integration suite wants a clean database. `TestGlobalMentalModelReadsAllNamespaces`
  builds a global model from a cross-namespace recall, so facts left behind by
  earlier runs can push the expected one out of the top N and fail the test.
  Use a fresh database for a full run, not the one used for manual poking.
- `docs/handoff-2026-10-18.md` still lists tags, Reflect and mental models as
  "not built". They shipped on 2026-10-23; the handoff stays as the record of
  that day.
