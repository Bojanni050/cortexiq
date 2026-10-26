# Status

Living page: what this build is, what is switched on, and what is still open.
For the dated snapshot it replaces, see [handoff-2026-10-18](handoff-2026-10-18.md) —
that file is history and is deliberately not updated.

Last updated: 2026-10-26, after the `v0.1.2` deploy.

## Build

| | |
|---|---|
| Latest release | `v0.1.2`, 2026-10-26 |
| Release commit | `fa2ebb3` |
| How to check | `cortexiq version` (works without a database) |

A binary built from a working tree reports `cortexiq dev (built from source)`.
Release and Docker builds inject the tag, commit and build date; the values live
in `internal/version`.

## What runs where

- Production: the `cortexiq-cortexiq-1` and `cortexiq-cortexiq-ui-1` containers on
  `strato-vps`, in `/opt/cortexiq`, running `v0.1.2` (`cortexiq version` inside the
  container is the source of truth; the binary lives at `/cortexiq` and the image
  has no shell, so use `docker compose exec -T cortexiq /cortexiq version`).
  Both containers run image `sha256:eac152ad97ee…`. Migration `00038` applied on
  the 2026-10-26 restart and needs nothing on later upgrades.
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
  else. GHCR holds a single version today — `0.1.2`, the one production pulls.
  No `latest`, no floating `0.1`: with a moving tag, "which version runs here"
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
- Storage: Postgres + pgvector. At the last count: 9,526 memories, 65.9 MB,
  1,019 facts, 281 episodes, 24 namespaces.

## Switched off, on purpose

| Setting | Production | Why |
|---|---|---|
| `CORTEXIQ_GRAPH_RECALL` | `true` | On, and it was on without this page saying so. Measured on 37 real queries (2026-10-26): it nearly doubles MRR on relation questions and lowers it on topical ones. See below. |
| `CORTEXIQ_RERANKER_MODEL` | `jina-reranker-v3.5` | On. It re-scores the top ~100 fused candidates and costs roughly 300 ms of a ~525 ms recall, so it owns the top of the list. |
| `CORTEXIQ_RECALL_MIN_SIMILARITY` | `0.40` | Drops weak vector-only hits. Model dependent. |
| `CORTEXIQ_CONSOLIDATION_CANDIDATE_CAP` | `0` | Threshold-only clustering is the known-good behaviour. The lexical arm reached 20/20 on rewordings in eval, but the extra members widen provenance. |
| `CORTEXIQ_ROUTE_RECALL` | `false` | Only widens recall; never measured in production. |
| `CORTEXIQ_RECALL_MMR_LAMBDA` | `0` | Available per call; off by default so ranking stays plain fusion. |

These are the values read from `/opt/cortexiq/.env` on 2026-10-26, not defaults
from the code. The candidate cap is the one most likely to be switched on by
accident because it looks like ordinary configuration; it needs a measurement
on real memories first.

### What the graph arm actually does (37 real queries, 2026-10-26)

| Set | n | graph off (MRR) | graph on (MRR) | better / worse |
|---|---|---|---|---|
| Topic questions | 19 | 0.496 | 0.423 | 2 / 4 |
| Entity and relation questions | 18 | 0.257 | 0.425 | 9 / 2 |
| All | 37 | 0.380 | 0.424 | 11 / 6 |

So the arm is not wrong, it is undiscriminating: where a question names two
entities it is worth a lot, and where the question is topical it drags in
neighbours that happen to share an entity. The mechanism is in `relevant()`:
a vector-only result must clear `min_similarity`, but a graph-only result skips
that check entirely, so unrelated neighbours enter the list and push real
answers down. Two candidate fixes, neither taken: gate the arm on the question
naming two or more entities, or give graph-only results a similarity floor. The
37 labels are hand-written and the thresholds were compared after the fact, so
treat the second fix as a hypothesis to measure, not a conclusion.

Fixed alongside it: a recall that asked for facts *and* episodes returned facts
only. Episodes were handed whatever slots the facts left over, and facts fill the
limit. Each type now searches with its own budget and the merged list is sorted
by score, so the better answer wins regardless of type.


## Tests and CI

| | |
|---|---|
| Test functions | 305 across 57 files |
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

## Open decisions

1. `CORTEXIQ_CONSOLIDATION_CANDIDATE_CAP`: turn on after measuring on real
   memories, or leave at 0.
2. Graph recall: it is on in production and measured. Decide between gating it
   on relation-shaped questions and giving graph-only results a similarity
   floor — or accept the noise and keep it as is.
3. RRF arm weights: the sweep over the 37 queries says the graph arm at any
   weight above 0 is worse before reranking, but the reranker decides the top
   anyway, so tuning weights now would be work without a measurable effect.
4. Cross-encoder reranker: a new external dependency, not just code.

## Known rough edges

- No semantic versioning history before `v0.1.0`, so nothing can depend on
  "the API as of v0.x".
- Windows: `go test ./...` can fail with `unlinkat ... test.exe` when a test
  binary is still running. Retry; it is not a test failure.
- The integration suite wants a clean database. `TestGlobalMentalModelReadsAllNamespaces`
  builds a global model from a cross-namespace recall, so facts left behind by
  earlier runs can push the expected one out of the top N and fail the test.
  Use a fresh database for a full run, not the one used for manual poking.
- `docs/handoff-2026-10-18.md` still lists tags, Reflect and mental models as
  "not built". They shipped on 2026-10-23; the handoff stays as the record of
  that day.
