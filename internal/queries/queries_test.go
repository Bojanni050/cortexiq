package queries

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pgvector/pgvector-go"
)

// The keyword templates bind the query several times and a variable number of
// namespace IDs; make sure placeholders and args stay in step. Facts and
// episodes use BM25 and bind the query three times (score, match guard, order);
// failures and goals still use ts_rank_cd and bind it twice (rank + filter).
func TestRecallKeywordTemplatesRender(t *testing.T) {
	q, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for name, tc := range map[string]struct {
		build    func([]int64) (string, []any, error)
		queryUse int    // how often the query is bound
		contains string // what proves the right ranker was rendered
	}{
		"facts": {func(ns []int64) (string, []any, error) { return q.RecallFactsKeyword(ns, "cortexiq or port", 30, nil) },
			3, "to_bm25query($1, 'facts_content_bm25')"},
		"episodes": {func(ns []int64) (string, []any, error) {
			return q.RecallEpisodesKeyword(ns, "cortexiq or port", 30, nil)
		}, 3, "to_bm25query($1, 'episodes_content_bm25')"},
		"failures": {func(ns []int64) (string, []any, error) {
			return q.RecallFailuresKeyword(ns, "cortexiq or port", 30, nil)
		}, 2, "websearch_to_tsquery('simple', $1)"},
		"goals": {func(ns []int64) (string, []any, error) { return q.RecallGoalsKeyword(ns, "cortexiq or port", 30, nil) },
			2, "websearch_to_tsquery('simple', $1)"},
	} {
		sql, args, err := tc.build([]int64{1, 2, 3})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// query xN, 3 namespace ids, limit
		want := tc.queryUse + 3 + 1
		if len(args) != want {
			t.Errorf("%s: got %d args, want %d", name, len(args), want)
		}
		if !strings.Contains(sql, tc.contains) || !strings.Contains(sql, "$"+strconv.Itoa(want)) {
			t.Errorf("%s: unexpected SQL:\n%s", name, sql)
		}

		// Without namespaces: query xN + limit, and no IN clause.
		sql, args, err = tc.build(nil)
		if err != nil {
			t.Fatalf("%s (no ns): %v", name, err)
		}
		if len(args) != tc.queryUse+1 || strings.Contains(sql, "namespace_id IN") {
			t.Errorf("%s (no ns): args=%d sql=\n%s", name, len(args), sql)
		}
	}
}

// BM25 reads the query text itself with a configuration that has no stop words,
// so the "or" that joins websearch terms must not reach it as a word.
func TestBM25TextDropsTheOperator(t *testing.T) {
	for in, want := range map[string]string{
		"":                         "",
		"cortexiq":                 "cortexiq",
		"cortexiq or port or 8445": "cortexiq port 8445",
		"welke or poort or draait": "welke poort draait",
		"  spaced   or  out  ":     "spaced out",
		"or":                       "",
		"order or origin":          "order origin",
	} {
		if got := bm25Text(in); got != want {
			t.Errorf("bm25Text(%q) = %q, want %q", in, got, want)
		}
	}
}

