package brain

import (
	"testing"
	"time"
)

// TestIntegrationConsolidationRunHistory round-trips a run through the database.
// A select/scan column mismatch (the bug this guards) fails here immediately.
func TestIntegrationConsolidationRunHistory(t *testing.T) {
	e := newEnv(t, nil)

	started := time.Now().UTC().Add(-time.Minute)
	res := ConsolidationResult{
		Namespace:          e.ns,
		Duration:           1500 * time.Millisecond,
		EpisodesRead:       7,
		FactsCreated:       3,
		RelationshipsFound: 2,
		LLMCalls:           9,
		Errors:             []string{"boom"},
	}
	if err := e.b.SaveConsolidationRun(e.ctx, e.nsID, res, started); err != nil {
		t.Fatalf("SaveConsolidationRun: %v", err)
	}

	runs, err := e.b.ListConsolidationRuns(e.ctx, []string{e.ns}, Pagination{Limit: 10})
	if err != nil {
		t.Fatalf("ListConsolidationRuns: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("expected at least one run")
	}
	got := runs[0]
	if got.NamespaceSlug != e.ns || got.FactsCreated != 3 || got.RelationshipsFound != 2 || got.LLMCalls != 9 {
		t.Fatalf("unexpected run values: %+v", got)
	}
	if got.DurationMS != 1500 {
		t.Fatalf("duration_ms = %d, want 1500", got.DurationMS)
	}
	if got.Errors != 1 || got.ErrorSample != "boom" {
		t.Fatalf("error fields not round-tripped: %+v", got)
	}
	if got.CreatedAt.IsZero() || got.StartedAt.IsZero() {
		t.Fatalf("timestamps should be set: %+v", got)
	}
}
