package brain

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const (
	defaultGraphNodes = 150
	maxGraphNodes     = 500
	// maxGraphRows bounds how many relationships one graph request reads.
	maxGraphRows = 100000
)

// GraphOptions narrows what QueryGraph returns.
type GraphOptions struct {
	// Canonical keeps only edges with one of these canonical types. Empty keeps all.
	Canonical []string
	// OnlyMapped drops edges that have no canonical_type.
	OnlyMapped bool
	// MinConfidence drops edges below this confidence.
	MinConfidence float32
	// Query focuses the graph on entities whose name contains it (case-insensitive),
	// plus their direct neighbours. Empty shows the best-connected entities.
	Query string
	// MaxNodes caps the number of entities returned (default 150, max 500).
	MaxNodes int
}

// GraphNode is one entity.
type GraphNode struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Degree      int    `json:"degree"`       // distinct edges in the (filtered) graph
	NamespaceID int64  `json:"namespace_id"` // namespace with most of its edges
}

// GraphEdge is one distinct statement between two entities. Repeats of the same
// statement (same ends, same canonical type or wording) are folded into Count.
type GraphEdge struct {
	ID          string   `json:"id"`
	Source      string   `json:"source"`
	Target      string   `json:"target"`
	Canonical   string   `json:"canonical,omitempty"`
	Label       string   `json:"label"` // canonical type, else the original wording
	Types       []string `json:"types"` // original wordings folded into this edge
	Confidence  float32  `json:"confidence"`
	Count       int      `json:"count"`
	FactID      *int64   `json:"fact_id,omitempty"`
	NamespaceID int64    `json:"namespace_id"`
}

// Graph is the knowledge graph slice returned to the UI.
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
	// TotalNodes and TotalEdges count the whole filtered graph, before the node cap.
	TotalNodes int  `json:"total_nodes"`
	TotalEdges int  `json:"total_edges"`
	Truncated  bool `json:"truncated"`
	// CanonicalCounts is how many distinct edges each canonical type has (ignoring
	// the Canonical/OnlyMapped filters, so a filter UI can show every choice).
	CanonicalCounts map[string]int `json:"canonical_counts"`
	Unmapped        int            `json:"unmapped"`
}

// graphRow is one relationships row as read from the database.
type graphRow struct {
	NamespaceID int64
	From, To    string
	Type        string
	Canonical   *string
	Confidence  float32
	FactID      *int64
}