// The score is negated in the select list (the operator returns lower-is-better),
// the order keeps the raw operator so the index can serve it, and the match guard
// keeps non-matching rows out when a selective filter makes Postgres score rows
// instead of walking the index.
func TestBM25KeywordShape(t *testing.T) {
	q, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for table, build := range map[string]func() (string, []any, error){
		"facts":    func() (string, []any, error) { return q.RecallFactsKeyword([]int64{1}, "a or b", 5, nil) },
		"episodes": func() (string, []any, error) { return q.RecallEpisodesKeyword([]int64{1}, "a or b", 5, nil) },
	} {
		sql, args, err := build()
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if args[0] != "a b" {
			t.Errorf("%s: query arg = %q, want plain terms %q", table, args[0], "a b")
		}
		doc := "regexp_replace(content, '[^[:alnum:]]+', ' ', 'g')"
		for _, frag := range []string{
			"-(" + doc + " <@> to_bm25query(",
			"AND " + doc + " <@> to_bm25query($",
			"ORDER BY " + doc + " <@> to_bm25query($",
			"'" + table + "_content_bm25') < 0",
		} {
			if !strings.Contains(sql, frag) {
				t.Errorf("%s: SQL lacks %q:\n%s", table, frag, sql)
			}
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
	// query x3 (BM25 score, guard, order), as_of, limit
	if len(args) != 5 || !strings.Contains(sql, "created_at <=") {
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
			return q.RecallFactsKeywordWindow([]int64{7}, "project", 10, nil, &from, &to, Known{}, TagFilter{})
		},
		"fact graph": func() (string, []any, error) {
			return q.RecallFactsGraphWindow([]int64{7}, "project", vec, 10, 0.4, nil, &from, &to, Known{}, TagFilter{})
		},
		"episode vector": func() (string, []any, error) {
			return q.RecallEpisodesWindow([]int64{7}, vec, 10, nil, &from, &to, Known{})
		},
		"episode keyword": func() (string, []any, error) {
			return q.RecallEpisodesKeywordWindow([]int64{7}, "project", 10, nil, &from, &to, Known{}, TagFilter{})
		},
		"fact temporal": func() (string, []any, error) {
			return q.RecallFactsTemporal([]int64{7}, 10, nil, &from, &to, Known{}, TagFilter{})
		},
		"episode temporal": func() (string, []any, error) {
			return q.RecallEpisodesTemporal([]int64{7}, 10, nil, &from, &to, Known{}, TagFilter{})
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
			return q.RecallFactsWindowWithEmbedding([]int64{7}, vec, 10, nil, nil, nil, Known{}, false, TagFilter{All: tags})
		},
		"fact keyword": func() (string, []any, error) {
			return q.RecallFactsKeywordWindow([]int64{7}, "project", 10, nil, nil, nil, Known{}, TagFilter{All: tags})
		},
		"fact graph": func() (string, []any, error) {
			return q.RecallFactsGraphWindow([]int64{7}, "project", vec, 10, 0, nil, nil, nil, Known{}, TagFilter{All: tags})
		},
		"fact temporal": func() (string, []any, error) {
			return q.RecallFactsTemporal([]int64{7}, 10, nil, nil, nil, Known{}, TagFilter{All: tags})
		},
		"episode vector": func() (string, []any, error) {
			return q.RecallEpisodesWindowWithEmbedding([]int64{7}, vec, 10, nil, nil, nil, Known{}, false, TagFilter{All: tags})
		},
		"episode keyword": func() (string, []any, error) {
			return q.RecallEpisodesKeywordWindow([]int64{7}, "project", 10, nil, nil, nil, Known{}, TagFilter{All: tags})
		},
		"episode temporal": func() (string, []any, error) {
			return q.RecallEpisodesTemporal([]int64{7}, 10, nil, nil, nil, Known{}, TagFilter{All: tags})
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
	sql, args, err := q.RecallFactsWindowWithEmbedding([]int64{7}, vec, 10, nil, nil, nil, Known{}, false, TagFilter{})
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

// tags_any must reach every arm too, as an overlap (&&) rather than a
// containment (@>): the two conditions are different operators against the same
// GIN index, and a missing one silently widens the result.
func TestRecallTagsAnyRendersInEveryArm(t *testing.T) {
	q, err := New()
	if err != nil {
		t.Fatal(err)
	}
	vec := pgvector.NewVector([]float32{1, 0})
	anyTags := []string{"gaia:hypothesis", "gaia:decision"}
	builders := map[string]func() (string, []any, error){
		"fact vector": func() (string, []any, error) {
			return q.RecallFactsWindowWithEmbedding([]int64{7}, vec, 10, nil, nil, nil, Known{}, false, TagFilter{Any: anyTags})
		},
		"fact keyword": func() (string, []any, error) {
			return q.RecallFactsKeywordWindow([]int64{7}, "project", 10, nil, nil, nil, Known{}, TagFilter{Any: anyTags})
		},
		"fact graph": func() (string, []any, error) {
			return q.RecallFactsGraphWindow([]int64{7}, "project", vec, 10, 0, nil, nil, nil, Known{}, TagFilter{Any: anyTags})
		},
		"fact temporal": func() (string, []any, error) {
			return q.RecallFactsTemporal([]int64{7}, 10, nil, nil, nil, Known{}, TagFilter{Any: anyTags})
		},
		"episode vector": func() (string, []any, error) {
			return q.RecallEpisodesWindowWithEmbedding([]int64{7}, vec, 10, nil, nil, nil, Known{}, false, TagFilter{Any: anyTags})
		},
		"episode keyword": func() (string, []any, error) {
			return q.RecallEpisodesKeywordWindow([]int64{7}, "project", 10, nil, nil, nil, Known{}, TagFilter{Any: anyTags})
		},
		"episode temporal": func() (string, []any, error) {
			return q.RecallEpisodesTemporal([]int64{7}, 10, nil, nil, nil, Known{}, TagFilter{Any: anyTags})
		},
	}
	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			sql, args, err := build()
			if err != nil {
				t.Fatal(err)
			}
			want := "tags && ARRAY[$"
			if name == "fact graph" {
				want = "f.tags && ARRAY[$"
			}
			if !strings.Contains(sql, want) {
				t.Errorf("SQL lacks %q:\n%s", want, sql)
			}
			if strings.Contains(sql, "tags @>") {
				t.Errorf("tags_any rendered as containment:\n%s", sql)
			}
			if !strings.Contains(sql, "]::text[]") {
				t.Errorf("SQL lacks the text[] cast:\n%s", sql)
			}
			if !argsContainAll(args, anyTags) {
				t.Errorf("tags_any arguments not bound: %v", args)
			}
		})
	}
}

// Both conditions at once must render as two clauses: tags_any is never allowed
// to stand in for tags, or a caller asking for "every tag and one of these" would
// get the union instead of the intersection.
func TestRecallTagsAllAndAnyRenderTogether(t *testing.T) {
	q, err := New()
	if err != nil {
		t.Fatal(err)
	}
	vec := pgvector.NewVector([]float32{1, 0})
	sql, args, err := q.RecallFactsWindowWithEmbedding([]int64{7}, vec, 10, nil, nil, nil, Known{}, false,
		TagFilter{All: []string{"beslissing"}, Any: []string{"gaia:hypothesis", "gaia:decision"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "tags @> ARRAY[$") || !strings.Contains(sql, "tags && ARRAY[$") {
		t.Errorf("both tag conditions should render:\n%s", sql)
	}
	if !strings.Contains(sql, " AND ") {
		t.Errorf("conditions should be ANDed:\n%s", sql)
	}
	bound := 0
	for _, a := range args {
		if s, ok := a.(string); ok && (s == "beslissing" || s == "gaia:hypothesis" || s == "gaia:decision") {
			bound++
		}
	}
	if bound != 3 {
		t.Errorf("bound %d of 3 tag arguments: %v", bound, args)
	}
}

// argsContainAll reports whether every tag reached the argument list. The
// template expands a slice into one placeholder per element, so tags arrive as
// separate string arguments rather than as a []string.
func argsContainAll(args []any, want []string) bool {
	seen := map[string]bool{}
	for _, a := range args {
		if s, ok := a.(string); ok {
			seen[s] = true
		}
	}
	for _, w := range want {
		if !seen[w] {
			return false
		}
	}
	return true
}
