// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"reflect"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/pgvector/pgvector-go"
)

func armOf(name string, ids ...int64) candidateArm {
	arm := candidateArm{name: name}
	for i, id := range ids {
		// Scores only order the arm; their scale differs per arm.
		arm.candidates = append(arm.candidates, clusterCandidate{
			episode: models.Episode{ID: id},
			score:   float32(len(ids) - i),
		})
	}
	return arm
}

func candidateIDs(episodes []models.Episode) []int64 {
	var out []int64
	for _, e := range episodes {
		out = append(out, e.ID)
	}
	return out
}

func TestInterleaveCandidatesPutsEveryArmsTopFirst(t *testing.T) {
	// The semantic arm ranks 1 first; the lexical arm misses 1 at depth 0 but
	// ranks 4 first. Both #1s must come before any #2.
	got := interleaveCandidates([]candidateArm{
		armOf("semantic", 1, 2, 3),
		armOf("lexical", 4, 1, 5),
	}, 5)

	want := []int64{1, 4, 2, 3, 5}
	if ids := candidateIDs(got); !reflect.DeepEqual(ids, want) {
		t.Fatalf("order = %v, want %v", ids, want)
	}
}

func TestInterleaveCandidatesSkipsDuplicatesAndRespectsLimit(t *testing.T) {
	arms := []candidateArm{
		armOf("semantic", 1, 2, 3),
		armOf("lexical", 1, 2, 3),
	}
	got := interleaveCandidates(arms, 3)
	if ids := candidateIDs(got); !reflect.DeepEqual(ids, []int64{1, 2, 3}) {
		t.Fatalf("order = %v, want [1 2 3]", ids)
	}
	if got := interleaveCandidates(arms, 0); got != nil {
		t.Errorf("limit 0 = %+v, want nil", got)
	}
	if got := interleaveCandidates(nil, 3); len(got) != 0 {
		t.Errorf("no arms = %+v, want empty", got)
	}
}

func TestInterleaveCandidatesStopsWhenEveryArmIsSpent(t *testing.T) {
	got := interleaveCandidates([]candidateArm{armOf("semantic", 1, 2)}, 10)
	if ids := candidateIDs(got); !reflect.DeepEqual(ids, []int64{1, 2}) {
		t.Fatalf("order = %v, want [1 2]", ids)
	}
}

func TestRankCandidatesDropsZeroScoresAndCaps(t *testing.T) {
	candidates := []clusterCandidate{
		{episode: models.Episode{ID: 1}, score: 0.2},
		{episode: models.Episode{ID: 2}, score: 0},
		{episode: models.Episode{ID: 3}, score: 0.9},
	}
	got := rankCandidates(candidates, 1)
	if len(got) != 1 || got[0].episode.ID != 3 {
		t.Fatalf("capped = %+v, want only 3", got)
	}
	all := rankCandidates(candidates, 0)
	if len(all) != 2 || all[0].episode.ID != 3 || all[1].episode.ID != 1 {
		t.Fatalf("uncapped = %+v, want 3 then 1 (zeros dropped)", all)
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
func clusterBrain(threshold float64, cap int) *Brain {
	return &Brain{config: Config{SimilarityThreshold: threshold, CandidateCap: cap}}
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

	clusters := clusterBrain(0.85, 0).clusterEpisodes(episodes)
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

	clusters := clusterBrain(0.85, 4).clusterEpisodes(episodes)
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

func TestClusterEpisodesCapBoundsTheCluster(t *testing.T) {
	episodes := []models.Episode{
		clusterEpisode(1, "CortexIQ draait op poort 9090", []float32{1, 0, 0}),
		clusterEpisode(2, "CortexIQ draait op poort 9090", []float32{1, 0, 0}),
		clusterEpisode(3, "CortexIQ draait op poort 9091", []float32{1, 0, 0}),
		clusterEpisode(4, "CortexIQ draait op poort 9092", []float32{1, 0, 0}),
		clusterEpisode(5, "CortexIQ draait op poort 9093", []float32{1, 0, 0}),
	}

	clusters := clusterBrain(0.85, 2).clusterEpisodes(episodes)
	if len(clusters[0]) != 3 {
		t.Fatalf("first cluster has %d episodes, want seed + cap = 3", len(clusters[0]))
	}
	// Every episode still lands in exactly one cluster; the cap only moves the
	// rest into later ones.
	total := 0
	for _, c := range clusters {
		total += len(c)
	}
	if total != len(episodes) {
		t.Errorf("clustered %d of %d episodes", total, len(episodes))
	}
}

func TestClusterEpisodesKeepsSeedWithoutEmbedding(t *testing.T) {
	episodes := []models.Episode{
		{ID: 1, Content: "CortexIQ draait op poort 9090"},
		clusterEpisode(2, "CortexIQ draait op poort 9090", []float32{1, 0, 0}),
	}
	clusters := clusterBrain(0.85, 4).clusterEpisodes(episodes)
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
