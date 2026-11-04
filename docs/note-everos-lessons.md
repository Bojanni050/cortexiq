# Note: lessons from EverOS

> Status: research note, not a design. Written 2026-11-03 after reading the
> code of `EverMind-AI/EverOS` (commit `962c63e`): `docs/reflection.md`,
> `src/everos/memory/reflection/`, `memory/strategies/` (agent case and skill
> extraction, foresight, finish_reflection), and `memory/search/`
> (`agentic.py`, `llm_multiround.py`, `hierarchy.py`). Nothing here is decided.

## What EverOS is

A local-first Python memory runtime. Markdown files are the source of truth;
SQLite and LanceDB are indexes synced from them. It extracts episodes and atomic
facts from conversations, clusters episodes, and has an offline "Reflection"
that merges a cluster into one narrative. It has a separate agent track (cases
and skills) and several search modes up to LLM-guided multi-round retrieval.

## Limits of this reading

- The extraction prompts and ranking operators live in a separate package,
  `everalgo`, which is not in the repo. Extraction quality cannot be judged
  from here.
- The repo contains a benchmark harness (`benchmarks/`) but no result numbers
  that I could find. Nothing below is backed by their published scores.
- No forgetting or decay, and no graph memory in the core runtime. CortexIQ is
  ahead there (`decay.go`, `graph.go`).

## Worth taking

### 1. Retire the old only after the new is complete

Reflection writes the merged narrative, re-extracts its facts, and only then
marks the replaced episodes and their facts `deprecated_by`
(`strategies/finish_reflection.py`). If fact extraction never succeeds, nothing
is deprecated: the originals stay searchable rather than being replaced by a
narrative without facts.

For CortexIQ: the supersede write in `design-gaia-parity.md` phase 3 should
make the old row invalid and the new row visible in **one transaction**, never
the other order. Postgres gives this for free; the point is to state it as an
invariant and test it (a failed write must leave the old row valid).

### 2. A merge is lossy; do not repeat it blindly

Their own documentation says each Reflection run is a lossy LLM merge, that
re-consolidating the same memories can make the narrative worse, and that weekly
is the recommended maximum. They also update incrementally: a cluster already
consolidated only folds in the new fragments.

CortexIQ already gates mental-model refresh on a watermark
(`mentalModelStaleReason` in `internal/brain/mental_model.go`: refresh only when
a newer source exists), which covers the "do not rerun for nothing" half. Open
question, not verified: whether a refresh re-reads the sources or re-summarises
its own previous output. The former does not compound loss; the latter does.
Check this before adding any automatic refresh schedule.

### 3. Multi-round retrieval with per-sub-query fusion

`search/llm_multiround.py`: each round an LLM decider sees the question and the
evidence so far, then selects the core memories, stops, or emits one focused
sub-query per missing aspect. Each sub-query is recalled and fused with RRF
**independently**, and the final list is assembled core-first, then each
sub-query's best remaining candidate, then the rest by the **maximum** RRF score
across sub-queries (max, not sum, so a specialist ranked first for one
sub-query keeps that score).

For CortexIQ: relevant to questions that need several facets ("what do we know
about X and why did we choose Y"). It would sit on top of the existing recall
arms, probably inside `reflect`, not as a replacement for plain recall. It costs
LLM calls per question, so it must stay opt-in.

### 4. Measure the reranker, do not assume it

Their module notes the cross-encoder was measured net-negative for them: it
scored the gold answers below RRF order and buried many past rank 20, so they
use RRF alone. That is their data and model, not a verdict on ours.

For CortexIQ: run `internal/brain/eval_recall.go` with the reranker on and off
on our own corpus and LongMemEval (`design-longmemeval.md`). If the reranker does
not win, it costs latency for nothing.

### 5. Persist why something was skipped

Every memcell run is stamped with `skip_reason` (and detail) whether or not a
case came out, so "why is this not in my memory" is a query, not a log search.
CortexIQ counts skipped episodes (`EpisodesSkipped` in `ConsolidationResult`)
but, as far as I can see, does not store the reason per episode.

For CortexIQ: a small `skip_reason` on the episode (noise gate, too short,
duplicate, extraction empty) would make the dashboard answer that question.
Cheap, and useful for debugging the noise gate.

### 6. No write-only output

Their foresight extraction is off by default because nothing reads it: it would
spend one LLM call per sender per memcell on data no search route surfaces.

For CortexIQ: list what consolidation produces (facts, relationships, causal
links, patterns, hypotheses) and confirm each has a reader, in recall, `reflect`,
or the dashboard. Anything without one is a cost with no use.

## Not worth taking

- **Agent cases and skills** (quality score, correctness judge gate, skills
  distilled from clusters of cases, `source_case_ids` lineage). It is a second
  product track with its own clustering and prompts. The light version for
  CortexIQ is a `case` tag next to `create_failure` (a positive counterpart: an
  approach that worked, with context); the heavy version only if cases pile up.
- **Markdown as source of truth, SQLite and LanceDB, the cascade watcher.** It
  solves consistency between files and indexes, which Postgres does not have.
  `export.go` already covers the Git-friendly export.
- **Multimodal ingestion** via an optional parser and LibreOffice. A different
  product.

## Suggested order, if any of this is picked up

1. Item 1 (transaction invariant and test) belongs inside phase 3, no extra work.
2. Item 4 (reranker on/off measurement) is an experiment with an existing tool.
3. Item 5 (stored skip reason) is a small migration and a dashboard column.
4. Items 2 and 6 are checks of existing code, no new feature.
5. Item 3 only after item 4, and only if recall on multi-facet questions
   measurably lags.
