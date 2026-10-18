package queries

import (
	"strconv"
	"time"

	"github.com/pgvector/pgvector-go"
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
		"facts": func(ns []int64) (string, []any, error) { return q.RecallFactsKeyword(ns, "cortexiq or port", 30, nil) },
		"episodes": func(ns []int64) (string, []any, error) {
			return q.RecallEpisodesKeyword(ns, "cortexiq or port", 30, nil)
		},
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

// As-of mode swaps "valid now" for a point-in-time window on facts and a
// "stored by then" bound on episodes; placeholders and args must stay in step.
func TestRecallAsOfTemplatesRender(t *testing.T) {
	q, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	asOf := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	vec := pgvector.NewVector([]float32{1, 0})

	sql, args, err := q.RecallFacts([]int64{7}, vec, 10, &asOf)
	if err != nil {
		t.Fatal(err)
	}
	// vector x2, namespace, as_of x2, limit
	if len(args) != 6 {
		t.Errorf("facts as-of: got %d args, want 6\n%s", len(args), sql)
	}
	if strings.Contains(sql, "now()") || !strings.Contains(sql, "COALESCE(valid_from, created_at) <=") {
		t.Errorf("facts as-of must not use now():\n%s", sql)
	}

	sql, _, err = q.RecallFacts([]int64{7}, vec, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "valid_until > now()") || strings.Contains(sql, "COALESCE(valid_from") {
		t.Errorf("facts without as-of keep the valid-now filter:\n%s", sql)
	}

	sql, args, err = q.RecallEpisodesKeyword(nil, "port", 5, &asOf)
	if err != nil {
		t.Fatal(err)
	}
	// query x2, as_of, limit
	if len(args) != 4 || !strings.Contains(sql, "created_at <=") {
		t.Errorf("episodes as-of: args=%d sql=\n%s", len(args), sql)
	}
}

// The graph templates share one entity-matching template; placeholders and
// args must stay in step with and without namespaces and a point in time.
func TestRecallGraphTemplatesRender(t *testing.T) {
	q, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	asOf := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	sql, args, err := q.RecallFactsGraph([]int64{1, 2}, "welke poort draait cortexiq", pgvector.NewVector([]float32{1, 0}), 30, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "WITH matched AS") || !strings.Contains(sql, "neighbours") {
		t.Errorf("unexpected SQL:\n%s", sql)
	}
	// Every $N must have an argument and every argument a $N.
	for i := range args {
		if !strings.Contains(sql, "$"+itoa(i+1)) {
			t.Errorf("argument %d is never used:\n%s", i+1, sql)
		}
	}
	if strings.Contains(sql, "$"+itoa(len(args)+1)) {
		t.Errorf("placeholder beyond the %d arguments:\n%s", len(args), sql)
	}

	withAsOf, argsAsOf, err := q.RecallFactsGraph(nil, "cortexiq", pgvector.NewVector([]float32{1, 0}), 10, &asOf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(withAsOf, "COALESCE(f.valid_from, f.created_at)") || strings.Contains(withAsOf, "namespace_id IN") {
		t.Errorf("as-of without namespaces rendered wrongly:\n%s", withAsOf)
	}
	if len(argsAsOf) == 0 {
		t.Error("no arguments bound")
	}

	sql, args, err = q.RecallMatchedEntities([]int64{3}, "pgvector")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "ORDER BY name") || len(args) == 0 {
		t.Errorf("matched entities rendered wrongly (%d args):\n%s", len(args), sql)
	}
}

func itoa(i int) string { return strconv.Itoa(i) }
