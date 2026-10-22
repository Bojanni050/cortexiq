// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Bojanni050/cortexiq/internal/scope"
)

func TestFeedbackFactorIsBoundedAndNeutralAtZero(t *testing.T) {
	cases := []struct {
		net  int
		want float64
	}{
		{0, 1},
		{1, 1.25},
		{2, feedbackCeiling},
		{9, feedbackCeiling},
		{-1, 0.75},
		{-2, feedbackFloor},
		{-9, feedbackFloor},
	}
	for _, c := range cases {
		if got := feedbackFactor(c.net); got != c.want {
			t.Errorf("feedbackFactor(%d) = %v, want %v", c.net, got, c.want)
		}
	}
}

func TestRateFactRejectsBadInputBeforeTouchingTheDatabase(t *testing.T) {
	b := &Brain{} // no pool: validation must refuse first
	for _, verdict := range []int{0, 2, -3} {
		if _, err := b.RateFact(context.Background(), 1, verdict, ""); !errors.Is(err, ErrBadVerdict) {
			t.Errorf("verdict %d: err = %v, want ErrBadVerdict", verdict, err)
		}
	}
	if _, err := b.RateFact(context.Background(), 0, 1, ""); err == nil {
		t.Error("fact id 0 should be refused")
	}
}

func TestRateFactRefusesReadOnlyCaller(t *testing.T) {
	b := &Brain{}
	ctx := scope.With(context.Background(), scope.Principal{Name: "agent", ReadOnly: true})
	if _, err := b.RateFact(ctx, 1, 1, ""); !errors.Is(err, ErrReadOnly) {
		t.Errorf("err = %v, want ErrReadOnly", err)
	}
}

// A vote must move the list even when recency and importance weighting are both
// off, and it must not rewrite fusedScore: that is the score the fact earned.
func TestApplyBoostsLowersDownvotedFactWithoutRewritingFusedScore(t *testing.T) {
	cfg := DefaultConfig()
	if reranksOn(cfg) {
		t.Fatal("default config should not rerank; this test pins the feedback-only path")
	}
	rs := []RecallResult{
		{ID: 1, Type: "fact", Score: 0.9, Feedback: -1},
		{ID: 2, Type: "fact", Score: 0.7},
	}
	got := applyBoosts(rs, 10, time.Now(), cfg)
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 1 {
		t.Fatalf("order = %v, want the downvoted fact second", ids(got))
	}
	if got[1].fusedScore != 0.9 {
		t.Errorf("fusedScore = %v, want 0.9 (the pre-boost score)", got[1].fusedScore)
	}
	if got[1].Score != 0.9*float32(feedbackFactor(-1)) {
		t.Errorf("score = %v, want %v", got[1].Score, 0.9*feedbackFactor(-1))
	}
	if got[0].Score != 0.7 {
		t.Errorf("an unvoted fact must keep its score, got %v", got[0].Score)
	}
}

func TestAnyFeedbackSeesOnlyNonZeroVotes(t *testing.T) {
	if anyFeedback([]RecallResult{{ID: 1, Type: "fact"}}) {
		t.Error("no votes should not count as feedback")
	}
	if !anyFeedback([]RecallResult{{ID: 1}, {ID: 2, Feedback: -1}}) {
		t.Error("a downvote should force a rescale")
	}
}

func TestPrincipalNameFallsBackToLocal(t *testing.T) {
	if got := principalName(context.Background()); got != "local" {
		t.Errorf("principalName without a principal = %q, want local", got)
	}
	named := scope.With(context.Background(), scope.Principal{Name: "agent"})
	if got := principalName(named); got != "agent" {
		t.Errorf("principalName = %q, want agent", got)
	}
	unnamed := scope.With(context.Background(), scope.Principal{})
	if got := principalName(unnamed); got != "local" {
		t.Errorf("principalName of an unnamed principal = %q, want local", got)
	}
}
