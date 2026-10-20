// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"testing"

	"github.com/Bojanni050/cortexiq/internal/scope"
)

// A vote is a rank opinion, not evidence: it must move recall results and leave
// confidence, validity and the vote history alone. Each principal keeps one
// standing vote, so changing your mind replaces it instead of stacking.
func TestIntegrationFactFeedbackIsBoundedAndPerPrincipal(t *testing.T) {
	e := newEnv(t, nil)
	id := e.insertFact(t, "the queue is kafka", nil)

	a := scope.With(e.ctx, scope.Principal{Name: "agent-a"})
	other := scope.With(e.ctx, scope.Principal{Name: "agent-b"})

	fb, err := e.b.RateFact(a, id, 1, "helpful")
	if err != nil {
		t.Fatalf("RateFact: %v", err)
	}
	if fb.Net != 1 || fb.Factor != 1.25 || fb.Replaced {
		t.Fatalf("first vote = %+v, want net 1, factor 1.25, not replaced", fb)
	}

	fb, err = e.b.RateFact(a, id, -1, "changed my mind")
	if err != nil {
		t.Fatalf("RateFact replace: %v", err)
	}
	if !fb.Replaced || fb.Net != -1 || fb.Factor != 0.75 {
		t.Fatalf("replaced vote = %+v, want net -1, factor 0.75, replaced", fb)
	}

	fb, err = e.b.RateFact(other, id, -1, "wrong twice")
	if err != nil {
		t.Fatalf("RateFact other principal: %v", err)
	}
	if fb.Net != -2 || fb.Factor != feedbackFloor {
		t.Fatalf("two downvotes = %+v, want net -2 at the floor", fb)
	}

	fb, err = e.b.RateFact(other, id, 1, "turns out it was right")
	if err != nil {
		t.Fatalf("RateFact other principal replace: %v", err)
	}
	if fb.Net != 0 || fb.Factor != 1 {
		t.Fatalf("cancelling votes = %+v, want net 0 and factor 1", fb)
	}

	var votes int
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT count(*) FROM fact_feedback WHERE fact_id = $1`, id).Scan(&votes); err != nil {
		t.Fatal(err)
	}
	if votes != 4 {
		t.Errorf("stored votes = %d, want 4 (every vote is kept for audit)", votes)
	}

	var confidence float32
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT confidence FROM facts WHERE id = $1`, id).Scan(&confidence); err != nil {
		t.Fatal(err)
	}
	if confidence != 0.8 {
		t.Errorf("confidence = %v, want 0.8 — a vote must not touch evidence", confidence)
	}

	// Cancelled votes leave the fused score alone but are still reported.
	rs := []RecallResult{{ID: id, Type: "fact", Score: 0.5}}
	e.b.applyFeedback(e.ctx, rs)
	if rs[0].Feedback != 0 || rs[0].Score != 0.5 {
		t.Errorf("cancelled feedback = %+v, want no net and unchanged score", rs[0])
	}

	if _, err := e.b.RateFact(a, id, 1, "net -1"); err != nil {
		t.Fatalf("RateFact: %v", err)
	}
	// a now holds +1, other holds +1: net 2, capped at the ceiling.
	rs = []RecallResult{{ID: id, Type: "fact", Score: 0.5}}
	e.b.applyFeedback(e.ctx, rs)
	if rs[0].Feedback != 2 {
		t.Errorf("feedback = %d, want 2", rs[0].Feedback)
	}

	if _, err := e.b.RateFact(a, id+999999, 1, ""); err == nil {
		t.Error("voting on a fact that does not exist should fail")
	}
}
