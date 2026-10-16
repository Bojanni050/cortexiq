package brain

import (
	"os"
	"strings"
	"testing"
)

func TestLoadDedupPairsRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"unknown label": `{"id":"x","label":"similar","a":"a","b":"b"}`,
		"missing text":  `{"id":"x","label":"same","a":"a","b":""}`,
		"missing id":    `{"label":"same","a":"a","b":"b"}`,
		"duplicate id":  "{\"id\":\"x\",\"label\":\"same\",\"a\":\"a\",\"b\":\"b\"}\n{\"id\":\"x\",\"label\":\"same\",\"a\":\"a\",\"b\":\"b\"}",
		"not json":      `nope`,
		"empty":         "\n\n",
	}
	for name, in := range cases {
		if _, err := LoadDedupPairs(strings.NewReader(in)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// The shipped set must stay loadable and balanced enough to calibrate on.
func TestShippedDedupPairsLoad(t *testing.T) {
	f, err := os.Open("../../eval/dedup_pairs.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	pairs, err := LoadDedupPairs(f)
	if err != nil {
		t.Fatal(err)
	}
	n := map[string]int{}
	for _, p := range pairs {
		n[p.Label]++
	}
	if len(pairs) < 50 || n[PairSame] < 15 || n[PairNearMiss] < 15 || n[PairDifferent] < 10 {
		t.Fatalf("set too small to calibrate on: %d pairs, %v", len(pairs), n)
	}
}

func TestScoreDedupPairs(t *testing.T) {
	pairs := []DedupPair{
		{ID: "s1", Label: PairSame},      // auto-merged
		{ID: "s2", Label: PairSame},      // reviewed
		{ID: "s3", Label: PairSame},      // missed
		{ID: "n1", Label: PairNearMiss},  // false merge
		{ID: "n2", Label: PairNearMiss},  // reviewed
		{ID: "d1", Label: PairDifferent}, // new
	}
	sims := []float32{0.95, 0.82, 0.60, 0.90, 0.81, 0.30}
	rep := ScoreDedupPairs(pairs, sims, 0.85, 0.80)

	c := rep.Current
	if c.AutoMergedSame != 1 || c.AutoMergedOther != 1 {
		t.Errorf("auto-merged = same %d other %d, want 1 and 1", c.AutoMergedSame, c.AutoMergedOther)
	}
	if c.ReviewedSame != 1 || c.ReviewedOther != 1 {
		t.Errorf("reviewed = same %d other %d, want 1 and 1", c.ReviewedSame, c.ReviewedOther)
	}
	if c.MissedSame != 1 {
		t.Errorf("missed = %d, want 1", c.MissedSame)
	}
	if len(rep.FalseMerges) != 1 || rep.FalseMerges[0].ID != "n1" {
		t.Errorf("false merges = %+v, want n1", rep.FalseMerges)
	}
	if len(rep.Missed) != 1 || rep.Missed[0].ID != "s3" {
		t.Errorf("missed = %+v, want s3", rep.Missed)
	}

	st := rep.Labels[PairSame]
	if st.Count != 3 || st.Min != 0.60 || st.Median != 0.82 || st.Max != 0.95 {
		t.Errorf("same stats = %+v", st)
	}

	for _, row := range rep.Sweep {
		if row.Threshold == 0.80 && (row.Same != 2 || row.NearMiss != 2 || row.Different != 0) {
			t.Errorf("sweep at 0.80 = %+v", row)
		}
	}
}
