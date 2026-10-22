package brain

import (
	"errors"
	"reflect"
	"testing"
)

func TestDecideDedup(t *testing.T) {
	const high = 0.90
	tests := []struct {
		name          string
		score, review float32
		want          dedupDecision
	}{
		{"above threshold is a duplicate", 0.95, 0.80, dedupDuplicate},
		{"exactly the threshold is a duplicate", 0.90, 0.80, dedupDuplicate},
		{"borderline goes to the LLM", 0.85, 0.80, dedupReview},
		{"exactly the review threshold goes to the LLM", 0.80, 0.80, dedupReview},
		{"below review is new", 0.79, 0.80, dedupNew},
		{"review disabled: borderline is new", 0.85, 0, dedupNew},
		{"review not below threshold: disabled", 0.85, 0.95, dedupNew},
	}
	for _, tt := range tests {
		if got := decideDedup(tt.score, high, tt.review); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPickSurvivor(t *testing.T) {
	ms := []dedupeMember{
		{id: 5, confidence: 0.9, sources: 1},
		{id: 3, confidence: 0.5, sources: 3}, // most sources wins
		{id: 9, confidence: 0.99, sources: 2},
	}
	if got := pickSurvivor(ms); got.id != 3 {
		t.Errorf("most sources should win, got %d", got.id)
	}
	ms = []dedupeMember{{id: 5, confidence: 0.5, sources: 1}, {id: 7, confidence: 0.8, sources: 1}}
	if got := pickSurvivor(ms); got.id != 7 {
		t.Errorf("then confidence, got %d", got.id)
	}
	ms = []dedupeMember{{id: 8, confidence: 0.5, sources: 1}, {id: 4, confidence: 0.5, sources: 1}}
	if got := pickSurvivor(ms); got.id != 4 {
		t.Errorf("then oldest, got %d", got.id)
	}
}

func TestUnionFindGroups(t *testing.T) {
	u := newUnionFind()
	u.union(1, 2)
	u.union(2, 3) // the 1-2-3 chain becomes one group
	u.union(10, 11)
	u.find(99) // a lone ID never forms a group
	got := u.groups()
	have := map[int64][]int64{}
	for _, g := range got {
		have[g[0]] = g
	}
	if len(got) != 2 || !reflect.DeepEqual(have[1], []int64{1, 2, 3}) || !reflect.DeepEqual(have[10], []int64{10, 11}) {
		t.Errorf("groups = %v", got)
	}
}

func TestReinforcedConfidence(t *testing.T) {
	if got := reinforcedConfidence(0.5, 0); got != 0.5 {
		t.Errorf("n=0 must not change confidence, got %v", got)
	}
	one, two := reinforcedConfidence(0.5, 1), reinforcedConfidence(0.5, 2)
	if one <= 0.5 || two <= one || two > 1 {
		t.Errorf("not monotonic or out of range: %v %v", one, two)
	}
}

func TestFormatSubject(t *testing.T) {
	tests := []struct{ slug, name, want string }{
		{"/projects/cortexiq", "CortexIQ", "CortexIQ"},
		{"/projects/cortexiq", "  CortexIQ (codebase)  ", "CortexIQ (codebase)"},
		{"/projects/cortexiq", "/projects/cortexiq", "cortexiq"}, // name is just the slug: use the last segment
		{"/projects/cortexiq", "", "cortexiq"},
		{"/clients/white-vision", "White Vision (klant)", "White Vision (klant)"},
		{"/", "/", ""},
	}
	for _, tt := range tests {
		if got := formatSubject(tt.slug, tt.name); got != tt.want {
			t.Errorf("formatSubject(%q,%q) = %q, want %q", tt.slug, tt.name, got, tt.want)
		}
	}
}

// A new fact is weighed against several neighbours, nearest first: the closest
// one is not always the twin, and a generic statement in front of the twin used
// to hide it.
func TestPickDedupTargetWalksCandidates(t *testing.T) {
	const high, review = 0.85, 0.80

	t.Run("clear duplicate merges without asking", func(t *testing.T) {
		asked := 0
		id, merged, calls, err := pickDedupTarget(
			[]nearestFact{{ID: 1, Score: 0.95}, {ID: 2, Score: 0.86}},
			high, review,
			func(nearestFact) (bool, error) { asked++; return true, nil },
		)
		if err != nil || !merged || id != 1 || calls != 0 || asked != 0 {
			t.Fatalf("got id=%d merged=%v calls=%d asked=%d err=%v", id, merged, calls, asked, err)
		}
	})

	t.Run("the twin behind an unrelated neighbour is found", func(t *testing.T) {
		// Rank 1 is borderline and the LLM says it is a different fact; rank 2
		// is the twin. Before, rank 2 was never looked at.
		var seen []int64
		id, merged, calls, err := pickDedupTarget(
			[]nearestFact{{ID: 1, Score: 0.82}, {ID: 2, Score: 0.81}},
			high, review,
			func(c nearestFact) (bool, error) { seen = append(seen, c.ID); return c.ID == 2, nil },
		)
		if err != nil || !merged || id != 2 || calls != 2 {
			t.Fatalf("got id=%d merged=%v calls=%d err=%v", id, merged, calls, err)
		}
		if !reflect.DeepEqual(seen, []int64{1, 2}) {
			t.Fatalf("asked about %v, want [1 2]", seen)
		}
	})

	t.Run("stops below the review band", func(t *testing.T) {
		asked := 0
		_, merged, calls, err := pickDedupTarget(
			[]nearestFact{{ID: 1, Score: 0.82}, {ID: 2, Score: 0.70}},
			high, review,
			func(nearestFact) (bool, error) { asked++; return false, nil },
		)
		if err != nil || merged || calls != 1 || asked != 1 {
			t.Fatalf("got merged=%v calls=%d asked=%d err=%v", merged, calls, asked, err)
		}
	})

	t.Run("review disabled never asks", func(t *testing.T) {
		asked := 0
		_, merged, calls, err := pickDedupTarget(
			[]nearestFact{{ID: 1, Score: 0.82}},
			high, 0,
			func(nearestFact) (bool, error) { asked++; return true, nil },
		)
		if err != nil || merged || calls != 0 || asked != 0 {
			t.Fatalf("got merged=%v calls=%d asked=%d err=%v", merged, calls, asked, err)
		}
	})

	t.Run("no candidates stores the fact", func(t *testing.T) {
		id, merged, calls, err := pickDedupTarget(nil, high, review, func(nearestFact) (bool, error) {
			t.Fatal("review must not be called without candidates")
			return false, nil
		})
		if err != nil || merged || id != 0 || calls != 0 {
			t.Fatalf("got id=%d merged=%v calls=%d err=%v", id, merged, calls, err)
		}
	})

	t.Run("a failing review stops the walk", func(t *testing.T) {
		want := errors.New("reasoner down")
		_, merged, calls, err := pickDedupTarget(
			[]nearestFact{{ID: 1, Score: 0.82}, {ID: 2, Score: 0.81}},
			high, review,
			func(nearestFact) (bool, error) { return false, want },
		)
		if !errors.Is(err, want) || merged || calls != 1 {
			t.Fatalf("got merged=%v calls=%d err=%v", merged, calls, err)
		}
	})
}

func TestDedupCandidatesFallsBackToDefault(t *testing.T) {
	if got := dedupCandidates(Config{}); got != defaultDedupCandidates {
		t.Errorf("unset config = %d, want %d", got, defaultDedupCandidates)
	}
	if got := dedupCandidates(Config{DedupCandidates: 3}); got != 3 {
		t.Errorf("configured = %d, want 3", got)
	}
}
