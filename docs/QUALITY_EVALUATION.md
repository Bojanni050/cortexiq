# CortexIQ quality evaluation

Use this procedure before changing the reasoner model, prompts, consolidation
thresholds, or relationship batch size. Section 4 covers the recall side, for
changes to the embedding model, hybrid recall, or the recall weights. It keeps
model quality separate from real user memory and produces a comparable result.

## 1. Use an isolated namespace

Create a temporary namespace such as `/eval/2026-10-13-qwen3` and use only
synthetic examples. Do not copy customer, user, or project memories into the
evaluation set.

Each example should contain one or more explicit claims, including cases with:

- names, versions, paths, ports, and numbers;
- two independent claims in one episode;
- a translation or rewording of an existing fact;
- compatible values versus a genuine contradiction;
- an explicit relationship and a statement with no relationship;
- an unsupported or ambiguous claim that should produce no fact.

Store the examples with `remember`, run one `consolidate` after the batch, then
inspect `query_facts`, `query_relationships`, and `list_contradictions`.

## 2. Score every run

Record the model, settings, date, reasoner calls, prompt tokens, completion
tokens, retries, and these scores:

| Dimension | Question | Target |
|---|---|---:|
| Fact recall | Were all explicit durable claims extracted? | >= 95% |
| Specificity | Were names, numbers, versions and paths preserved? | 100% |
| Grounding | Were unsupported claims invented? | 0 |
| Separation | Were independent claims split into separate facts? | >= 90% |
| Deduplication | Were rewordings merged without losing new details? | >= 90% |
| Relationships | Are extracted edges explicit and correctly attributed? | >= 90% |

The run fails if any unsupported claim is accepted, a critical number/name is
changed, or the reasoner exceeds the configured call budget.

Deduplication rests on cosine similarity, which on our own data separates signal
from noise less cleanly than the vector model suggests (see section 4 on
ranking): these thresholds are properties of the model, not measurements of the
data. A new fact is weighed against `CORTEXIQ_DEDUP_CANDIDATES` nearest facts
instead of one, so a twin that is not the closest neighbour still reaches the LLM
review. Every consolidation run also reports `dedup_near_misses`: facts stored as
new while a near-twin (similarity >= 0.75) was already there. That band is where
duplicates are born, and it is where the calibration should start — label ~50
pairs (rewordings, translations, near-misses) and record the raw cosine.

That set exists: `eval/dedup_pairs.jsonl` holds 68 labelled pairs (`same`,
`near-miss`, `different`). Score it against the configured embedder with

```bash
cortexiq eval dedup            # cosine per label, threshold sweep, effect of the current thresholds
cortexiq eval dedup --llm      # also asks the reasoner about the pairs in the review band
```

Read the report like this: `auto_merged_other` must be 0 (a false merge loses
information), `missed_same` is the twins no test ever merges, and the sweep shows
at which threshold the `same` and `near-miss` distributions separate. Run it again
whenever the embedding model changes. It writes nothing to the database. The set
is synthetic and was written by Claude; the labels still need a human check
before the thresholds are changed on the strength of them.

Measured 2026-10-16 (bge-m3, 68 pairs, `--threshold 0.99 --review-threshold 0.75
--llm`, so the whole band goes to the reasoner): cosine alone cannot separate
`same` (0.77–0.97) from `near-miss` (0.58–0.95); at 0.85/0.80 eleven of seventeen
near-misses were auto-merged. With the review prompt that names subject, role and
scope as new information, three runs gave 1 false merge (N35, warning vs critical
threshold) and 0 missed twins each time. The highest near-miss is 0.955 and the
highest twin 0.965, so an auto-merge threshold of 0.98 sits above everything
measured; the range between 0.965 and 0.98 holds no pairs and is an estimate.

Clustering has its own check, on the same set: `cortexiq eval cluster` pools every
text as an episode and asks, for each `same` pair, whether the twin lands in the
seed's cluster with threshold-only clustering and with each `--cap` (default 2, 4
and 8). It reports how many twins the lexical arm adds (`only with cap N`), how many
other members a cluster gains (`avg other`, the provenance price of the cap) and
which twins no cap reaches. It needs the configured embedder and writes nothing.

## 3. Compare models safely

Run the same examples with the old and new configuration. Keep the raw JSONL
export and the score sheet for each run. Compare quality and completion tokens;
the cheapest model is not better if it drops identifiers or causes retries.

