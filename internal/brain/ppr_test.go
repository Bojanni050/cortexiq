package brain

import (
	"math"
	"os"
	"regexp"
	"strings"
	"testing"
)

func lower(s string) string { return strings.ToLower(s) }

// entityDistances is the number of relationships between every pair of corpus
// entities (lower case), over the undirected graph. Pairs that do not connect are
// missing.
func entityDistances(rels []GraphRel) map[string]map[string]int {
	adj := map[string][]string{}
	for _, r := range rels {
		a, b := lower(r.From), lower(r.To)
		adj[a], adj[b] = append(adj[a], b), append(adj[b], a)
	}
	out := map[string]map[string]int{}
	for start := range adj {
		d := map[string]int{start: 0}
		queue := []string{start}
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			for _, m := range adj[n] {
				if _, seen := d[m]; !seen {
					d[m] = d[n] + 1
					queue = append(queue, m)
				}
			}
		}
		out[start] = d
	}
	return out
}

// namedEntity is the one corpus entity (lower case) a question names as a whole
// word, or "" when it names none or several.
func namedEntity(facts []GraphFact, query string) string {
	found := map[string]bool{}
	for _, f := range facts {
		re := regexp.MustCompile(`(?i)(^|[^[:alnum:]])` + regexp.QuoteMeta(f.Entity) + `([^[:alnum:]]|$)`)
		if re.MatchString(query) {
			found[lower(f.Entity)] = true
		}
	}
	if len(found) != 1 {
		return ""
	}
	for e := range found {
		return e
	}
	return ""
}

// chain is a - b - c - d, with a hub h that also touches a and e, f, g.
func chain() []graphEdge {
	return []graphEdge{
		{"a", "b", 1}, {"b", "c", 1}, {"c", "d", 1},
	}
}

func TestPersonalizedPageRankDecaysWithDistance(t *testing.T) {
	p := personalizedPageRank(chain(), []string{"a"}, pprDamping, pprIterations)
	if !(p["a"] > p["b"] && p["b"] > p["c"] && p["c"] > p["d"] && p["d"] > 0) {
		t.Errorf("mass should fall along the chain and still reach d: %v", p)
	}
}

func TestPersonalizedPageRankRewardsSeveralPaths(t *testing.T) {
	// x is reached from the seed over two routes, y over one.
	edges := []graphEdge{{"s", "m1", 1}, {"s", "m2", 1}, {"s", "m3", 1}, {"m1", "x", 1}, {"m2", "x", 1}, {"m3", "y", 1}}
	p := personalizedPageRank(edges, []string{"s"}, pprDamping, pprIterations)
	if p["x"] <= p["y"] {
		t.Errorf("x (two paths) = %v should outscore y (one path) = %v", p["x"], p["y"])
	}
}

func TestPersonalizedPageRankHubPassesLittleOn(t *testing.T) {
	// The seed reaches a quiet entity q directly and a hub that splits over many
	// edges; the hub's neighbours get less than q does.
	edges := []graphEdge{{"s", "q", 1}, {"s", "hub", 1}}
	for _, n := range []string{"n1", "n2", "n3", "n4", "n5", "n6"} {
		edges = append(edges, graphEdge{"hub", n, 1})
	}
	p := personalizedPageRank(edges, []string{"s"}, pprDamping, pprIterations)
	if p["n1"] >= p["q"] {
		t.Errorf("a hub neighbour (%v) should score below a direct neighbour (%v)", p["n1"], p["q"])
	}
}

func TestPersonalizedPageRankKeepsIsolatedSeedsAndSplitsSeedMass(t *testing.T) {
	p := personalizedPageRank(chain(), []string{"a", "lonely"}, pprDamping, pprIterations)
	if p["lonely"] <= 0 {
		t.Errorf("a seed without edges keeps its mass: %v", p)
	}
	var sum float64
	for _, m := range p {
		sum += m
	}
	if math.Abs(sum-1) > 1e-6 {
		t.Errorf("total mass = %v, want 1", sum)
	}
	if personalizedPageRank(chain(), nil, pprDamping, pprIterations) != nil {
		t.Error("no seeds, no walk")
	}
}

