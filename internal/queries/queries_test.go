package queries

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pgvector/pgvector-go"
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
		"failures": func(ns []int64) (string, []any, error) {
			return q.RecallFailuresKeyword(ns, "cortexiq or port", 30, nil)
		},
		"goals": func(ns []int64) (string, []any, error) { return q.RecallGoalsKeyword(ns, "cortexiq or port", 30, nil) },
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
	if strings.Contains(sql, "now()") || !strings.Contains(sql, "known_until >") {
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
	if !strings.Contains(withAsOf, "f.known_until") || strings.Contains(withAsOf, "namespace_id IN") {
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

func TestRecallWindowTemplatesRender(t *testing.T) {
	q, err := New()
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	vec := pgvector.NewVector([]float32{1, 0})
	checks := map[string]func() (string, []any, error){
		"fact vector": func() (string, []any, error) {
			return q.RecallFactsWindow([]int64{7}, vec, 10, nil, &from, &to, Known{})
		},
		"fact keyword": func() (string, []any, error) {
			return q.RecallFactsKeywordWindow([]int64{7}, "project", 10, nil, &from, &to, Known{}, nil)
		},
		"fact graph": func() (string, []any, error) {
			return q.RecallFactsGraphWindow([]int64{7}, "project", vec, 10, nil, &from, &to, Known{}, nil)
		},
		"episode vector": func() (string, []any, error) {
			return q.RecallEpisodesWindow([]int64{7}, vec, 10, nil, &from, &to, Known{})
		},
		"episode keyword": func() (string, []any, error) {
			return q.RecallEpisodesKeywordWindow([]int64{7}, "project", 10, nil, &from, &to, Known{}, nil)
		},
		"fact temporal": func() (string, []any, error) {
			return q.RecallFactsTemporal([]int64{7}, 10, nil, &from, &to, Known{}, nil)
		},
		"episode temporal": func() (string, []any, error) {
			return q.RecallEpisodesTemporal([]int64{7}, 10, nil, &from, &to, Known{}, nil)
		},
	}
	for name, build := range checks {
		t.Run(name, func(t *testing.T) {
			sql, args, err := build()
			if err != nil {
				t.Fatal(err)
			}
			for i := range args {
				if !strings.Contains(sql, "$"+itoa(i+1)) {
					t.Errorf("argument %d is not referenced:\n%s", i+1, sql)
				}
			}
			if strings.Contains(sql, "$"+itoa(len(args)+1)) {
				t.Errorf("placeholder beyond %d arguments:\n%s", len(args), sql)
			}
			if name == "episode vector" || name == "episode keyword" || name == "episode temporal" {
				if !strings.Contains(sql, "occurred_at >=") || !strings.Contains(sql, "occurred_at <") {
					t.Errorf("episode window missing event-time filters:\n%s", sql)
				}
			} else if (!strings.Contains(sql, "valid_until IS NULL OR valid_until >") && !strings.Contains(sql, "f.valid_until IS NULL OR f.valid_until >")) || (!strings.Contains(sql, "COALESCE(valid_from, created_at) <") && !strings.Contains(sql, "COALESCE(f.valid_from, f.created_at) <")) {
				t.Errorf("fact window missing interval-overlap filters:\n%s", sql)
			}
		})
	}

	// An upper-only fact window must still exclude facts that start after `to`.
	sql, args, err := q.RecallFactsWindow(nil, vec, 10, nil, nil, &to, Known{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "COALESCE(valid_from, created_at) < $2") || !strings.Contains(sql, "valid_until IS NULL OR valid_until > '-infinity'::timestamptz") || len(args) != 4 {
		t.Errorf("upper-only fact window rendered wrongly: args=%v\n%s", args, sql)
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

// Every recall arm must carry the tag filter: the arms are fused by rank, so one
// unfiltered arm would put untagged rows back into the result.
func TestRecallTagFilterRendersInEveryArm(t *testing.T) {
	q, err := New()
	if err != nil {
		t.Fatal(err)
	}
	vec := pgvector.NewVector([]float32{1, 0})
	tags := []string{"beslissing", "pgvector"}
	builders := map[string]func() (string, []any, error){
		"fact vector": func() (string, []any, error) {
			return q.RecallFactsWindowWithEmbedding([]int64{7}, vec, 10, nil, nil, nil, Known{}, false, tags)
		},
		"fact keyword": func() (string, []any, error) {
			return q.RecallFactsKeywordWindow([]int64{7}, "project", 10, nil, nil, nil, Known{}, tags)
		},
		"fact graph": func() (string, []any, error) {
			return q.RecallFactsGraphWindow([]int64{7}, "project", vec, 10, nil, nil, nil, Known{}, tags)
		},
		"fact temporal": func() (string, []any, error) {
			return q.RecallFactsTemporal([]int64{7}, 10, nil, nil, nil, Known{}, tags)
		},
		"episode vector": func() (string, []any, error) {
			return q.RecallEpisodesWindowWithEmbedding([]int64{7}, vec, 10, nil, nil, nil, Known{}, false, tags)
		},
		"episode keyword": func() (string, []any, error) {
			return q.RecallEpisodesKeywordWindow([]int64{7}, "project", 10, nil, nil, nil, Known{}, tags)
		},
		"episode temporal": func() (string, []any, error) {
			return q.RecallEpisodesTemporal([]int64{7}, 10, nil, nil, nil, Known{}, tags)
		},
	}
	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			sql, args, err := build()
			if err != nil {
				t.Fatal(err)
			}
			want := "tags @> ARRAY[$"
			if name == "fact graph" {
				want = "f.tags @> ARRAY[$"
			}
			if !strings.Contains(sql, want) {
				t.Errorf("SQL lacks %q:\n%s", want, sql)
			}
			if !strings.Contains(sql, "]::text[]") {
				t.Errorf("SQL lacks the text[] cast:\n%s", sql)
			}
			bound := 0
			for _, a := range args {
				if s, ok := a.(string); ok && (s == "beslissing" || s == "pgvector") {
					bound++
				}
			}
			if bound != len(tags) {
				t.Errorf("bound %d of %d tag arguments: %v", bound, len(tags), args)
			}
		})
	}

	// Without tags the filter must disappear entirely, not match on an empty array.
	sql, args, err := q.RecallFactsWindowWithEmbedding([]int64{7}, vec, 10, nil, nil, nil, Known{}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sql, "tags @>") {
		t.Errorf("unfiltered recall still filters on tags:\n%s", sql)
	}
	for _, a := range args {
		if _, ok := a.([]string); ok {
			t.Errorf("unfiltered recall bound a tag argument: %v", args)
		}
	}
}
