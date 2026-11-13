package brain

import (
	"math"
	"testing"
)

// Without weights the fusion is the plain one, score for score.
func TestFuseArmsWeightedNilWeightsEqualsPlain(t *testing.T) {
	arms := func() []rankedArm {
		return []rankedArm{{"vector", rr(1, 2, 3)}, {"keyword", rr(3, 1)}, {"graph", rr(2)}}
	}
	plain := fuseArms(arms(), 3, 10, 0)
	weighted := fuseArmsWeighted(arms(), nil, 3, 10, 0)
	if len(plain) != len(weighted) {
		t.Fatalf("len %d vs %d", len(plain), len(weighted))
	}
	for i := range plain {
		if plain[i].ID != weighted[i].ID || plain[i].Score != weighted[i].Score {
			t.Fatalf("row %d: plain %+v, weighted %+v", i, plain[i], weighted[i])
		}
	}
	// A weight of zero or less counts as 1, the same as no weight at all.
	zero := fuseArmsWeighted(arms(), map[string]float64{"graph": 0, "keyword": -1}, 3, 10, 0)
	for i := range plain {
		if plain[i].ID != zero[i].ID || plain[i].Score != zero[i].Score {
			t.Fatalf("zero weight row %d: plain %+v, got %+v", i, plain[i], zero[i])
		}
	}
}

// The scale follows the weights, so a result that ranks first in every arm still
// scores 1, and no score leaves 0..1 however the arms are weighted.
func TestFuseArmsWeightedKeepsScoresOnTheUnitScale(t *testing.T) {
	arms := []rankedArm{{"vector", rr(1)}, {"keyword", rr(1)}, {"graph", rr(1)}}
	got := fuseArmsWeighted(arms, map[string]float64{"graph": 0.4}, 3, 10, 0)
	if len(got) != 1 || math.Abs(float64(got[0].Score)-1) > 1e-6 {
		t.Fatalf("top of every arm scored %+v, want 1", got)
	}
	// Episodes have no graph arm but are scored on the graph-inclusive scale; the
	// missing arm counts as weight 1 there.
	eps := fuseArmsWeighted([]rankedArm{{"vector", rr(1)}, {"keyword", rr(1)}}, map[string]float64{"graph": 0.4}, 3, 10, 0)
	want := 2.0 / 3.0
	if len(eps) != 1 || math.Abs(float64(eps[0].Score)-want) > 1e-6 {
		t.Fatalf("episode scored %+v, want %v", eps, want)
	}
}

// The case the graph arm lost topical questions on: a fact that shares an entity
// with the question has a vector rank and a graph rank, so equal weights count it
// twice and it outranks a better answer. A lower graph weight puts them right.
func TestFuseArmsWeightedStopsTheGraphArmCountingTwice(t *testing.T) {
	const better, neighbour = 1, 2
	arms := func() []rankedArm {
		return []rankedArm{
			{"vector", rr(9, better, neighbour)},
			{"keyword", rr(7, 8, better)},
			{"graph", rr(neighbour)},
		}
	}
	if got := fuseArms(arms(), 3, 10, 0); got[0].ID != neighbour {
		t.Fatalf("equal weights: %v first, want the neighbour (%d) to outrank the better answer", ids(got), neighbour)
	}
	got := fuseArmsWeighted(arms(), map[string]float64{"graph": 0.4}, 3, 10, 0)
	if got[0].ID != better {
		t.Fatalf("graph weight 0.4: %v first, want the better answer (%d)", ids(got), better)
	}
}

func TestIsRelational(t *testing.T) {
	cases := []struct {
		query    string
		entities int
		want     bool
	}{
		{"hoe loopt de uitrol van cortexiq", 1, false},
		{"what port does the UI use", 0, false},
		{"wat is de relatie tussen pgvector en postgres", 2, true},
		{"how is pgvector related to postgres", 1, true},
		{"welke diensten hangt af van postgres", 1, true},
		{"what depends on pgvector", 1, true},
		{"vergelijk strato en hetzner", 2, true}, // two named entities, no cue
		{"the interbetween of things", 0, false}, // a cue is a whole word, not a substring
		{"", 0, false},
	}
	for _, c := range cases {
		if got := isRelational(c.query, c.entities); got != c.want {
			t.Errorf("isRelational(%q, %d) = %v, want %v", c.query, c.entities, got, c.want)
		}
	}
}

func TestGraphArmWeightDefaultsAndOverrides(t *testing.T) {
	var c Config
	if got := c.graphArmWeight(false); got != defaultGraphWeightTopic {
		t.Fatalf("topic default = %v, want %v", got, defaultGraphWeightTopic)
	}
	if got := c.graphArmWeight(true); got != defaultGraphWeightRelational {
		t.Fatalf("relational default = %v, want %v", got, defaultGraphWeightRelational)
	}
	c.GraphWeightTopic, c.GraphWeightRelational = 0.25, 0.9
	if c.graphArmWeight(false) != 0.25 || c.graphArmWeight(true) != 0.9 {
		t.Fatalf("overrides ignored: topic %v, relational %v", c.graphArmWeight(false), c.graphArmWeight(true))
	}
}
