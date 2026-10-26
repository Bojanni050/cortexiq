# Status

Living page: what this build is, what is switched on, and what is still open.
For the dated snapshot it replaces, see [handoff-2026-10-18](handoff-2026-10-18.md) —
that file is history and is deliberately not updated.

Last updated: 2026-10-26, after the `v0.1.0` deploy.

## Build

| | |
|---|---|
| Latest release | `v0.1.0`, 2026-10-26 |
| Release commit | `f8847f3` |
| How to check | `cortexiq version` (works without a database) |

A binary built from a working tree reports `cortexiq dev (built from source)`.
Release and Docker builds inject the tag, commit and build date; the values live
in `internal/version`.

## What runs where

- Production: the `cortexiq-cortexiq-1` and `cortexiq-cortexiq-ui-1` containers on
  `strato-vps`, in `/opt/cortexiq`, running `v0.1.0` (`cortexiq version` inside the
  container is the source of truth). Migration `00038` applied on the 2026-10-26
  restart.
- Deploying: production pulls the image CI built —
  `ghcr.io/bojanni050/cortexiq:${CORTEXIQ_IMAGE_TAG}`. To deploy, bump
  `CORTEXIQ_IMAGE_TAG` in `/opt/cortexiq/.env` to the new release and run
  `docker compose -f docker-compose.prod.yml up -d`; nothing is compiled on the
  server, so the running binary is byte-for-byte the one the release workflow
  produced (compare `.Image` with the tag's digest). Rolling back is setting the
  previous tag. Migrations run on startup.
- `build: .` is no longer in the prod compose. The old locally built
  `cortexiq:local` image is still on the server as a rollback artifact until it
  is pruned; nothing pulls it any more.
- This page may be one documentation-only commit ahead of the deployed binary;
  compare with `cortexiq version` rather than assuming.
- Storage: Postgres + pgvector. At the last count: 9,526 memories, 65.9 MB,
  1,019 facts, 281 episodes, 24 namespaces.

## Switched off, on purpose

| Setting | Value | Why |
|---|---|---|
| `CORTEXIQ_GRAPH_RECALL` | `false` | Measured on a synthetic 22-question set only (hit@3 0.86 → 0.95). Not yet measured on real memories. |
| `CORTEXIQ_CONSOLIDATION_CANDIDATE_CAP` | `0` | Threshold-only clustering is the known-good behaviour. The lexical arm reached 20/20 on rewordings in eval, but the extra members widen provenance. |
| `CORTEXIQ_ROUTE_RECALL` | `false` | Only widens recall; never measured in production. |
| `CORTEXIQ_RERANKER_MODEL` | empty | Cross-encoder reranking adds a dependency (Jina/SiliconFlow/Ollama). Not decided. |
| `CORTEXIQ_RECALL_MMR_LAMBDA` | `0` | Available per call; off by default so ranking stays plain fusion. |
| `CORTEXIQ_RECALL_MIN_SIMILARITY` | `0` | Default cutoff depends on the embedding model; a wrong value silently drops good hits. |

The first two are the ones most likely to be switched on by accident because
they look like ordinary configuration. They are not: both need a measurement on
real memories first.

## Tests and CI

| | |
|---|---|
| Test functions | 304 across 57 files |
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

There is no undo for a purge, and there is no soft-delete sweep: tombstones stay
in place.

## Open decisions

1. `CORTEXIQ_CONSOLIDATION_CANDIDATE_CAP`: turn on after measuring on real
   memories, or leave at 0.
2. Graph recall: needs a real question set before it is on by default.
3. RRF arm weights: currently equal; no data yet to tune them.
4. Cross-encoder reranker: a new external dependency, not just code.

## Known rough edges

- No semantic versioning history before `v0.1.0`, so nothing can depend on
  "the API as of v0.x".
- Windows: `go test ./...` can fail with `unlinkat ... test.exe` when a test
  binary is still running. Retry; it is not a test failure.
- `docs/handoff-2026-10-18.md` still lists tags, Reflect and mental models as
  "not built". They shipped on 2026-10-23; the handoff stays as the record of
  that day.
