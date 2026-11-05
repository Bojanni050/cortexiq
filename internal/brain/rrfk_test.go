package brain

import "testing"

// The fusion constant is tunable, and its default is the paper's 60.
func TestRRFKDefaultAndOverride(t *testing.T) {
	if got := rrfK(); got != 60 {
		t.Fatalf("default rrfK = %d, want 60", got)
	}
	t.Cleanup(func() { SetRRFK(0) })

	SetRRFK(10)
	if got := rrfK(); got != 10 {
		t.Fatalf("rrfK after SetRRFK(10) = %d", got)
	}
	// A rank-1 hit in a single arm scores 1/(k+1); a smaller k widens the gap
	// between the top rank and the rest.
	top := fuseArms([]rankedArm{{"vector", rr(1, 2, 3)}}, 1, 3, 0)
	if len(top) != 3 || top[0].Score <= top[2].Score {
		t.Fatalf("scores = %+v; want the top rank ahead", top)
	}
	small := top[0].Score - top[2].Score

	SetRRFK(0)
	if got := rrfK(); got != 60 {
		t.Fatalf("SetRRFK(0) = %d, want the default back", got)
	}
	big := fuseArms([]rankedArm{{"vector", rr(1, 2, 3)}}, 1, 3, 0)
	if gap := big[0].Score - big[2].Score; gap >= small {
		t.Fatalf("gap with k=60 (%v) should be narrower than with k=10 (%v)", gap, small)
	}
}
