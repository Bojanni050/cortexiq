package brain

import "testing"

func canon(s string) *string { return &s }

func gr(from, typ string, c *string, to string, conf float32) graphRow {
	return graphRow{NamespaceID: 1, From: from, Type: typ, Canonical: c, To: to, Confidence: conf}
}

func edgeBetween(g *Graph, from, to string) *GraphEdge {
	for i := range g.Edges {
		if g.Edges[i].Source == from && g.Edges[i].Target == to {
			return &g.Edges[i]
		}
	}
	return nil
}

func TestBuildGraphFoldsRepeatsAndWordings(t *testing.T) {
	g := buildGraph([]graphRow{
		gr("Bo", "uses", canon("uses"), "Kilo Code", 0.8),
		gr("bo", "uses", canon("uses"), "KILO CODE", 0.95),
		gr("kit", "includes", canon("contains"), "battery", 0.8),
		gr("kit", "contains", canon("contains"), "battery", 0.8),
		gr("kit", "reachable_at", nil, "port", 0.8),
		gr("kit", "Reachable_At ", nil, "port", 0.9),
		gr("kit", "does_not_use", nil, "battery", 0.8),
	}, GraphOptions{})

	if len(g.Edges) != 4 {
		t.Fatalf("want 4 distinct edges, got %d: %+v", len(g.Edges), g.Edges)
	}
	use := edgeBetween(g, "bo", "kilo code")
	if use == nil || use.Count != 2 || use.Confidence != 0.95 || use.Label != "uses" {
		t.Errorf("repeats should fold into one edge with the best confidence: %+v", use)
	}
	has := edgeBetween(g, "kit", "battery")
	if has == nil {
		t.Fatal("missing kit -> battery")
	}
	var contains *GraphEdge
	for i := range g.Edges {
		if g.Edges[i].Canonical == "contains" {
			contains = &g.Edges[i]
		}
	}
	if contains == nil || contains.Count != 2 || len(contains.Types) != 2 {
		t.Errorf("includes/contains should fold, keeping both wordings: %+v", contains)
	}
	var free *GraphEdge
	for i := range g.Edges {
		if g.Edges[i].Canonical == "" && g.Edges[i].Target == "port" {
			free = &g.Edges[i]
		}
	}
	if free == nil || free.Count != 2 || free.Label != "reachable_at" {
		t.Errorf("unmapped wordings fold on their lowercased text: %+v", free)
	}
	if g.CanonicalCounts["uses"] != 1 || g.CanonicalCounts["contains"] != 1 || g.Unmapped != 2 {
		t.Errorf("counts: %v unmapped=%d", g.CanonicalCounts, g.Unmapped)
	}
	for _, n := range g.Nodes {
		if n.ID == "bo" && n.Label != "Bo" {
			t.Errorf("label should be the most common spelling, got %q", n.Label)
		}
	}
}

func TestBuildGraphFilters(t *testing.T) {
	rows := []graphRow{
		gr("a", "uses", canon("uses"), "b", 0.9),
		gr("a", "owns", canon("owns"), "c", 0.9),
		gr("a", "weird", nil, "d", 0.9),
		gr("a", "uses", canon("uses"), "e", 0.5),
		gr("a", "self", canon("uses"), "a", 0.9),
	}
	if g := buildGraph(rows, GraphOptions{Canonical: []string{"owns"}}); len(g.Edges) != 1 || g.Edges[0].Target != "c" {
		t.Errorf("canonical filter: %+v", g.Edges)
	}
	if g := buildGraph(rows, GraphOptions{OnlyMapped: true}); len(g.Edges) != 3 {
		t.Errorf("only mapped should drop the free-text edge: %+v", g.Edges)
	}
	if g := buildGraph(rows, GraphOptions{MinConfidence: 0.8}); edgeBetween(g, "a", "e") != nil {
		t.Errorf("min confidence should drop a -> e")
	}
	g := buildGraph(rows, GraphOptions{Canonical: []string{"owns"}})
	if g.CanonicalCounts["uses"] != 3-1 { // a->b, a->e (self-loop ignored)
		t.Errorf("filter UI counts must ignore the canonical filter, got %v", g.CanonicalCounts)
	}
	for _, e := range buildGraph(rows, GraphOptions{}).Edges {
		if e.Source == e.Target {
			t.Errorf("self loops are dropped: %+v", e)
		}
	}
}

func TestBuildGraphCapsAndFocus(t *testing.T) {
	// A hub with three leaves, plus a separate pair.
	rows := []graphRow{
		gr("hub", "has", canon("has"), "l1", 0.9),
		gr("hub", "has", canon("has"), "l2", 0.9),
		gr("hub", "has", canon("has"), "l3", 0.9),
		gr("x", "uses", canon("uses"), "y", 0.9),
	}
	g := buildGraph(rows, GraphOptions{MaxNodes: 2})
	if !g.Truncated || g.TotalNodes != 7-1 {
		t.Errorf("total/truncated: %+v", g)
	}
	if len(g.Nodes) != 0 && g.Nodes[0].ID != "hub" {
		t.Errorf("best-connected entity first: %+v", g.Nodes)
	}

	f := buildGraph(rows, GraphOptions{Query: "HUB"})
	if len(f.Nodes) != 4 || len(f.Edges) != 3 {
		t.Errorf("focus on hub should return it with its neighbours: nodes=%d edges=%d", len(f.Nodes), len(f.Edges))
	}
	if e := buildGraph(rows, GraphOptions{Query: "nothing like this"}); len(e.Nodes) != 0 || len(e.Edges) != 0 {
		t.Errorf("no match -> empty graph: %+v", e)
	}
}

func TestBuildGraphNamespaceIsTheBusiestOne(t *testing.T) {
	r1 := gr("a", "uses", canon("uses"), "b", 0.9)
	r2 := gr("a", "owns", canon("owns"), "c", 0.9)
	r3 := gr("a", "has", canon("has"), "d", 0.9)
	r1.NamespaceID, r2.NamespaceID, r3.NamespaceID = 5, 9, 9
	g := buildGraph([]graphRow{r1, r2, r3}, GraphOptions{})
	for _, n := range g.Nodes {
		if n.ID == "a" && n.NamespaceID != 9 {
			t.Errorf("namespace of a = %d, want 9", n.NamespaceID)
		}
	}
}
