package brain

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/pgvector/pgvector-go"

	"github.com/Bojanni050/cortexiq/internal/queries"
)

// Personalized PageRank for the graph arm (CORTEXIQ_GRAPH_PPR), after HippoRAG
// (arXiv:2405.14831). The one-hop arm gives a fixed score to the neighbours of a
// named entity; PPR spreads the mass of the named entities over the relationship
// graph, so an entity two or three relationships away still scores, an entity
// reached over several paths scores higher, and a hub that splits its mass over
// many edges passes little on to each. The starting values are a first guess to be
// swept with `eval graph` on eval/multihop_queries.jsonl, not tuned numbers.
const (
	// pprUnionExtras caps what the walk adds to the one-hop result in the union.
	pprUnionExtras = 3
	// pprHops bounds the subgraph loaded around the named entities.
	pprHops = 3
	// pprDamping is the chance that the walk follows an edge instead of jumping
	// back to a named entity. HippoRAG uses 0.5, which keeps the mass close to the
	// seeds.
	pprDamping = 0.5
	// pprIterations is the number of power iterations; the walk has converged well
	// before this on a graph of a few hundred edges.
	pprIterations = 30
	// pprMaxEntities is how many of the best-scoring entities contribute facts.
	pprMaxEntities = 12
	// pprFactsPerEntity caps the facts one entity may add, so a hub with dozens of
	// facts cannot fill the whole arm.
	pprFactsPerEntity = 3
)

// graphEdge is one undirected relationship between two lower-cased entities.
type graphEdge struct {
	A, B   string
	Weight float64
}

// personalizedPageRank returns the stationary mass of a random walk that restarts
// at the seeds with probability 1-damping, over the undirected, weighted graph.
// Seeds start with equal mass. The result holds every seed, also one without an
// edge, and every entity reachable from a seed. Only the order of the scores is
// used downstream, so they are not normalised to a distribution.
func personalizedPageRank(edges []graphEdge, seeds []string, damping float64, iterations int) map[string]float64 {
	if len(seeds) == 0 {
		return nil
	}
	type arc struct {
		to string
		w  float64
	}
	adj := map[string][]arc{}
	out := map[string]float64{}
	for _, e := range edges {
		if e.A == e.B || e.Weight <= 0 {
			continue
		}
		adj[e.A] = append(adj[e.A], arc{e.B, e.Weight})
		adj[e.B] = append(adj[e.B], arc{e.A, e.Weight})
		out[e.A] += e.Weight
		out[e.B] += e.Weight
	}

	restart := map[string]float64{}
	for _, s := range seeds {
		restart[s] = 1 / float64(len(seeds))
	}
	p := map[string]float64{}
	for s, m := range restart {
		p[s] = m
	}
	for i := 0; i < iterations; i++ {
		next := map[string]float64{}
		var dangling float64
		for n, m := range p {
			if out[n] == 0 {
				dangling += m
				continue
			}
			for _, a := range adj[n] {
				next[a.to] += damping * m * a.w / out[n]
			}
		}
		// The mass of a node without edges, and the share lost to the restart, goes
		// back to the seeds, so the total stays 1 and an isolated seed keeps its mass.
		back := (1 - damping) + damping*dangling
		for s, m := range restart {
			next[s] += back * m
		}
		p = next
	}
	return p
}

// pprEntities orders the walk's result best first and keeps the top n. Ties break
// on name, so the arm is deterministic.
func pprEntities(scores map[string]float64, n int) (names []string, vals []float32) {
	for name := range scores {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if scores[names[i]] != scores[names[j]] {
			return scores[names[i]] > scores[names[j]]
		}
		return names[i] < names[j]
	})
	if len(names) > n {
		names = names[:n]
	}
	for _, name := range names {
		vals = append(vals, float32(scores[name]))
	}
	return names, vals
}

// pprFactArm is the graph arm of fact recall when CORTEXIQ_GRAPH_PPR is on. It
// walks the relationship graph from the entities the query names and returns facts
// about the best-scoring entities, ordered by entity score and then by closeness to
// the query. It returns the same shape as the one-hop arm, so fusion and routing
// treat it as that arm.
func (b *Brain) pprFactArm(ctx context.Context, nsIDs []int64, query string, vec pgvector.Vector, limit int, minSimilarity float32, asOf, from, to *time.Time, known queries.Known, tags queries.TagFilter) ([]RecallResult, error) {
	seeds := b.matchedEntities(ctx, nsIDs, query)
	if len(seeds) == 0 {
		return nil, nil
	}
	sql, args, err := b.queries.RecallGraphEdges(nsIDs, query, pprHops)
	if err != nil {
		return nil, fmt.Errorf("build graph edges query: %w", err)
	}
	rows, err := b.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("graph edges: %w", err)
	}
	var edges []graphEdge
	for rows.Next() {
		var e graphEdge
		var conf float32
		if err := rows.Scan(&e.A, &e.B, &conf); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan graph edge: %w", err)
		}
		e.Weight = float64(conf)
		edges = append(edges, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	names, scores := pprEntities(personalizedPageRank(edges, seeds, pprDamping, pprIterations), pprMaxEntities)
	isSeed := make([]bool, len(names))
	seedSet := map[string]bool{}
	for _, s := range seeds {
		seedSet[s] = true
	}
	for i, n := range names {
		isSeed[i] = seedSet[n]
	}
	factSQL, factArgs, err := b.queries.RecallFactsPPR(nsIDs, names, scores, isSeed, vec, limit, pprFactsPerEntity, minSimilarity, asOf, from, to, known, tags)
	if err != nil {
		return nil, fmt.Errorf("build ppr fact query: %w", err)
	}
	return b.runRecall(ctx, factSQL, factArgs, "fact")
}

// unionGraphFacts is the graph arm when the walk is added to the one-hop
// expansion (CORTEXIQ_GRAPH_PPR_UNION): every one-hop fact keeps its place, and
// the facts only the walk reached follow in the walk's order. The one-hop result
// stays exactly what the arm gives without the walk, so the walk can only add, and
// it adds at most pprUnionExtras facts: rank fusion hands every fact in the arm a
// bonus whatever its rank, so a long tail of weak extras only blurs the arm.
func unionGraphFacts(oneHop, walked []RecallResult) []RecallResult {
	out := make([]RecallResult, 0, len(oneHop)+pprUnionExtras)
	seen := make(map[int64]bool, len(oneHop)+len(walked))
	for _, r := range oneHop {
		seen[r.ID] = true
		out = append(out, r)
	}
	extras := 0
	for _, r := range walked {
		if extras >= pprUnionExtras {
			break
		}
		if !seen[r.ID] {
			seen[r.ID] = true
			out = append(out, r)
			extras++
		}
	}
	return out
}
