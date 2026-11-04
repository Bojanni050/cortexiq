// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"reflect"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/pgvector/pgvector-go"
)

func candidateIDs(episodes []models.Episode) []int64 {
	var out []int64
	for _, e := range episodes {
		out = append(out, e.ID)
	}
	return out
}

func TestRankCandidatesDropsZeroScoresBelowFloorAndCaps(t *testing.T) {
	candidates := []clusterCandidate{
		{episode: models.Episode{ID: 1}, score: 0.2},
		{episode: models.Episode{ID: 2}, score: 0},
		{episode: models.Episode{ID: 3}, score: 0.9},
		{episode: models.Episode{ID: 4}, score: 0.1},
	}
	got := rankCandidates(candidates, 0, 1)
	if len(got) != 1 || got[0].episode.ID != 3 {
		t.Fatalf("capped = %+v, want only 3", got)
	}
	all := rankCandidates(candidates, 0, 0)
	if len(all) != 3 || all[0].episode.ID != 3 || all[1].episode.ID != 1 || all[2].episode.ID != 4 {
		t.Fatalf("uncapped = %+v, want 3, 1, 4 (zero dropped)", all)
	}
	floored := rankCandidates(candidates, 0.2, 0)
	if len(floored) != 2 || floored[0].episode.ID != 3 || floored[1].episode.ID != 1 {
		t.Fatalf("floored = %+v, want 3 then 1 (0.2 is at the floor, 0.1 below it)", floored)
	}
}

func TestLexicalScoreSeesRewordingNotUnrelatedText(t *testing.T) {
	if got := lexicalScore("CortexIQ draait op poort 9090", "CortexIQ draait op poort 9090"); got != 1 {
		t.Errorf("identical text = %v, want 1", got)
	}
	if got := lexicalScore("CortexIQ, draait op poort 9090.", "cortexiq draait op poort 9090"); got != 1 {
		t.Errorf("case and punctuation must not matter, got %v", got)
	}
	reworded := lexicalScore(
		"CortexIQ draait op poort 9090 na de herstart",
		"CortexIQ luistert op poort 9090 na de herstart",
	)
	unrelated := lexicalScore(
		"CortexIQ draait op poort 9090 na de herstart",
		"De honden krijgen om zeven uur eten",
	)
	if reworded <= unrelated {
		t.Errorf("reworded %v should beat unrelated %v", reworded, unrelated)
	}
	if unrelated > 0.1 {
		t.Errorf("unrelated texts scored %v, want near 0", unrelated)
	}
	if got := lexicalScore("abcdefgh", "01234567"); got != 0 {
		t.Errorf("text with no shared trigram = %v, want 0", got)
	}
	if got := lexicalScore("ok", "ok"); got != 0 {
		t.Errorf("text shorter than a trigram = %v, want 0", got)
	}
}

// clusterBrain builds a brain with clustering config and no infrastructure: the
// clustering path never touches the pool or the reasoner.
func clusterBrain(threshold float64, cap int, floor float64) *Brain {
	return &Brain{config: Config{SimilarityThreshold: threshold, CandidateCap: cap, LexicalFloor: floor}}
}

func clusterEpisode(id int64, content string, vec []float32) models.Episode {
	return models.Episode{ID: id, Content: content, Embedding: pgvector.NewVector(vec)}
}

func TestClusterEpisodesWithoutCapIsThresholdOnly(t *testing.T) {
	episodes := []models.Episode{
		clusterEpisode(1, "CortexIQ draait op poort 9090", []float32{1, 0, 0}),
		clusterEpisode(2, "De API luistert op 9090", []float32{1, 0, 0}),
		// Lexically close to 1, but far in vector space: without a cap the
		// lexical arm does not exist and this stays its own cluster.
		clusterEpisode(3, "CortexIQ draait op poort 9090 na herstart", []float32{0, 1, 0}),
	}

	clusters := clusterEpisodes(episodes, clusterBrain(0.85, 0, 0).config)
	if len(clusters) != 2 {
		t.Fatalf("clusters = %d, want 2", len(clusters))
	}
	if ids := candidateIDs(clusters[0]); !reflect.DeepEqual(ids, []int64{1, 2}) {
		t.Errorf("first cluster = %v, want [1 2]", ids)
	}
	if ids := candidateIDs(clusters[1]); !reflect.DeepEqual(ids, []int64{3}) {
		t.Errorf("second cluster = %v, want [3]", ids)
	}
}

func TestClusterEpisodesWithCapAddsLexicalTwinBelowThreshold(t *testing.T) {
	episodes := []models.Episode{
		clusterEpisode(1, "CortexIQ draait op poort 9090", []float32{1, 0, 0}),
		// Orthogonal embedding, near-identical wording: the twin the threshold misses.
		clusterEpisode(2, "CortexIQ draait op poort 9090 na de herstart", []float32{0, 1, 0}),
		// Orthogonal and lexically unrelated: must not join.
		clusterEpisode(3, "De honden krijgen om zeven uur eten", []float32{0, 0, 1}),
	}

	clusters := clusterEpisodes(episodes, clusterBrain(0.85, 4, 0).config)
	if len(clusters) != 2 {
		t.Fatalf("clusters = %d, want 2", len(clusters))
	}
	if ids := candidateIDs(clusters[0]); !reflect.DeepEqual(ids, []int64{1, 2}) {
		t.Errorf("first cluster = %v, want [1 2]", ids)
	}
	if ids := candidateIDs(clusters[1]); !reflect.DeepEqual(ids, []int64{3}) {
		t.Errorf("second cluster = %v, want [3]", ids)
	}
}