// QueryGraph builds the knowledge graph for the given namespaces (each includes
// its descendants).
func (b *Brain) QueryGraph(ctx context.Context, namespaceSlugs []string, opts GraphOptions) (*Graph, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}
	rows, err := b.pool.Query(ctx,
		`SELECT namespace_id, from_entity, relation_type, canonical_type, to_entity, confidence, source_fact_id
		 FROM relationships WHERE namespace_id = ANY($1) AND deleted_at IS NULL
		 ORDER BY id LIMIT $2`, nsIDs, maxGraphRows)
	if err != nil {
		return nil, fmt.Errorf("query graph: %w", err)
	}
	defer rows.Close()

	var all []graphRow
	for rows.Next() {
		var r graphRow
		if err := rows.Scan(&r.NamespaceID, &r.From, &r.Type, &r.Canonical, &r.To, &r.Confidence, &r.FactID); err != nil {
			return nil, fmt.Errorf("scan graph row: %w", err)
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return buildGraph(all, opts), nil
}

type edgeAgg struct {
	GraphEdge
	types map[string]bool
}

func entityKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// buildGraph is the pure part of QueryGraph: fold repeats into edges, filter, pick
// the entities to show and cut the graph down to them.
func buildGraph(rows []graphRow, opts GraphOptions) *Graph {
	if opts.MaxNodes <= 0 {
		opts.MaxNodes = defaultGraphNodes
	}
	if opts.MaxNodes > maxGraphNodes {
		opts.MaxNodes = maxGraphNodes
	}
	wantCanon := map[string]bool{}
	for _, c := range opts.Canonical {
		if c = strings.TrimSpace(c); c != "" {
			wantCanon[c] = true
		}
	}

	// 1. Fold rows into distinct edges. Two statements are the same when they
	// connect the same entities with the same canonical type (or, without one, the
	// same lowercased wording): "includes" and "contains" collapse, "uses" and
	// "does_not_use" do not.
	labels := map[string]map[string]int{} // entity key -> spelling -> count
	nsVotes := map[string]map[int64]int{} // entity key -> namespace -> edges
	edges := map[string]*edgeAgg{}
	var order []string
	for _, r := range rows {
		if r.Confidence < opts.MinConfidence {
			continue
		}
		from, to := entityKey(r.From), entityKey(r.To)
		if from == "" || to == "" || from == to {
			continue
		}
		canon := ""
		if r.Canonical != nil {
			canon = *r.Canonical
		}
		kind := canon
		if kind == "" {
			kind = "~" + strings.ToLower(strings.TrimSpace(r.Type))
		}
		for _, e := range [2]struct{ key, spelling string }{{from, r.From}, {to, r.To}} {
			if labels[e.key] == nil {
				labels[e.key] = map[string]int{}
			}
			labels[e.key][strings.TrimSpace(e.spelling)]++
		}
		id := from + "\x00" + kind + "\x00" + to
		e := edges[id]
		if e == nil {
			label := canon
			if label == "" {
				label = strings.TrimSpace(r.Type)
			}
			e = &edgeAgg{
				GraphEdge: GraphEdge{ID: id, Source: from, Target: to, Canonical: canon, Label: label,
					Confidence: r.Confidence, FactID: r.FactID, NamespaceID: r.NamespaceID},
				types: map[string]bool{},
			}
			edges[id] = e
			order = append(order, id)
			for _, k := range [2]string{from, to} {
				if nsVotes[k] == nil {
					nsVotes[k] = map[int64]int{}
				}
				nsVotes[k][r.NamespaceID]++
			}
		}
		e.Count++
		e.types[strings.TrimSpace(r.Type)] = true
		if r.Confidence > e.Confidence {
			e.Confidence = r.Confidence
		}
	}

	g := &Graph{CanonicalCounts: map[string]int{}, Nodes: []GraphNode{}, Edges: []GraphEdge{}}
	var kept []*edgeAgg
	for _, id := range order {
		e := edges[id]
		if e.Canonical == "" {
			g.Unmapped++
		} else {
			g.CanonicalCounts[e.Canonical]++
		}
		if opts.OnlyMapped && e.Canonical == "" {
			continue
		}
		if len(wantCanon) > 0 && !wantCanon[e.Canonical] {
			continue
		}
		kept = append(kept, e)
	}

	// 2. Degrees over the filtered graph.
	degree := map[string]int{}
	neighbours := map[string]map[string]bool{}
	for _, e := range kept {
		degree[e.Source]++
		degree[e.Target]++
		for _, p := range [2][2]string{{e.Source, e.Target}, {e.Target, e.Source}} {
			if neighbours[p[0]] == nil {
				neighbours[p[0]] = map[string]bool{}
			}
			neighbours[p[0]][p[1]] = true
		}
	}
	g.TotalNodes, g.TotalEdges = len(degree), len(kept)

	// 3. Choose the entities to show.
	byDegree := func(keys []string) {
		sort.Slice(keys, func(i, j int) bool {
			if degree[keys[i]] != degree[keys[j]] {
				return degree[keys[i]] > degree[keys[j]]
			}
			return keys[i] < keys[j]
		})
	}
	selected := map[string]bool{}
	if q := strings.ToLower(strings.TrimSpace(opts.Query)); q != "" {
		var seeds []string
		for k := range degree {
			if strings.Contains(k, q) {
				seeds = append(seeds, k)
			}
		}
		byDegree(seeds)
		for _, s := range seeds {
			if len(selected) >= opts.MaxNodes {
				break
			}
			selected[s] = true
		}
		// Then neighbours of the matches, best connected first.
		var around []string
		seen := map[string]bool{}
		for s := range selected {
			for n := range neighbours[s] {
				if !selected[n] && !seen[n] {
					seen[n] = true
					around = append(around, n)
				}
			}
		}
		byDegree(around)
		for _, n := range around {
			if len(selected) >= opts.MaxNodes {
				break
			}
			selected[n] = true
		}
	} else {
		keys := make([]string, 0, len(degree))
		for k := range degree {
			keys = append(keys, k)
		}
		byDegree(keys)
		if len(keys) > opts.MaxNodes {
			keys = keys[:opts.MaxNodes]
		}
		for _, k := range keys {
			selected[k] = true
		}
	}
	g.Truncated = len(selected) < len(degree)

	// 4. Edges between selected entities; entities without any of them are dropped.
	shown := map[string]bool{}
	for _, e := range kept {
		if !selected[e.Source] || !selected[e.Target] {
			continue
		}
		e.Types = make([]string, 0, len(e.types))
		for t := range e.types {
			e.Types = append(e.Types, t)
		}
		sort.Strings(e.Types)
		g.Edges = append(g.Edges, e.GraphEdge)
		shown[e.Source], shown[e.Target] = true, true
	}
	for k := range shown {
		g.Nodes = append(g.Nodes, GraphNode{ID: k, Label: topSpelling(labels[k]), Degree: degree[k], NamespaceID: topNamespace(nsVotes[k])})
	}
	sort.Slice(g.Nodes, func(i, j int) bool {
		if g.Nodes[i].Degree != g.Nodes[j].Degree {
			return g.Nodes[i].Degree > g.Nodes[j].Degree
		}
		return g.Nodes[i].ID < g.Nodes[j].ID
	})
	return g
}

// topSpelling returns the most common spelling (ties: the lexically smallest).
func topSpelling(m map[string]int) string {
	best, bestN := "", -1
	for s, n := range m {
		if n > bestN || (n == bestN && s < best) {
			best, bestN = s, n
		}
	}
	return best
}

func topNamespace(m map[int64]int) int64 {
	var best int64
	bestN := -1
	for ns, n := range m {
		if n > bestN || (n == bestN && ns < best) {
			best, bestN = ns, n
		}
	}
	return best
}
