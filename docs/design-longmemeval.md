# Design: LongMemEval Harness

> Status: proposal — not implemented. Written 2026-11-02, as the seventh and
> final gap from `docs/design-gaia-parity.md`: Hindsight's SOTA claim on
> LongMemEval is publicly and independently reproduced; CortexIQ has an
> internal quality eval (`docs/QUALITY_EVALUATION.md`) but no number anyone
> can cite. This document ports the *benchmark*, not an API — the harness it
> builds on already exists.

## What already exists (and is honest)

`cortexiq benchmark` (`internal/benchmark/`) already does the hard part
correctly, and it is worth stating because most projects get this wrong:

- It **ingests** each item's sessions as episodes (runner.go `runItem`) — the
  haystack goes *through the store*, not into the prompt.
- It **answers with `reflect`**, not raw recall — LongMemEval measures
  question answering, and `reflect` is the answer step.
- It carries `question_date` through to time-aware recall, so temporal
  questions query "what was known then", not "what is known now".
- It has both an exact-match judge and an `--llm-judge`, with per-question
  scores, errors, and sources preserved in the report.

The dataset format (`eval/benchmark/README.md`) is LongMemEval-compatible.
What is missing is the official dataset, the fairness protocol, and the
published number.

## Phase 1 — The official dataset, converted

Source: the LongMemEval release (Wu et al., *LongMemEval: Benchmarking Chat
Assistants on Long-Term Interactive Memory*, arXiv:2410.10813), distributed on
HuggingFace. Two variants, both to run:

- **LongMemEval_S** — 500 questions over ~115k-token haystacks. The headline
  number; everything published about the benchmark cites this one.
- **LongMemEval_M** — 2670 questions over ~6k-token sessions with multi-round
  follow-ups. Second line of the table, and the better probe for
  consolidation-heavy stores like CortexIQ.

A converter `cmd/longmemeval-convert` (or `scripts/`) maps the official JSONL
onto the internal format:

| Official field | Internal field | Note |
|---|---|---|
| `question_id` | `question_id` | verbatim — the paper's IDs are the join key for comparisons |
| `haystack_session / haystack_day` | `haystack_sessions[]` | `*_day` items become multiple sessions; keep the official session ids |
| `session_time` | `sessions[].date` | keep the original timestamps; never regenerate them |
| `question_date` | `qa_pairs[].question_date` | drives `as_of` recall |
| `question_type` | `qa_pairs[].category` | mapped verbatim (below) |
| `answer` | `qa_pairs[].answer` | verbatim |

Question-type mapping — the official taxonomy is the breakdown everyone
compares on, so categories must match it exactly, not our own:
`single-session-user`, `single-session-assistant`, `multi-session`,
`temporal-reasoning`, `knowledge-update`, `abstention`, plus the
`session`/`day` suffix. The report keeps the official names.

**Abstention needs one runner change:** official items where the system should
admit ignorance have `answer: "unknown"`. The LLM judge gets an explicit rule
for this category — a "I don't have that in memory" answer scores *correct*,
and any confident guess scores *wrong*. Without that rule the category is
unfairly lost.

## Phase 2 — Fairness protocol

Comparisons are only meaningful if the run is the same shape as the published
ones (the paper's and Hindsight's). Protocol, all non-negotiable:

1. **Isolation.** Every question runs against a fresh namespace ingested from
   its own haystack only. No cross-item leakage; consolidation runs per item,
   after ingest, before the first question (matching how a real memory system
   digests a conversation history).
2. **No context handout.** The question is answered from `reflect` alone —
   no haystack text in the prompt. The runner already guarantees this; keep it.
3. **Fixed answer step.** The reflect synthesis model is pinned and recorded
   (name + version + temperature 0) in the report header. The judge is a
   *different, stronger* model than the answer model — the paper uses a
   GPT-4-class judge — because a model grading its own answers inflates
   scores. Both go in the report header, so a rerun with a newer model is a
   new row, not a correction of the old one.
4. **Both judges reported.** Exact-match alongside the LLM judge, per
   category. Exact-match undercounts paraphrases; LLM judges drift. Showing
   both is the honest middle.
5. **One run, full dataset.** No max-items subsets in published numbers, no
   retries per question. Category-level failures stay visible in the per-item
   JSON, which ships with the report.
6. **As-of discipline.** `question_date` always maps to recall time bounds —
   a knowledge-update item must not be answered with the newer fact.

## Phase 3 — The number and where it goes

- **Report:** `cortexiq benchmark run` gains `--dataset longmemeval_s` /
  `longmemeval_m` (converted files live in `eval/benchmark/` or are fetched
  by script — dataset license permitting, checked before vendoring).
- **Per-category breakdown** in the summary: overall, session vs day, and the
  six ability categories, with both judge scores.
- **Published page:** `docs/BENCHMARKS.md` — one table, per variant:
  CortexIQ (model, judge model, date) with a link to the run artifact, and
  the published Hindsight / paper baselines *quoted with their source*,
  clearly marked as their measurements, not ours. We never run their system
  and claim the comparison; we run ours and place it next to their published
  numbers.
- **Regression guard:** the full run is too expensive for CI, but
  `eval/benchmark/example.json` already smoke-tests the path. Optional:
  a 50-question pinned subset as a nightly job with a tolerance band, so a
  recall regression shows up in days, not at the next publication.

## Cost and time budget

Rough order, to be re-measured on the first run: LongMemEval_S means ~115k
tokens ingested per question — embedding + consolidation across 500 items —
plus 500 reflect calls and 500 judge calls. With local Ollama this is hours,
not minutes; with API models it is a double-digit dollar figure. Two levers
that do *not* compromise the protocol: consolidate per item (already isolated)
and run items in parallel (CPU-bound embedding is the bottleneck, not the
store). The budget estimate goes in `docs/BENCHMARKS.md` alongside the numbers
so the cost of the claim is as visible as the claim.

## What this deliberately does not do

- **No benchmark-specific tuning.** If recall loses points on
  `knowledge-update`, the fix is a better recall (e.g. phase 3 of the parity
  design), not a benchmark special case. Any change motivated by a category
  failure gets its own rationale and lands on main, then the benchmark is
  re-run.
- **No re-judging.** Scores are final when published; a better judge model
  means a new dated row.
- **No day-level shortening.** The `*_day` variants exist specifically because
  session-level compression makes memory look better than it is; running only
  `*_S` would flatter any store with aggressive consolidation — including ours.

## Rollout order

1. Converter + the two dataset files (license check first).
2. Abstention judge rule + per-category official taxonomy in the report.
3. Protocol wiring: pinned models in the header, per-item artifact output.
4. First full LongMemEval_S run → `docs/BENCHMARKS.md` with the table.
5. LongMemEval_M run, same table, second row.
