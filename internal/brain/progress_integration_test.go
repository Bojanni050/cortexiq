// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"testing"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
)

// The stall signal is only trustworthy if a checkpoint that did not move does not
// get a fresh timestamp. This covers the SQL that records it: unchanged stages
// stay untouched, changed ones are stamped, and a second save with the same values
// changes nothing.
func TestIntegrationSaveProgressMarksOnlyChangedStages(t *testing.T) {
	e := newEnv(t, nil)

	// Every run calls GetOrCreate first, so the row exists before the checkpoint is
	// written; without it SaveConsolidationProgress has nothing to update.
	if _, err := e.b.GetOrCreateConsolidationProgress(e.ctx, e.nsID); err != nil {
		t.Fatalf("GetOrCreateConsolidationProgress: %v", err)
	}

	cp := models.ConsolidationProgress{NamespaceID: e.nsID, LastFactID: 5}
	if err := e.b.SaveConsolidationProgress(e.ctx, cp); err != nil {
		t.Fatalf("SaveConsolidationProgress: %v", err)
	}

	stamps := func() map[string]string {
		t.Helper()
		var raw map[string]string
		if err := e.b.pool.QueryRow(e.ctx,
			`SELECT stage_changed_at FROM consolidation_progress WHERE namespace_id = $1`, e.nsID,
		).Scan(&raw); err != nil {
			t.Fatalf("read stage_changed_at: %v", err)
		}
		return raw
	}

	first := stamps()
	if _, ok := first["relationships"]; !ok {
		t.Fatalf("a moved checkpoint was not stamped: %v", first)
	}
	if _, ok := first["patterns"]; ok {
		t.Errorf("an unchanged stage was stamped: %v", first)
	}

	// Same values again: nothing moved, so nothing may be restamped — otherwise a
	// stall would look like progress on every run.
	time.Sleep(5 * time.Millisecond)
	if err := e.b.SaveConsolidationProgress(e.ctx, cp); err != nil {
		t.Fatalf("second SaveConsolidationProgress: %v", err)
	}
	if second := stamps(); second["relationships"] != first["relationships"] {
		t.Errorf("an unchanged checkpoint was restamped: %q -> %q", first["relationships"], second["relationships"])
	}

	cp.LastPatternFactID = 9
	if err := e.b.SaveConsolidationProgress(e.ctx, cp); err != nil {
		t.Fatalf("third SaveConsolidationProgress: %v", err)
	}
	third := stamps()
	if _, ok := third["patterns"]; !ok {
		t.Errorf("a newly moved stage was not stamped: %v", third)
	}
	if third["relationships"] != first["relationships"] {
		t.Errorf("the untouched stage moved: %q -> %q", first["relationships"], third["relationships"])
	}
}

// Work waiting behind a frozen checkpoint is a stall; the same checkpoint with
// nothing behind it is just an idle namespace, and calling that a stall would make
// the warning meaningless.
func TestIntegrationStageStallsNeedWaitingWork(t *testing.T) {
	e := newEnv(t, nil)

	fact := e.insertFact(t, "a fact nobody has looked at yet", nil)
	if _, err := e.b.GetOrCreateConsolidationProgress(e.ctx, e.nsID); err != nil {
		t.Fatalf("GetOrCreateConsolidationProgress: %v", err)
	}
	cp := models.ConsolidationProgress{NamespaceID: e.nsID}
	if err := e.b.SaveConsolidationProgress(e.ctx, cp); err != nil {
		t.Fatalf("SaveConsolidationProgress: %v", err)
	}

	// Nobody is stalled yet: the checkpoint is fresh.
	stalls, err := e.b.ConsolidationStageStalls(e.ctx, []string{e.ns})
	if err != nil {
		t.Fatalf("ConsolidationStageStalls: %v", err)
	}
	for _, s := range stalls {
		if s.Stalled {
			t.Errorf("%s reported stalled right after a run: %+v", s.Stage, s)
		}
	}

	// Backdate the relationship checkpoint and leave fact #N waiting behind it.
	if _, err := e.b.pool.Exec(e.ctx,
		`UPDATE consolidation_progress
		    SET stage_changed_at = jsonb_build_object('relationships', to_jsonb(now() - interval '6 hours')),
		        last_fact_id = $2
		  WHERE namespace_id = $1`, e.nsID, fact-1,
	); err != nil {
		t.Fatalf("backdate checkpoint: %v", err)
	}

	stalls, err = e.b.ConsolidationStageStalls(e.ctx, []string{e.ns})
	if err != nil {
		t.Fatalf("ConsolidationStageStalls: %v", err)
	}
	var found bool
	for _, s := range stalls {
		if s.Stage == "relationships" && s.Stalled {
			found = true
			if s.Pending < 1 {
				t.Errorf("stalled with pending %d, want at least 1", s.Pending)
			}
			if s.IdleHours < 5 {
				t.Errorf("idle_hours = %v, want about 6", s.IdleHours)
			}
		}
	}
	if !found {
		t.Errorf("a frozen checkpoint with work behind it was not reported: %+v", stalls)
	}
}
