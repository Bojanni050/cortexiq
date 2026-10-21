// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"math"

	"github.com/Bojanni050/cortexiq/internal/embedder"
)

// MMR (Maximal Marginal Relevance) diversifies a ranked list of recall results.
// Instead of returning the top-N by relevance alone, it greedily picks results
// that balance two goals:
//
//  1. Relevance: how well the result matches the query (its Score).
//  2. Diversity: how different the result is from what is already selected.
//
// The balance is controlled by lambda (λ):
//
//   - λ = 0: pure diversity (pick the most different result each time)
//   - λ = 1: pure relevance (pick the highest-scoring result each time)
//   - λ = 0.5: balanced (Hindsight's default)
//
// The MMR score for candidate c is:
//
//	MMR(c) = λ * sim(c, query) - (1-λ) * max_{s in selected} sim(c, s)
//
// where sim is cosine similarity. The first pick is always the top-scoring
// result (there is nothing to be diverse from yet).
//
// Results without an embedding (keyword-only, graph-only, temporal-only hits)
// are treated as maximally diverse: they never reduce another result's MMR
// score, and they are picked by their relevance alone. This keeps MMR from
// pushing out results that the vector search could not judge.

// applyMMR reorders results using Maximal Marginal Relevance. It runs after
// RRF fusion and reranking, before recency/importance boosts, so those still
// reorder the diversified list. lambda is clamped to [0,1]; 0 disables MMR and
// returns the input unchanged.
//
// The query embedding is needed to score relevance; results carry their own
// embeddings (populated by runRecall when MMR is enabled). Results without an
// embedding are kept in their relative order at the position their score puts
// them, because MMR cannot measure their similarity to anything.
func applyMMR(results []RecallResult, queryVec []float32, lambda float64, limit int) []RecallResult {
	if lambda <= 0 || len(results) <= 1 {
		return results
	}
	if lambda > 1 {
		lambda = 1
	}

	// Separate results with embeddings (MMR-eligible) from those without.
	// Results without embeddings keep their score-based position relative to
	// each other but do not participate in the diversity penalty.
	type candidate struct {
		idx    int
		res    RecallResult
		vec    []float32
		hasVec bool
	}

	var withVec, withoutVec []candidate
	for i, r := range results {
		c := candidate{idx: i, res: r, vec: r.embedding, hasVec: len(r.embedding) > 0}
		if c.hasVec {
			withVec = append(withVec, c)
		} else {
			withoutVec = append(withoutVec, c)
		}
	}

	// If nothing has an embedding, MMR has nothing to diversify: keep order.
	if len(withVec) == 0 {
		return results
	}

	selected := make([]candidate, 0, len(results))
	selectedVecs := make([][]float32, 0, len(results))
	remaining := make([]candidate, len(withVec))
	copy(remaining, withVec)

	for len(selected) < limit && len(remaining) > 0 {
		bestIdx := -1
		bestScore := math.Inf(-1)

		for i, c := range remaining {
			relevance := float64(c.res.Score)
			var maxSim float32
			for _, sv := range selectedVecs {
				sim := cosineSimilarity(c.vec, sv)
				if sim > maxSim {
					maxSim = sim
				}
			}
			mmrScore := lambda*relevance - (1-lambda)*float64(maxSim)
			if mmrScore > bestScore {
				bestScore = mmrScore
				bestIdx = i
			}
		}

		if bestIdx < 0 {
			break
		}
		c := remaining[bestIdx]
		c.res.mmrScore = float32(bestScore)
		selected = append(selected, c)
		selectedVecs = append(selectedVecs, c.vec)
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	// Interleave: MMR-selected results first (in MMR order), then the
	// no-embedding results in their original score order. This keeps every
	// result that was in the input, just reordered.
	out := make([]RecallResult, 0, len(results))
	for _, c := range selected {
		out = append(out, c.res)
	}
	for _, c := range withoutVec {
		out = append(out, c.res)
	}
	return out
}

// applyMMRWithEmbedder is applyMMR for callers that have not pre-fetched
// embeddings. It embeds each result's content on the fly, which is slower but
// correct when the recall query did not select embeddings. Prefer passing
// pre-fetched embeddings via RecallResult.embedding when possible.
func applyMMRWithEmbedder(ctx context.Context, e embedder.Embedder, results []RecallResult, queryVec []float32, lambda float64, limit int) []RecallResult {
	if lambda <= 0 || len(results) <= 1 {
		return results
	}
	// Populate embeddings for results that do not have them yet.
	for i := range results {
		if len(results[i].embedding) == 0 {
			vec, err := e.Embed(ctx, results[i].Content)
			if err == nil {
				results[i].embedding = vec
			}
		}
	}
	return applyMMR(results, queryVec, lambda, limit)
}

// mmrEnabled reports whether MMR is configured for this recall. lambda > 0
// enables it; the value controls the relevance/diversity balance.
func mmrEnabled(lambda float64) bool {
	return lambda > 0
}
