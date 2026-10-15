# Dedup candidates: one uncalibrated number decides what counts as the same memory

Found while reading the Hindsight engine (`vectorize-io/hindsight`, MIT, ~160k lines
of Python) against CortexIQ's consolidation path. Two independent observations, one
proposal. Everything below is read from source; nothing here is benchmarked yet.

## What CortexIQ does today

`internal/brain/consolidate.go` clusters the episode batch before extraction:

```go
// consolidate.go:256
clusters := b.clusterEpisodes(extractable)
```

and inside `clusterEpisodes` (`consolidate.go:380-395`), for each seed it walks
**every** episode in the batch and adds those that pass one test:

```go
sim := cosineSimilarity(seedVec, candVec)
if sim >= float32(b.config.SimilarityThreshold) {
    cluster = append(cluster, candidate)
}
```

`SimilarityThreshold` is `CORTEXIQ_CONSOLIDATION_SIMILARITY_THRESHOLD`, default
**0.85** (`internal/config/config.go:66`).

The cluster is the unit of work: its episodes go into **one** consolidation call, and
`FactsDeduplicated` counts what gets merged *within* that call. So a twin that misses
the cluster never enters the call, and becomes a duplicate fact instead of a merge.

One number, raw cosine, one arm — that is the whole candidacy decision.

## Why that is fragile, for two separate reasons

**1. The number is not calibrated.** Measured on this codebase's own data, cosine did
not separate signal from noise: in a recall sample the noise population sat *above*
the relevant one on `similarity` (noise ≈0.52, relevant ≈0.44), which is why recall
thresholds on the RRF score instead (see `extractor-findings.md` and `GETTING_STARTED`).
The hybrid-recall cutoff is documented as "Model dependent" (`internal/brain/brain.go:91`).
Dedup has no such fallback: it is 100% raw cosine against a fixed 0.85, and 0.85 is a
property of the embedding model, not of the data.

**2. There is only one arm.** Vector similarity is the only signal consulted. A
reworded or translated twin has little lexical overlap and can sit below the cutoff,
and nothing else gets a vote.

## What Hindsight found, and it is the mirror image

Their recall fuses four arms — semantic, BM25, graph (spreading activation) and
temporal — with the same algorithm and the same constant as CortexIQ:
`reciprocal_rank_fusion(..., k=60)` in
`hindsight-api-slim/hindsight_api/engine/search/fusion.py`. `internal/brain/recall.go:55`
uses `rrfK = 60`.

But they ship a **second** fusion, `interleave_fusion()`, and the docstring names the
exact failure it exists for:

> RRF scores a doc by the *sum* of its reciprocal ranks across arms, so a result that
> is #1 in one arm but absent/low in the others gets averaged down. That is exactly the
> consolidation-dedup failure mode: the near-identical existing observation (the "twin"
> to merge into) is semantic rank #1, yet shares no source-fact graph link and little
> lexical overlap, so RRF drops it below the recall budget cutoff and the LLM never
> sees it → creates a duplicate.

Interleave instead guarantees every arm's top hits a slot: all #1s, then all #2s, and
so on, in arm-priority order. They also cap each arm before fusion (`cap_per_source`)
so one over-expanding arm cannot crowd the others out of the reranker budget.

So they lose the twin to *averaging*; CortexIQ loses it to a *cutoff*. Both are a
single decision made by a single derived score. Their per-arm result type also models
absence explicitly — `ArmScores.semantic` is `None` when "the doc was not surfaced by
that arm" — which is the general form of the pitfall we hit with CortexIQ's missing
`similarity` field on keyword-only hits.

## Proposal, smallest step first

**Step 1 — measure the number instead of inheriting it.** On an isolated eval namespace
(the method already in `QUALITY_EVALUATION.md`), build ~50 labelled pairs: true twins
(rewordings, translations, one detail added) and near-misses (same topic, different
claim). Record raw cosine for every pair. That gives the distribution, the valley, and a
defensible threshold — the same procedure that produced the 0.6 recall threshold, and it
directly serves the existing `Deduplication >= 90%` target. Until this exists, 0.85 is
a guess with a comment.

**Step 2 — give the twin a second route in.** Two options, both cheap, neither needing
new infrastructure:

- *Multi-arm candidacy.* Add a second generator to the candidate set — the full-text
  search that hybrid recall already uses, or a trigram similarity on the episode text
  (Hindsight carries trigram indexes for entities for exactly this kind of fuzzy match).
  Widen the union, let the *LLM* make the merge call, since it already sees the cluster.
- *Interleave instead of threshold-only.* Rank candidates per arm and take each arm's
  top-k round-robin. A twin that is #1 in one arm then always reaches the call, even
  when the other arm disagrees. That is their `interleave_fusion`, and it is a dozen
  lines on top of the arms you already have.

**Step 3 — make the near-miss band visible.** Add a counter for "episode produced a new
fact while a near-twin (cosine in [0.75, threshold)) existed in the same namespace".
That band is where duplicates are born, and today nothing reports on it. With that number,
the dedup dimension in `QUALITY_EVALUATION.md` cites evidence instead of a target.

## Not verified

I read their Python; I did not run Hindsight, and I did not measure CortexIQ's dedup
rate. The cosine figures quoted are from the recall sample in `extractor-findings.md`,
not from a dedup-specific measurement — which is precisely the gap Step 1 closes.
