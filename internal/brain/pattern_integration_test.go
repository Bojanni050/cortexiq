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