func TestClusterEpisodesCapBoundsOnlyTheLexicalExtras(t *testing.T) {
	episodes := []models.Episode{
		clusterEpisode(1, "CortexIQ draait op poort 9090", []float32{1, 0, 0}),
		// Semantic hits: all of them join, however many there are.
		clusterEpisode(2, "iets heel anders", []float32{1, 0, 0}),
		clusterEpisode(3, "nog iets anders", []float32{1, 0, 0}),
		clusterEpisode(4, "en nog iets", []float32{1, 0, 0}),
		// Lexical candidates below the threshold: only the best cap of them join.
		clusterEpisode(5, "CortexIQ draait op poort 9090 na herstart", []float32{0, 1, 0}),
		clusterEpisode(6, "CortexIQ draait op poort 9091", []float32{0, 0, 1}),
		clusterEpisode(7, "CortexIQ draait op poort 9092", []float32{0, 1, 1}),
	}

	clusters := clusterEpisodes(episodes, clusterBrain(0.85, 2, 0).config)
	if got := len(clusters[0]); got != 1+3+2 {
		t.Fatalf("first cluster has %d episodes, want seed + 3 semantic + cap 2 lexical = 6", got)
	}
	for _, id := range []int64{2, 3, 4} {
		found := false
		for _, e := range clusters[0] {
			found = found || e.ID == id
		}
		if !found {
			t.Errorf("semantic hit %d was dropped by the cap", id)
		}
	}
	// Every episode still lands in exactly one cluster.
	total := 0
	for _, c := range clusters {
		total += len(c)
	}
	if total != len(episodes) {
		t.Errorf("clustered %d of %d episodes", total, len(episodes))
	}
}

func TestClusterEpisodesLexicalFloorKeepsClustersFromPadding(t *testing.T) {
	episodes := []models.Episode{
		clusterEpisode(1, "CortexIQ draait op poort 9090", []float32{1, 0, 0}),
		// Shares only a few trigrams with the seed: above zero, below a sane floor.
		clusterEpisode(2, "De hond draait rondjes bij de poort van het park", []float32{0, 1, 0}),
	}

	padded := clusterEpisodes(episodes, clusterBrain(0.85, 4, 0).config)
	if len(padded) != 1 {
		t.Fatalf("without a floor: %d clusters, want the unrelated episode padded in (1)", len(padded))
	}
	floored := clusterEpisodes(episodes, clusterBrain(0.85, 4, 0.2).config)
	if len(floored) != 2 {
		t.Fatalf("with a floor: %d clusters, want 2", len(floored))
	}
}

func TestClusterEpisodesKeepsSeedWithoutEmbedding(t *testing.T) {
	episodes := []models.Episode{
		{ID: 1, Content: "CortexIQ draait op poort 9090"},
		clusterEpisode(2, "CortexIQ draait op poort 9090", []float32{1, 0, 0}),
	}
	clusters := clusterEpisodes(episodes, clusterBrain(0.85, 4, 0).config)
	if len(clusters) != 2 || len(clusters[0]) != 1 || clusters[0][0].ID != 1 {
		t.Fatalf("clusters = %+v, want [1] then [2]", clusters)
	}
}

func TestTrigramCacheMatchesLexicalScoreAndReuses(t *testing.T) {
	a := models.Episode{ID: 1, Content: "CortexIQ draait op poort 9090"}
	b := models.Episode{ID: 2, Content: "de poort van CortexIQ is 9090"}
	cache := make(trigramCache)

	want := lexicalScore(a.Content, b.Content)
	if got := trigramJaccard(cache.of(a), cache.of(b)); got != want {
		t.Fatalf("cached score = %v, want %v", got, want)
	}
	if len(cache) != 2 {
		t.Fatalf("cache holds %d episodes, want 2", len(cache))
	}
	first := cache.of(a)
	first["sentinel"] = true
	if !cache.of(a)["sentinel"] {
		t.Fatal("second lookup recomputed instead of reusing the cached set")
	}
}

func TestScoreClusterArmsCountsLexicalOnlyTwins(t *testing.T) {
	pairs := []DedupPair{
		// Twins whose vectors are orthogonal: cosine misses them, wording does not.
		{ID: "S1", Label: PairSame, A: "CortexIQ draait op poort 9090", B: "CortexIQ draait op poort 9090 na herstart"},
		// Twins with identical vectors: cosine alone already reaches them.
		{ID: "S2", Label: PairSame, A: "De database is PostgreSQL", B: "Postgres wordt gebruikt als opslag"},
		{ID: "D1", Label: PairDifferent, A: "Consolidatie draait elke vijf minuten", B: "Het logo is blauw"},
	}
	vecs := [][]float32{
		{1, 0, 0}, {0, 1, 0},
		{0, 0, 1}, {0, 0, 1},
		{0.6, 0.6, 0.5}, {0.5, 0.6, 0.6},
	}

	rep := ScoreClusterArms(pairs, vecs, 0.85, []int{2}, []float32{0.2})
	if rep.Seeds != 2 {
		t.Fatalf("seeds = %d, want 2", rep.Seeds)
	}
	if got := rep.Rows[0].TwinReached; got != 1 {
		t.Errorf("threshold-only reached %d twins, want 1 (S2)", got)
	}
	if got := rep.Rows[1].TwinReached; got != 2 {
		t.Errorf("cap 2 reached %d twins, want 2", got)
	}
	if got := rep.Rows[1].LexicalOnly; len(got) != 1 || got[0] != "S1" {
		t.Errorf("lexical-only twins = %v, want [S1]", got)
	}
	if len(rep.Rows[1].Lost) != 0 {
		t.Errorf("lost twins = %v, want none", rep.Rows[1].Lost)
	}
	if len(rep.Unreached) != 0 {
		t.Errorf("unreached = %v, want none", rep.Unreached)
	}
}
