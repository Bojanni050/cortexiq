package brain

import "testing"

func TestContradictionReviewDue(t *testing.T) {
	for _, c := range []struct {
		sim, threshold float64
		want           bool
	}{
		{0.95, 0, false},     // off
		{0.95, -1, false},    // off
		{0.857, 0.85, true},  // at the dedup review line
		{0.85, 0.85, true},   // the line itself is reviewed
		{0.849, 0.85, false}, // clearly a different statement
		{0, 0.85, false},     // no embedding
	} {
		if got := contradictionReviewDue(c.sim, c.threshold); got != c.want {
			t.Errorf("contradictionReviewDue(%v, %v) = %v, want %v", c.sim, c.threshold, got, c.want)
		}
	}
}
