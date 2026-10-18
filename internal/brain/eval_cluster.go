package brain

import (
	"context"
	"fmt"
	"sort"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/pgvector/pgvector-go"
)

// ClusterEvalReport measures what the lexical arm adds to episode clustering.
//
// Every text of the calibration set becomes an episode of one pool. For every
// `same` pair the A text is the seed, and the question is whether its twin B ends
// up in the seed's cluster — the only way the extractor can merge it instead of
// writing a duplicate. Everything else in the pool is, for this purpose, not the
// twin: the other members a cluster gains are the price of the cap.
type ClusterEvalReport struct {
	Model     string  `json:"model"`
	Episodes  int     `json:"episodes"`
	Seeds     int     `json:"seeds"`
	Threshold float32 `json:"threshold"`
	// TwinCosine and TwinLexical are the raw scores of the seed against its twin.
	TwinCosine  LabelStats `json:"twin_cosine"`
	TwinLexical LabelStats `json:"twin_lexical"`
	// Rows has the threshold-only clustering that runs today first (cap 0), then
	// one row per cap and lexical floor.
	Rows []ClusterEvalRow `json:"rows"`
	// Unreached lists the twins no row reaches.
	Unreached []string `json:"unreached,omitempty"`
}

// ClusterEvalRow is the outcome for one cap and floor.
type ClusterEvalRow struct {
	Cap   int     `json:"cap"`
	Floor float32 `json:"floor"`
	// TwinReached counts seeds whose twin is in the seed's cluster.
	TwinReached int `json:"twin_reached"`
	// AvgMembers is the mean number of members a cluster gains, and AvgOther the
	// mean number of those that are not the twin.
	AvgMembers float64 `json:"avg_members"`
	AvgOther   float64 `json:"avg_other"`
	// LexicalOnly lists the twins this row reaches and threshold-only does not.
	LexicalOnly []string `json:"lexical_only,omitempty"`
	// Lost lists the twins threshold-only reaches and this row does not. It must
	// be empty: the lexical arm adds to the cosine arm, it never replaces it.
	Lost []string `json:"lost,omitempty"`
}

// ScoreClusterArms runs the real clusterMembers over the set. vecs holds the
// embedding of pairs[i].A at 2i and of pairs[i].B at 2i+1. It is pure: embedding
// happens in EvalCluster.
func ScoreClusterArms(pairs []DedupPair, vecs [][]float32, threshold float32, caps []int, floors []float32) ClusterEvalReport {
	episodes := make([]models.Episode, 0, 2*len(pairs))
	for i, p := range pairs {
		episodes = append(episodes,
			models.Episode{ID: int64(2*i + 1), Content: p.A, Embedding: pgvector.NewVector(vecs[2*i])},
			models.Episode{ID: int64(2*i + 2), Content: p.B, Embedding: pgvector.NewVector(vecs[2*i+1])},
		)
	}

	rep := ClusterEvalReport{Episodes: len(episodes), Threshold: threshold}

	var seeds []int // pair indexes of the `same` pairs
	var cosines, lexicals []float32
	for i, p := range pairs {
		if p.Label != PairSame {
			continue
		}
		seeds = append(seeds, i)
		cosines = append(cosines, cosineSimilarity(vecs[2*i], vecs[2*i+1]))
		lexicals = append(lexicals, lexicalScore(p.A, p.B))
	}
	rep.Seeds = len(seeds)
	rep.TwinCosine = statsOf(cosines)
	rep.TwinLexical = statsOf(lexicals)

	type setting struct {
		cap   int
		floor float32
	}
	settings := []setting{{0, 0}}
	for _, c := range caps {
		for _, f := range floors {
			settings = append(settings, setting{c, f})
		}
	}

	var baseline map[int]bool // pair index -> twin reached with threshold only
	everReached := map[int]bool{}
	for n, st := range settings {
		b := &Brain{config: Config{SimilarityThreshold: float64(threshold), CandidateCap: st.cap, LexicalFloor: float64(st.floor)}}
		row := ClusterEvalRow{Cap: st.cap, Floor: st.floor}
		reached := map[int]bool{}
		var members, other int
		for _, i := range seeds {
			twinID := int64(2*i + 2)
			clustered := map[int64]bool{int64(2*i + 1): true}
			got := b.clusterMembers(episodes[2*i], episodes, clustered, make(trigramCache))
			hit := false
			for _, m := range got {
				if m.ID == twinID {
					hit = true
				}
			}
			if hit {
				reached[i] = true
				everReached[i] = true
				row.TwinReached++
			}
			members += len(got)
			other += len(got)
			if hit {
				other--
			}
		}
		if n == 0 {
			baseline = reached
		}
		for _, i := range seeds {
			if reached[i] && !baseline[i] {
				row.LexicalOnly = append(row.LexicalOnly, pairs[i].ID)
			}
			if !reached[i] && baseline[i] {
				row.Lost = append(row.Lost, pairs[i].ID)
			}
		}
		if n := float64(len(seeds)); n > 0 {
			row.AvgMembers = float64(members) / n
			row.AvgOther = float64(other) / n
		}
		rep.Rows = append(rep.Rows, row)
	}
	for _, i := range seeds {
		if !everReached[i] {
			rep.Unreached = append(rep.Unreached, pairs[i].ID)
		}
	}
	return rep
}

func statsOf(vals []float32) LabelStats {
	if len(vals) == 0 {
		return LabelStats{}
	}
	sorted := append([]float32(nil), vals...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return LabelStats{Count: len(sorted), Min: sorted[0], Median: median(sorted), Max: sorted[len(sorted)-1]}
}

// EvalCluster embeds every text of the set with the configured embedder and
// scores the clustering arms. Nothing is written to the database. A zero
// threshold takes the configured similarity threshold.
func (b *Brain) EvalCluster(ctx context.Context, pairs []DedupPair, threshold float32, caps []int, floors []float32) (ClusterEvalReport, error) {
	if threshold == 0 {
		threshold = float32(b.config.SimilarityThreshold)
	}
	vecs := make([][]float32, 0, 2*len(pairs))
	for _, p := range pairs {
		for _, text := range []string{p.A, p.B} {
			v, err := b.embedder.Embed(ctx, text)
			if err != nil {
				return ClusterEvalReport{}, fmt.Errorf("embed %s: %w", p.ID, err)
			}
			vecs = append(vecs, v)
		}
	}
	rep := ScoreClusterArms(pairs, vecs, threshold, caps, floors)
	rep.Model = b.embedder.Model()
	return rep, nil
}