The relevant Prometheus metrics are:

```text
reasoner_requests_total{stage,status}
reasoner_prompt_tokens_total{stage}
reasoner_completion_tokens_total{stage}
```

Consolidation results also expose `llm_calls` and `llm_calls_limited`.

## 4. Recall ranking regression (fast)

Run this whenever the embedding model, `CORTEXIQ_HYBRID_RECALL`, the recall
weights (`CORTEXIQ_RECALL_*`), or `CORTEXIQ_RECALL_MIN_SIMILARITY` changes. It
takes a minute and catches both a broken keyword ranker and a shifted RRF
weighting.

1. Pick eight identifiers that occur exactly once in the corpus — a doc name, an
   env var, a file path, a function name. Confirm uniqueness first:

```sql
SELECT count(*) FROM facts WHERE deleted_at IS NULL AND content ILIKE '%GETTING_STARTED%';
```

2. Query each one verbatim and record the top hit:

```bash
curl -s -X POST http://localhost:9090/api/v1/recall \
  -H 'Content-Type: application/json' -d '{"query":"GETTING_STARTED","namespaces":["/"]}' \
  | jq '.results[0] | {content, score, similarity, match}'
```

3. Passing means, for every identifier:

| Check | Target |
|---|---:|
| The fact containing the identifier | rank 1 |
| `score` | >= 0.8 |
| `match` | `hybrid` (or `vector` with hybrid recall off) |

A rare identifier that comes back as keyword-only means the vector ranker
missed something it should find; the candidate pool is only `limit * 3` per
ranker (capped at 300), so that can happen for a common term but not for one
that occurs once.

Measured 2026-10-15 (qwen3 embedder, hybrid on, 344 facts): 8/8 top-1, all
`hybrid`, scores 0.81–0.89.

4. Never filter on `similarity` instead of `score`. In that same run the matching
   facts spanned 0.51–0.74 on `similarity` while their `score` was 0.81–0.89, so
   a `similarity >= 0.6` filter would have dropped four of the eight, including
   the best hit. `similarity` is the raw cosine of the vector ranker (absent
   entirely on keyword-only hits, which is why it must never drive ranking);
   `score` is the fused 0..1 rank score. Keyword-only rows — a stopword query
   such as "een" returns them at about 0.42 — are exactly what a score threshold
   is meant to drop.

### Labelled recall set

The eight-identifier check above only tests the keyword ranker. For everything
else there is `eval/recall_corpus.jsonl` (60 synthetic memories, Dutch and
English, with deliberate near-twins as distractors) and `eval/recall_queries.jsonl`
(36 questions in four kinds: `identifier`, `paraphrase`, `cross-language` and
`confuser`, where a near-twin must not outrank the answer):

```bash
cortexiq eval recall           # hit@1, hit@3, hit@k and MRR per kind, plus every miss
```

It stores the corpus as episodes in a temporary `/eval/recall-<time>` namespace,
asks the questions through the normal recall path (configured embedder, hybrid
ranking, weights) and deletes the namespace again; `--keep` leaves it. Only
episode results are ranked, because the corpus has no facts. Run it before and
after any change to the embedding model, the recall weights or a reranker, and
compare the per-kind rows, not just the overall number. The set is small (36
questions), so treat a difference of one question as noise.

Baseline 2026-10-18 (bge-m3, hybrid recall, production weights, k=10):

| kind | n | hit@1 | hit@3 | hit@10 | MRR |
|---|---:|---:|---:|---:|---:|
| identifier | 8 | 1.00 | 1.00 | 1.00 | 1.000 |
| paraphrase | 10 | 0.70 | 1.00 | 1.00 | 0.817 |
| cross-language | 8 | 0.50 | 1.00 | 1.00 | 0.708 |
| confuser | 10 | 0.80 | 0.90 | 1.00 | 0.875 |
| overall | 36 | 0.75 | 0.97 | 1.00 | 0.850 |

Every answer is in the top 10 and all but one in the top 3, so the headroom is
ordering (hit@1), not coverage. All four cross-language misses have the same
shape: a document in the language of the question outranks the answer in the
other language (q19, q21, q22, q26). q11 and q10 are defensible alternatives
(a read-only-token memory and a reinforcement memory also bear on the question),
so the set understates recall slightly.

## 5. Clean up

After scoring, remove the isolated namespace and its exported evaluation file.
Never leave synthetic evaluation data mixed into production namespaces.