func TestPersonalizedPageRankIgnoresLoopsAndZeroWeights(t *testing.T) {
	p := personalizedPageRank([]graphEdge{{"a", "a", 1}, {"a", "b", 0}}, []string{"a"}, pprDamping, pprIterations)
	if _, ok := p["b"]; ok {
		t.Errorf("a zero-weight edge must not carry mass: %v", p)
	}
}

func TestPPREntitiesOrdersBestFirstAndIsDeterministic(t *testing.T) {
	names, vals := pprEntities(map[string]float64{"b": 0.2, "a": 0.2, "c": 0.5, "d": 0.01}, 3)
	if len(names) != 3 || names[0] != "c" || names[1] != "a" || names[2] != "b" {
		t.Errorf("names = %v, want [c a b]", names)
	}
	if len(vals) != 3 || vals[0] < vals[1] {
		t.Errorf("vals = %v", vals)
	}
}

func TestShippedMultihopSetLoads(t *testing.T) {
	cf, err := os.Open("../../eval/graph_corpus.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer cf.Close()
	facts, rels, err := LoadGraphCorpus(cf)
	if err != nil {
		t.Fatal(err)
	}
	qf, err := os.Open("../../eval/multihop_queries.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer qf.Close()
	qs, err := LoadGraphQueries(qf, facts)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, q := range qs {
		kinds[q.Kind]++
	}
	if kinds[GraphQueryHop2] == 0 || kinds[GraphQueryHop3] == 0 {
		t.Errorf("the multihop set needs hop2 and hop3 questions: %v", kinds)
	}

	// Every question must be as far away as its kind says: the nearest expected fact
	// sits at least that many relationships from every entity the question names,
	// so the one-hop arm cannot answer it by construction.
	dist := entityDistances(rels)
	entityOf := map[string]string{}
	for _, f := range facts {
		entityOf[f.ID] = f.Entity
	}
	want := map[string]int{GraphQueryHop2: 2, GraphQueryHop3: 3}
	for _, q := range qs {
		seed := namedEntity(facts, q.Query)
		if seed == "" {
			t.Errorf("%s must name exactly one corpus entity", q.ID)
			continue
		}
		best := -1
		for _, id := range q.Expect {
			if d, ok := dist[seed][lower(entityOf[id])]; ok && (best < 0 || d < best) {
				best = d
			}
		}
		if best < 0 {
			t.Errorf("%s: no expected fact is connected to %s", q.ID, seed)
		} else if best != want[q.Kind] {
			t.Errorf("%s (%s): nearest expected fact is %d relationships from %s, want %d", q.ID, q.Kind, best, seed, want[q.Kind])
		}
	}
}

func TestUnionGraphFactsKeepsOneHopOrderAndAppendsOnlyNewFacts(t *testing.T) {
	ids := func(rs []RecallResult) []int64 {
		var out []int64
		for _, r := range rs {
			out = append(out, r.ID)
		}
		return out
	}
	oneHop := []RecallResult{{ID: 3}, {ID: 1}}
	walked := []RecallResult{{ID: 1}, {ID: 7}, {ID: 3}, {ID: 5}, {ID: 8}, {ID: 9}}
	got := ids(unionGraphFacts(oneHop, walked))
	want := []int64{3, 1, 7, 5, 8}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if got := ids(unionGraphFacts(nil, walked)); len(got) != pprUnionExtras {
		t.Errorf("an empty one-hop result keeps the whole walk: %v", got)
	}
	if got := ids(unionGraphFacts(oneHop, nil)); len(got) != 2 {
		t.Errorf("an empty walk keeps the one-hop result: %v", got)
	}
}

// namedEntities lists every corpus entity (lower case) a question names as a
// whole word.
func namedEntities(facts []GraphFact, query string) []string {
	found := map[string]bool{}
	for _, f := range facts {
		re := regexp.MustCompile(`(?i)(^|[^[:alnum:]])` + regexp.QuoteMeta(f.Entity) + `([^[:alnum:]]|$)`)
		if re.MatchString(query) {
			found[lower(f.Entity)] = true
		}
	}
	var out []string
	for e := range found {
		out = append(out, e)
	}
	return out
}

// sharedContentWords counts the words of six letters or more that a question
// shares with a fact, matching on a common six-letter start so that "vernieuwd"
// and "vernieuwen" or "werkgeheugen" and "geheugen" count as one word.
func sharedContentWords(query, fact string) int {
	words := func(s string) []string {
		var out []string
		for _, w := range regexp.MustCompile(`[\p{L}\p{N}]+`).FindAllString(strings.ToLower(s), -1) {
			if len([]rune(w)) >= 6 {
				out = append(out, w)
			}
		}
		return out
	}
	n := 0
	fw := words(fact)
	for _, q := range words(query) {
		for _, f := range fw {
			if q == f || strings.Contains(q, f) || strings.Contains(f, q) || string([]rune(q)[:6]) == string([]rune(f)[:6]) {
				n++
				break
			}
		}
	}
	return n
}

// TestShippedLargeGraphSetShape guards eval/graph_corpus_large.jsonl and
// eval/graph_queries_large.jsonl: every question sits exactly as many
// relationships from the entity it names as its kind says, a control names no
// entity, and three relationships reach only a small part of the graph, which the
// 26-fact corpus could not offer.
func TestShippedLargeGraphSetShape(t *testing.T) {
	cf, err := os.Open("../../eval/graph_corpus_large.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer cf.Close()
	facts, rels, err := LoadGraphCorpus(cf)
	if err != nil {
		t.Fatal(err)
	}
	qf, err := os.Open("../../eval/graph_queries_large.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer qf.Close()
	qs, err := LoadGraphQueries(qf, facts)
	if err != nil {
		t.Fatal(err)
	}

	entities := map[string]bool{}
	entityOf := map[string]string{}
	for _, f := range facts {
		entities[lower(f.Entity)] = true
		entityOf[f.ID] = lower(f.Entity)
	}
	dist := entityDistances(rels)
	want := map[string]int{GraphQueryDirect: 0, GraphQueryNeighbour: 1, GraphQueryHop2: 2, GraphQueryHop3: 3}

	kinds := map[string]int{}
	for _, q := range qs {
		kinds[q.Kind]++
		named := namedEntities(facts, q.Query)
		if q.Kind == GraphQueryControl {
			if len(named) != 0 {
				t.Errorf("%s is a control but names %v", q.ID, named)
			}
			continue
		}
		if len(named) != 1 {
			t.Errorf("%s must name exactly one corpus entity, names %v", q.ID, named)
			continue
		}
		seed := named[0]
		best := -1
		for _, id := range q.Expect {
			d, ok := dist[seed][entityOf[id]]
			if entityOf[id] == seed {
				d, ok = 0, true
			}
			if ok && (best < 0 || d < best) {
				best = d
			}
		}
		if best != want[q.Kind] {
			t.Errorf("%s (%s): nearest expected fact is %d relationships from %s, want %d", q.ID, q.Kind, best, seed, want[q.Kind])
		}
		if q.Kind == GraphQueryHop2 || q.Kind == GraphQueryHop3 {
			// A question that repeats the answer's words is solved by the vector arm and
			// says nothing about the graph: allow one shared word at most.
			content := map[string]string{}
			for _, f := range facts {
				content[f.ID] = f.Content
			}
			for _, id := range q.Expect {
				if n := sharedContentWords(q.Query, content[id]); n > 1 {
					t.Errorf("%s shares %d long words with its answer %s; rephrase it", q.ID, n, id)
				}
			}
		}
		if q.Kind == GraphQueryHop3 {
			within := 0
			for _, d := range dist[seed] {
				if d <= 3 {
					within++
				}
			}
			if share := float64(within) / float64(len(entities)); share > 0.25 {
				t.Errorf("%s: %d of %d entities lie within three relationships of %s (%.0f%%); the walk would reach a quarter of the graph or more", q.ID, within, len(entities), seed, 100*share)
			}
		}
	}
	for _, k := range []string{GraphQueryDirect, GraphQueryNeighbour, GraphQueryHop2, GraphQueryHop3, GraphQueryControl} {
		if kinds[k] < 6 {
			t.Errorf("the large set needs at least six %s questions, has %d", k, kinds[k])
		}
	}
	if len(entities) < 80 {
		t.Errorf("the large corpus should hold at least 80 entities, has %d", len(entities))
	}
}
