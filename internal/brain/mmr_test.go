// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"math"
	"testing"
)

// vec builds a simple 2D unit vector from an angle (for readable test vectors).
func vec(angle float64) []float32 {
	return []float32{float32(math.Cos(angle)), float32(math.Sin(angle))}
}

// vecOf builds a vector from explicit components.
func vecOf(x, y float32) []float32 {
	return []float32{x, y}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b []float32
		want float32
	}{
		{"identical", vecOf(1, 0), vecOf(1, 0), 1.0},
		{"orthogonal", vecOf(1, 0), vecOf(0, 1), 0.0},
		{"opposite", vecOf(1, 0), vecOf(-1, 0), -1.0},
		{"45 degrees", vecOf(1, 0), vec(0.785398), float32(math.Cos(0.785398))}, // ~0.707
		{"empty", nil, vecOf(1, 0), 0.0},
		{"zero vector", vecOf(0, 0), vecOf(1, 0), 0.0},
		{"dimension mismatch", vecOf(1, 0), []float32{1, 0, 0}, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			if math.Abs(float64(got-tt.want)) > 0.001 {
				t.Errorf("cosineSimilarity() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestApplyMMRDisabled(t *testing.T) {
	results := []RecallResult{
		{ID: 1, Content: "a", Score: 0.9, embedding: vecOf(1, 0)},
		{ID: 2, Content: "b", Score: 0.8, embedding: vecOf(0.9, 0.1)},
	}
	queryVec := vecOf(1, 0)

	// lambda = 0 disables MMR: order unchanged.
	got := applyMMR(results, queryVec, 0, 10)
	if got[0].ID != 1 || got[1].ID != 2 {
		t.Errorf("lambda=0 should not reorder, got order %d, %d", got[0].ID, got[1].ID)
	}

	// Empty results.
	got = applyMMR(nil, queryVec, 0.5, 10)
	if len(got) != 0 {
		t.Errorf("empty input should give empty output, got %d", len(got))
	}

	// Single result.
	got = applyMMR(results[:1], queryVec, 0.5, 10)
	if len(got) != 1 {
		t.Errorf("single result should stay, got %d", len(got))
	}
}

func TestApplyMMRPureRelevance(t *testing.T) {
	// Three results: two very similar to each other, one different but lower score.
	results := []RecallResult{
		{ID: 1, Content: "similar A", Score: 0.95, embedding: vecOf(1, 0)},
		{ID: 2, Content: "similar B", Score: 0.90, embedding: vecOf(0.99, 0.01)}, // almost identical to 1
		{ID: 3, Content: "different", Score: 0.70, embedding: vecOf(0, 1)},       // orthogonal
	}
	queryVec := vecOf(1, 0)

	// lambda = 1: pure relevance, no diversity penalty. Order unchanged.
	got := applyMMR(results, queryVec, 1.0, 10)
	if got[0].ID != 1 || got[1].ID != 2 || got[2].ID != 3 {
		t.Errorf("lambda=1 should keep relevance order, got %d, %d, %d", got[0].ID, got[1].ID, got[2].ID)
	}
}

func TestApplyMMRPureDiversity(t *testing.T) {
	// Two near-identical high-scoring results, one different lower-scoring.
	results := []RecallResult{
		{ID: 1, Content: "similar A", Score: 0.95, embedding: vecOf(1, 0)},
		{ID: 2, Content: "similar B", Score: 0.94, embedding: vecOf(0.99, 0.01)}, // nearly identical
		{ID: 3, Content: "different", Score: 0.50, embedding: vecOf(0, 1)},       // orthogonal
	}
	queryVec := vecOf(1, 0)

	// lambda = 0.01: near-pure diversity. First pick is still the top scorer
	// (ID 1, nothing to be diverse from yet). Second pick should be the most
	// different (ID 3, orthogonal), because the diversity gain outweighs the
	// relevance loss. (lambda = 0 exactly disables MMR entirely.)
	got := applyMMR(results, queryVec, 0.01, 10)
	if got[0].ID != 1 {
		t.Errorf("first pick should be top scorer, got ID %d", got[0].ID)
	}
	if got[1].ID != 3 {
		t.Errorf("near-zero lambda second pick should be most diverse (ID 3), got ID %d", got[1].ID)
	}
}

func TestApplyMMRBalanced(t *testing.T) {
	// lambda = 0.5: balanced. The very similar pair should be split by the
	// different result if the diversity gain outweighs the relevance loss.
	results := []RecallResult{
		{ID: 1, Content: "A", Score: 0.90, embedding: vecOf(1, 0)},
		{ID: 2, Content: "B (similar)", Score: 0.85, embedding: vecOf(0.98, 0.02)}, // sim to 1 ~0.999
		{ID: 3, Content: "C (different)", Score: 0.60, embedding: vecOf(0, 1)},     // sim to 1 = 0
	}
	queryVec := vecOf(1, 0)

	got := applyMMR(results, queryVec, 0.5, 10)

	// First pick: ID 1 (highest score, nothing selected yet).
	if got[0].ID != 1 {
		t.Errorf("first pick should be ID 1, got ID %d", got[0].ID)
	}

	// Second pick: MMR score for ID 2 = 0.5*0.85 - 0.5*0.999 ≈ 0.425 - 0.5 = -0.075
	// MMR score for ID 3 = 0.5*0.60 - 0.5*0 = 0.30
	// So ID 3 wins: diversity beats the small relevance gap.
	if got[1].ID != 3 {
		t.Errorf("lambda=0.5 second pick should be ID 3 (diverse), got ID %d", got[1].ID)
	}
	if got[2].ID != 2 {
		t.Errorf("third pick should be ID 2, got ID %d", got[2].ID)
	}
}

func TestApplyMMRWithLimit(t *testing.T) {
	results := []RecallResult{
		{ID: 1, Score: 0.9, embedding: vecOf(1, 0)},
		{ID: 2, Score: 0.8, embedding: vecOf(0, 1)},
		{ID: 3, Score: 0.7, embedding: vecOf(1, 0)},
		{ID: 4, Score: 0.6, embedding: vecOf(0, 1)},
	}
	queryVec := vecOf(1, 0)

	// Limit 2: only 2 results returned.
	got := applyMMR(results, queryVec, 0.5, 2)
	if len(got) != 2 {
		t.Errorf("limit 2 should return 2 results, got %d", len(got))
	}
}

func TestApplyMMRNoEmbeddings(t *testing.T) {
	// Results without embeddings: MMR cannot diversify, order unchanged.
	results := []RecallResult{
		{ID: 1, Score: 0.9, Content: "no emb"},
		{ID: 2, Score: 0.8, Content: "no emb"},
	}
	queryVec := vecOf(1, 0)

	got := applyMMR(results, queryVec, 0.5, 10)
	if got[0].ID != 1 || got[1].ID != 2 {
		t.Errorf("no embeddings should keep order, got %d, %d", got[0].ID, got[1].ID)
	}
}

func TestApplyMMRMixedEmbeddings(t *testing.T) {
	// Mix of results with and without embeddings. No-embedding results are
	// appended after the MMR-selected ones, in their original order.
	results := []RecallResult{
		{ID: 1, Score: 0.9, embedding: vecOf(1, 0)},
		{ID: 2, Score: 0.85, Content: "keyword only"}, // no embedding
		{ID: 3, Score: 0.8, embedding: vecOf(0, 1)},
		{ID: 4, Score: 0.75, Content: "graph only"}, // no embedding
	}
	queryVec := vecOf(1, 0)

	got := applyMMR(results, queryVec, 0.5, 10)
	if len(got) != 4 {
		t.Fatalf("all 4 results should be returned, got %d", len(got))
	}
	// MMR-selected (with embeddings) come first: ID 1, then ID 3 (diverse).
	if got[0].ID != 1 {
		t.Errorf("first should be ID 1, got ID %d", got[0].ID)
	}
	if got[1].ID != 3 {
		t.Errorf("second should be ID 3 (diverse), got ID %d", got[1].ID)
	}
	// No-embedding results follow in original order.
	if got[2].ID != 2 || got[3].ID != 4 {
		t.Errorf("no-embedding results should follow in order, got %d, %d", got[2].ID, got[3].ID)
	}
}

func TestApplyMMRLambdaClamping(t *testing.T) {
	results := []RecallResult{
		{ID: 1, Score: 0.9, embedding: vecOf(1, 0)},
		{ID: 2, Score: 0.8, embedding: vecOf(0, 1)},
	}
	queryVec := vecOf(1, 0)

	// lambda > 1 is clamped to 1 (pure relevance).
	got := applyMMR(results, queryVec, 1.5, 10)
	if got[0].ID != 1 || got[1].ID != 2 {
		t.Errorf("lambda>1 should clamp to 1 (relevance order), got %d, %d", got[0].ID, got[1].ID)
	}
}

func TestMMREnabled(t *testing.T) {
	if mmrEnabled(0) {
		t.Error("mmrEnabled(0) should be false")
	}
	if mmrEnabled(-0.5) {
		t.Error("mmrEnabled(-0.5) should be false")
	}
	if !mmrEnabled(0.5) {
		t.Error("mmrEnabled(0.5) should be true")
	}
	if !mmrEnabled(1.0) {
		t.Error("mmrEnabled(1.0) should be true")
	}
}

func TestApplyMMRPreservesAllResults(t *testing.T) {
	// MMR must never drop a result: it only reorders.
	results := []RecallResult{
		{ID: 1, Score: 0.9, embedding: vecOf(1, 0)},
		{ID: 2, Score: 0.8, embedding: vecOf(0.9, 0.1)},
		{ID: 3, Score: 0.7, embedding: vecOf(0, 1)},
		{ID: 4, Score: 0.6, Content: "no emb"},
		{ID: 5, Score: 0.5, embedding: vecOf(-1, 0)},
	}
	queryVec := vecOf(1, 0)

	got := applyMMR(results, queryVec, 0.3, 10)
	if len(got) != len(results) {
		t.Fatalf("MMR dropped results: got %d, want %d", len(got), len(results))
	}

	// All IDs must be present.
	seen := map[int64]bool{}
	for _, r := range got {
		seen[r.ID] = true
	}
	for _, r := range results {
		if !seen[r.ID] {
			t.Errorf("ID %d missing from MMR output", r.ID)
		}
	}
}
