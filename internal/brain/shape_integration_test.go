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
	return shapeFixtureEdges(t, e, "uses_transport", "http-first transport", sources...)
}

// shapeFixtureEdges is the same shape with the relation type and target spelled
// out, so a test can build two clusters over the same sources.
func shapeFixtureEdges(t *testing.T, e *testEnv, relType, target string, sources ...string) []int64 {
	t.Helper()

	var ids []int64
	for _, source := range sources {
		factID := e.insertFact(t, source+" reaches "+target+" via "+relType, nil)
		if _, err := e.b.pool.Exec(e.ctx,
			`INSERT INTO relationships (namespace_id, from_entity, relation_type, to_entity, confidence, source_fact_id)
			 VALUES ($1, $2, $3, $4, 0.8, $5)`,
			e.nsID, source, relType, target, factID); err != nil {
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

// The same regularity reaches the sweep through two clusters — `uses` and
// `uses_transport` over the same eleven servers — and the model words it
// differently each time. That is one pattern with more evidence, not two rows.
func TestIntegrationShapeSweepMergesAnotherWordingOfTheSameEvidence(t *testing.T) {
	e := newEnv(t, nil)
	shapeFixtureEdges(t, e, "uses", "http-first transport", "alpha", "beta", "gamma")
	shapeFixtureEdges(t, e, "uses_transport", "http-first transport", "alpha", "beta", "gamma")

	e.r.shapes = []*reasoner.ShapeStatement{
		{Statement: "Services use an HTTP-first transport.", Coherence: 1.0},
		{Statement: "Every client reaches its server over the http-first transport.", Coherence: 1.0},
	}

	count, llmCalls, errs := e.b.consolidateToShapePatterns(e.ctx, e.nsID)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if count != 2 || llmCalls != 1 {
		t.Fatalf("want both clusters judged in one call, got %d patterns from %d calls", count, llmCalls)
	}

	var rows int
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT count(*) FROM patterns WHERE namespace_id = $1 AND deleted_at IS NULL`, e.nsID,
	).Scan(&rows); err != nil {
		t.Fatalf("count patterns: %v", err)
	}
	if rows != 1 {
		t.Fatalf("one regularity in two wordings must be one row, got %d", rows)
	}

	var relIDs []int64
	var entities []string
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT source_rel_ids, source_entities
		   FROM patterns WHERE namespace_id = $1 AND deleted_at IS NULL`, e.nsID,
	).Scan(&relIDs, &entities); err != nil {
		t.Fatalf("read merged pattern: %v", err)
	}
	if len(relIDs) != 6 {
		t.Errorf("want the union of both clusters as evidence (6 edges), got %v", relIDs)
	}
	if len(entities) != 3 {
		t.Errorf("want the three sources once each, got %v", entities)
	}
}

// The same four suppliers both chase documents and categorise receipts. Same
// sources, different statement: these stay apart, which is why sources alone can
// never be the merge key.
func TestIntegrationShapeSweepKeepsTwoRegularitiesOverTheSameSources(t *testing.T) {
	e := newEnv(t, nil)
	shapeFixtureEdges(t, e, "handles", "document chasing", "nmbrs", "yuki", "blinqx")
	shapeFixtureEdges(t, e, "handles", "receipt categorization", "nmbrs", "yuki", "blinqx")

	e.r.shapes = []*reasoner.ShapeStatement{
		{Statement: "Nmbrs, Yuki and Blinqx handle document chasing.", Coherence: 1.0},
		{Statement: "Nmbrs, Yuki and Blinqx handle receipt categorization.", Coherence: 1.0},
	}

	count, _, errs := e.b.consolidateToShapePatterns(e.ctx, e.nsID)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if count != 2 {
		t.Fatalf("want two patterns, got %d", count)
	}

	var rows int
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT count(*) FROM patterns WHERE namespace_id = $1 AND deleted_at IS NULL`, e.nsID,
	).Scan(&rows); err != nil {
		t.Fatalf("count patterns: %v", err)
	}
	if rows != 2 {
		t.Fatalf("two different regularities over the same sources must stay two rows, got %d", rows)
	}
}

// A second opinion must not undo an accepted verdict. The scheduler tick and a
// manual run judge the same clusters at the same time, and production showed the
// later rejection overwriting the stored pattern's cursor: a pattern row nobody
// would ask about again, while the cluster claimed it had been refused.
func TestIntegrationShapeSweepNeverDowngradesAnAcceptedVerdict(t *testing.T) {
	e := newEnv(t, nil)
	shapeFixture(t, e, "alpha", "beta", "gamma")

	if count, _, errs := e.b.consolidateToShapePatterns(e.ctx, e.nsID); count != 1 || len(errs) != 0 {
		t.Fatalf("first sweep: %d patterns, errors %v", count, errs)
	}

	var verdict string
	var wantSeen int
	var wantMax int64
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT verdict, sources_seen, max_rel_id FROM pattern_clusters WHERE namespace_id = $1`, e.nsID,
	).Scan(&verdict, &wantSeen, &wantMax); err != nil {
		t.Fatalf("read cluster cursor: %v", err)
	}
	if verdict != "pattern" {
		t.Fatalf("first sweep did not accept the cluster: %q", verdict)
	}

	// The competing run read the cluster before the pattern existed and records
	// its rejection with the cursor it saw then.
	if err := e.b.recordShapeCluster(e.ctx, e.nsID, &shapeCluster{
		relationType: "uses_transport",
		target:       "http-first transport",
		sources:      []string{"alpha", "beta"},
		maxRelID:     1,
	}, "rejected"); err != nil {
		t.Fatalf("record stale rejection: %v", err)
	}

	var gotSeen int
	var gotMax int64
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT verdict, sources_seen, max_rel_id FROM pattern_clusters WHERE namespace_id = $1`, e.nsID,
	).Scan(&verdict, &gotSeen, &gotMax); err != nil {
		t.Fatalf("read cluster cursor: %v", err)
	}
	if verdict != "pattern" {
		t.Errorf("an accepted cluster lost its verdict: %q", verdict)
	}
	if gotSeen != wantSeen {
		t.Errorf("sources_seen moved backwards from %d to %d", wantSeen, gotSeen)
	}
	if gotMax != wantMax {
		t.Errorf("max_rel_id moved backwards from %d to %d", wantMax, gotMax)
	}
}
