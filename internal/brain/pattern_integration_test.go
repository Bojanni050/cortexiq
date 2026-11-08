// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"errors"
	"testing"
)

// Patterns were write-only for months: derived, stored, and never read by
// anything. Making them readable also makes them deletable, because some of them
// are fact paraphrases rather than regularities and a reader has to be able to
// throw those away.
// The same regularity derived twice is one row, not two. A consolidation run that
// failed after this stage keeps its checkpoint, so the next tick reads the same
// facts and derives the same sentence again — that is how /projects/infra reached
// 91% duplicates. Seeing it a second time is better evidence, so the row is merged
// instead of copied.
func TestIntegrationStorePatternMergesTheSameRegularity(t *testing.T) {
	e := newEnv(t, nil)

	inserted, err := e.b.storePattern(e.ctx, e.nsID,
		"The deploy pipeline runs migrations before the container starts", 0.4,
		[]int64{1, 2}, []int64{7}, 0.8)
	if err != nil {
		t.Fatalf("storePattern: %v", err)
	}
	if !inserted {
		t.Error("the first write was reported as a repeat")
	}

	// Same sentence: different spacing and case, and one more source of each kind.
	inserted, err = e.b.storePattern(e.ctx, e.nsID,
		"  the deploy pipeline   RUNS migrations before the container starts ", 0.6,
		[]int64{2, 3}, []int64{8}, 0.9)
	if err != nil {
		t.Fatalf("storePattern repeat: %v", err)
	}
	if inserted {
		t.Fatal("a repeated regularity was inserted as a second row")
	}

	var confidence, coherence float32
	var factIDs, relIDs []int64
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT confidence, coherence_score, source_fact_ids, source_rel_ids
		 FROM patterns WHERE namespace_id = $1 AND deleted_at IS NULL`, e.nsID,
	).Scan(&confidence, &coherence, &factIDs, &relIDs); err != nil {
		t.Fatalf("read merged pattern: %v", err)
	}
	if confidence != 0.6 {
		t.Errorf("confidence = %v, want the strongest 0.6", confidence)
	}
	if coherence != 0.9 {
		t.Errorf("coherence = %v, want the strongest 0.9", coherence)
	}
	if len(factIDs) != 3 || len(relIDs) != 2 {
		t.Errorf("sources = %v facts / %v rels, want the union (3 facts, 2 relations)", factIDs, relIDs)
	}
	if n, err := e.b.CountPatterns(e.ctx, []string{e.ns}); err != nil || n != 1 {
		t.Errorf("CountPatterns = %d (err %v), want 1", n, err)
	}

	// A different regularity is a different row.
	inserted, err = e.b.storePattern(e.ctx, e.nsID,
		"nginx binds to the Tailscale address for hosted services", 0.5,
		[]int64{4, 5}, []int64{9}, 0.7)
	if err != nil {
		t.Fatalf("storePattern new content: %v", err)
	}
	if !inserted {
		t.Error("a new regularity was reported as a repeat")
	}
	if n, err := e.b.CountPatterns(e.ctx, []string{e.ns}); err != nil || n != 2 {
		t.Errorf("CountPatterns = %d (err %v), want 2", n, err)
	}
}

// Patterns were write-only for months: derived, stored, and never read by
// anything. Making them readable also makes them deletable, because some of them
// are fact paraphrases rather than regularities and a reader has to be able to
// throw those away.
func TestIntegrationPatternsListAndDelete(t *testing.T) {
	e := newEnv(t, nil)

	insert := func(content string) int64 {
		var id int64
		if err := e.b.pool.QueryRow(e.ctx,
			`INSERT INTO patterns (namespace_id, content, confidence, source_fact_ids, source_rel_ids, coherence_score)
			 VALUES ($1, $2, 0.53, '{1,2}'::bigint[], '{}'::bigint[], 1.0) RETURNING id`,
			e.nsID, content,
		).Scan(&id); err != nil {
			t.Fatalf("insert pattern: %v", err)
		}
		return id
	}
	keep := insert("a regularity that spans several facts")
	drop := insert("a single fact, restated")

	items, err := e.b.QueryPatterns(e.ctx, []string{e.ns}, Pagination{Limit: 10})
	if err != nil {
		t.Fatalf("QueryPatterns: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("patterns = %d, want 2", len(items))
	}
	if n, err := e.b.CountPatterns(e.ctx, []string{e.ns}); err != nil || n != 2 {
		t.Errorf("CountPatterns = %d (err %v), want 2", n, err)
	}

	if err := e.b.DeletePattern(e.ctx, drop); err != nil {
		t.Fatalf("DeletePattern: %v", err)
	}

	items, err = e.b.QueryPatterns(e.ctx, []string{e.ns}, Pagination{Limit: 10})
	if err != nil {
		t.Fatalf("QueryPatterns after delete: %v", err)
	}
	if len(items) != 1 || items[0].ID != keep {
		t.Fatalf("patterns after delete = %+v, want only #%d", items, keep)
	}
	if n, err := e.b.CountPatterns(e.ctx, []string{e.ns}); err != nil || n != 1 {
		t.Errorf("CountPatterns after delete = %d (err %v), want 1", n, err)
	}

	// Deleting twice is an unknown id, not a silent success: the second call had
	// nothing to do and must say so.
	if err := e.b.DeletePattern(e.ctx, drop); !errors.Is(err, ErrPatternNotFound) {
		t.Errorf("second delete error = %v, want ErrPatternNotFound", err)
	}
	if err := e.b.DeletePattern(e.ctx, 0); !errors.Is(err, ErrPatternNotFound) {
		t.Errorf("delete of id 0 error = %v, want ErrPatternNotFound", err)
	}
}
