// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"testing"

	"github.com/Bojanni050/cortexiq/internal/reasoner"
)

// shapeFixture inserts one fact per source and points them all at the same
// target with the same relation type: the shape the sweep looks for.
func shapeFixture(t *testing.T, e *testEnv, sources ...string) []int64 {
	t.Helper()

	var ids []int64
	for _, source := range sources {
		factID := e.insertFact(t, source+" reaches its server over the http-first transport", nil)
		if _, err := e.b.pool.Exec(e.ctx,
			`INSERT INTO relationships (namespace_id, from_entity, relation_type, to_entity, confidence, source_fact_id)
			 VALUES ($1, $2, 'uses_transport', 'http-first transport', 0.8, $3)`,
			e.nsID, source, factID); err != nil {
			t.Fatalf("insert relationship for %s: %v", source, err)
		}
		ids = append(ids, factID)
	}
	return ids
}

// A cluster of three sources is recurrence the window stage could never see.
// The sweep must store it and prove which edges it rests on.
func TestIntegrationShapeSweepStoresARegularity(t *testing.T) {
	e := newEnv(t, nil)
	shapeFixture(t, e, "alpha", "beta", "gamma")

	count, llmCalls, errs := e.b.consolidateToShapePatterns(e.ctx, e.nsID)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if count != 1 || llmCalls != 1 {
		t.Fatalf("want 1 pattern from 1 call, got %d patterns from %d calls", count, llmCalls)
	}

	var content string
	var relIDs, factIDs []int64
	var coherence, confidence float32
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT content, source_rel_ids, source_fact_ids, coherence_score, confidence
		   FROM patterns WHERE namespace_id = $1 AND deleted_at IS NULL`, e.nsID,
	).Scan(&content, &relIDs, &factIDs, &coherence, &confidence); err != nil {
		t.Fatalf("read stored pattern: %v", err)
	}

	if content != "every source shares the behaviour" {
		t.Errorf("unexpected statement %q", content)
	}
	if len(relIDs) != 3 {
		t.Errorf("want the three edges as evidence, got %v", relIDs)
	}
	if len(factIDs) != 3 {
		t.Errorf("want the three facts as evidence, got %v", factIDs)
	}
	if coherence != 1.0 {
		t.Errorf("want coherence 1.0, got %v", coherence)
	}
	// 0.8 (weakest edge) times 1.0.
	if confidence != 0.8 {
		t.Errorf("want confidence 0.8, got %v", confidence)
	}

	var verdict string
	var seen int
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT verdict, sources_seen FROM pattern_clusters WHERE namespace_id = $1`, e.nsID,
	).Scan(&verdict, &seen); err != nil {
		t.Fatalf("read cluster cursor: %v", err)
	}
	if verdict != "pattern" || seen != 3 {
		t.Errorf("want verdict pattern with 3 sources, got %q with %d", verdict, seen)
	}
}

// The sweep runs on every consolidation tick, so a judged cluster must leave the
// queue. Without this the same four candidates would cost an LLM call forever.
func TestIntegrationShapeSweepDoesNotAskTwice(t *testing.T) {
	e := newEnv(t, nil)
	shapeFixture(t, e, "alpha", "beta", "gamma")

	if count, _, errs := e.b.consolidateToShapePatterns(e.ctx, e.nsID); count != 1 || len(errs) != 0 {
		t.Fatalf("first sweep: %d patterns, errors %v", count, errs)
	}

	count, llmCalls, errs := e.b.consolidateToShapePatterns(e.ctx, e.nsID)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if count != 0 || llmCalls != 0 {
		t.Fatalf("second sweep should be free, got %d patterns from %d calls", count, llmCalls)
	}

	e.r.mu.Lock()
	calls := len(e.r.shapeCandidates)
	e.r.mu.Unlock()
	if calls != 1 {
		t.Errorf("want a single ReasonShapes call, got %d", calls)
	}
}

// A rejection is recorded, not retried — but a fourth source is new evidence, so
// the candidate comes back.
func TestIntegrationShapeSweepReasksWhenTheClusterGrows(t *testing.T) {
	e := newEnv(t, nil)
	shapeFixture(t, e, "alpha", "beta", "gamma")

	e.r.shapes = []*reasoner.ShapeStatement{{Coherence: 0}}

	count, _, errs := e.b.consolidateToShapePatterns(e.ctx, e.nsID)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if count != 0 {
		t.Fatalf("a rejected candidate must not store a pattern, got %d", count)
	}

	var verdict string
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT verdict FROM pattern_clusters WHERE namespace_id = $1`, e.nsID).Scan(&verdict); err != nil {
		t.Fatalf("read cluster cursor: %v", err)
	}
	if verdict != "rejected" {
		t.Errorf("want verdict rejected, got %q", verdict)
	}

	if count, llmCalls, _ := e.b.consolidateToShapePatterns(e.ctx, e.nsID); count != 0 || llmCalls != 0 {
		t.Fatalf("a rejected cluster should not be asked again unchanged, got %d calls", llmCalls)
	}

	shapeFixture(t, e, "delta")
	e.r.shapes = nil

	count, llmCalls, errs := e.b.consolidateToShapePatterns(e.ctx, e.nsID)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if count != 1 || llmCalls != 1 {
		t.Fatalf("want the grown cluster judged again, got %d patterns from %d calls", count, llmCalls)
	}

	var seen int
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT sources_seen FROM pattern_clusters WHERE namespace_id = $1`, e.nsID).Scan(&seen); err != nil {
		t.Fatalf("read cluster cursor: %v", err)
	}
	if seen != 4 {
		t.Errorf("want 4 sources recorded, got %d", seen)
	}
}

// Two sources that happen to share a target are a coincidence, not a habit.
func TestIntegrationShapeSweepIgnoresSmallClusters(t *testing.T) {
	e := newEnv(t, nil)
	shapeFixture(t, e, "alpha", "beta")

	count, llmCalls, errs := e.b.consolidateToShapePatterns(e.ctx, e.nsID)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if count != 0 || llmCalls != 0 {
		t.Fatalf("want no work for two sources, got %d patterns from %d calls", count, llmCalls)
	}
}
