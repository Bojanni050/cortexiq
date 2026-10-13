package queries

import (
	"strings"
	"testing"
)

// The keyword templates bind the query twice (rank + filter) and a variable
// number of namespace IDs; make sure placeholders and args stay in step.
func TestRecallKeywordTemplatesRender(t *testing.T) {
	q, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for name, build := range map[string]func([]int64) (string, []any, error){
		"facts":    func(ns []int64) (string, []any, error) { return q.RecallFactsKeyword(ns, "stash or port", 30) },
		"episodes": func(ns []int64) (string, []any, error) { return q.RecallEpisodesKeyword(ns, "stash or port", 30) },
	} {
		sql, args, err := build([]int64{1, 2, 3})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// query x2, 3 namespace ids, limit
		if len(args) != 6 {
			t.Errorf("%s: got %d args, want 6", name, len(args))
		}
		if !strings.Contains(sql, "websearch_to_tsquery('simple', $1)") || !strings.Contains(sql, "$6") {
			t.Errorf("%s: unexpected SQL:\n%s", name, sql)
		}

		// Without namespaces: query x2 + limit, and no IN clause.
		sql, args, err = build(nil)
		if err != nil {
			t.Fatalf("%s (no ns): %v", name, err)
		}
		if len(args) != 3 || strings.Contains(sql, "namespace_id IN") {
			t.Errorf("%s (no ns): args=%d sql=\n%s", name, len(args), sql)
		}
	}
}
