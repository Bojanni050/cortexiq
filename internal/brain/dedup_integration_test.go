package brain

import (
	"context"
	"testing"
)

// simBetween returns the cosine similarity the fake embedder gives two texts.
func simBetween(a, b string) float64 {
	va, _ := fakeEmbedder{}.Embed(context.Background(), a)
	vb, _ := fakeEmbedder{}.Embed(context.Background(), b)
	var dot float64
	for i := range va {
		dot += float64(va[i] * vb[i])
	}
	return dot
}

const (
	dedupTextA = "alpha beta gamma delta epsilon zeta eta theta"
	dedupTextB = "alpha beta gamma delta epsilon zeta eta theta iota"
)

// A borderline-similar fact is decided by the LLM: "same" reinforces the existing
// fact, "not same" stores a new one.
func TestIntegrationBorderlineDuplicateIsReviewedByLLM(t *testing.T) {
	sim := simBetween(dedupTextA, dedupTextB)
	for _, same := range []bool{true, false} {
		e := newEnv(t, func(c *Config) {
			c.DedupThreshold = sim + 0.02      // B is NOT an automatic duplicate of A...
			c.DedupReviewThreshold = sim - 0.1 // ...but falls in the review band
		})
		e.r.sameAnswer = same

		if _, err := e.b.Remember(e.ctx, e.ns, dedupTextA, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := e.b.ConsolidateByID(e.ctx, e.nsID); err != nil {
			t.Fatal(err)
		}
		if _, err := e.b.Remember(e.ctx, e.ns, dedupTextB, nil); err != nil {
			t.Fatal(err)
		}
		res, err := e.b.ConsolidateByID(e.ctx, e.nsID)
		if err != nil {
			t.Fatal(err)
		}

		if e.r.sameCalls != 1 {
			t.Fatalf("same=%v: ReasonSameFact calls = %d, want 1", same, e.r.sameCalls)
		}
		if same && (res.FactsDeduplicated != 1 || res.FactsCreated != 0) {
			t.Errorf("LLM said same: want 1 deduplicated, got %+v", res)
		}
		if !same && (res.FactsDeduplicated != 0 || res.FactsCreated != 1) {
			t.Errorf("LLM said different: want 1 created, got %+v", res)
		}
	}
}

// Clearly different facts never reach the LLM.
func TestIntegrationDissimilarFactSkipsReview(t *testing.T) {
	e := newEnv(t, nil)
	if _, err := e.b.Remember(e.ctx, e.ns, dedupTextA, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.Remember(e.ctx, e.ns, "completely unrelated kubernetes cluster upgrade", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.ConsolidateByID(e.ctx, e.nsID); err != nil {
		t.Fatal(err)
	}
	if e.r.sameCalls != 0 {
		t.Errorf("ReasonSameFact was called %d times for dissimilar facts", e.r.sameCalls)
	}
}

// The namespace name and description reach the fact synthesis, so facts can name
// their project.
func TestIntegrationSynthesisReceivesNamespaceSubject(t *testing.T) {
	e := newEnv(t, nil)
	if _, err := e.b.CreateNamespace(e.ctx, e.ns, "Zebra Project", "Tracks zebra migration"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.Remember(e.ctx, e.ns, "it now supports the new route", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.ConsolidateByID(e.ctx, e.nsID); err != nil {
		t.Fatal(err)
	}
	if len(e.r.subjects) != 1 || e.r.subjects[0] != "Zebra Project — Tracks zebra migration" {
		t.Errorf("subjects = %q", e.r.subjects)
	}
}

func TestIntegrationDedupeFactsMergesAndIsDryRunByDefault(t *testing.T) {
	e := newEnv(t, nil)
	a := e.insertFact(t, dedupTextA, nil)
	dup := e.insertFact(t, dedupTextA, nil) // identical text: similarity 1
	other := e.insertFact(t, "completely unrelated kubernetes cluster upgrade", nil)

	// Give the duplicate a source so the merge has something to move.
	epID, err := e.b.Remember(e.ctx, e.ns, dedupTextA, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.pool.Exec(e.ctx, "INSERT INTO fact_sources (fact_id, episode_id) VALUES ($1, $2)", dup, epID); err != nil {
		t.Fatal(err)
	}

	opts := DedupeOptions{Namespaces: []string{e.ns}, Threshold: 0.95}
	dry, err := e.b.DedupeFacts(e.ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if dry.Applied || len(dry.Groups) != 1 || dry.FactsMerged != 1 {
		t.Fatalf("dry run = %+v", dry)
	}
	var live int
	_ = e.b.pool.QueryRow(e.ctx, "SELECT count(*) FROM facts WHERE namespace_id = $1 AND deleted_at IS NULL", e.nsID).Scan(&live)
	if live != 3 {
		t.Fatalf("dry run changed data: %d live facts, want 3", live)
	}

	opts.Apply = true
	res, err := e.b.DedupeFacts(e.ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	// The duplicate has a source and the original none, so the duplicate survives.
	if !res.Applied || len(res.Groups) != 1 || res.Groups[0].Keep != dup || len(res.Groups[0].Merge) != 1 || res.Groups[0].Merge[0] != a {
		t.Fatalf("apply = %+v (a=%d dup=%d)", res, a, dup)
	}
	for id, wantDeleted := range map[int64]bool{a: true, dup: false, other: false} {
		var deleted bool
		if err := e.b.pool.QueryRow(e.ctx, "SELECT deleted_at IS NOT NULL FROM facts WHERE id = $1", id).Scan(&deleted); err != nil || deleted != wantDeleted {
			t.Errorf("fact %d deleted=%v (err %v), want %v", id, deleted, err, wantDeleted)
		}
	}

	again, err := e.b.DedupeFacts(e.ctx, opts)
	if err != nil || again.FactsMerged != 0 {
		t.Errorf("second run must find nothing: %+v err=%v", again, err)
	}
}
