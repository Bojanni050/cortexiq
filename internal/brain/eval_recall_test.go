package brain

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestLoadRecallDocsRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"missing content":   `{"id":"d1","content":""}`,
		"duplicate id":      "{\"id\":\"d1\",\"content\":\"a\"}\n{\"id\":\"d1\",\"content\":\"b\"}",
		"duplicate content": "{\"id\":\"d1\",\"content\":\"a\"}\n{\"id\":\"d2\",\"content\":\" a \"}",
		"not json":          `nope`,
		"empty":             "\n",
	}
	for name, in := range cases {
		if _, err := LoadRecallDocs(strings.NewReader(in)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestLoadRecallQueriesRejectsBadInput(t *testing.T) {
	docs := []RecallDoc{{ID: "d1", Content: "a"}}
	cases := map[string]string{
		"unknown kind":    `{"id":"q1","kind":"fuzzy","query":"a","expect":["d1"]}`,
		"unknown doc":     `{"id":"q1","kind":"identifier","query":"a","expect":["d9"]}`,
		"no expectation":  `{"id":"q1","kind":"identifier","query":"a","expect":[]}`,
		"empty query":     `{"id":"q1","kind":"identifier","query":" ","expect":["d1"]}`,
		"duplicate query": "{\"id\":\"q1\",\"kind\":\"identifier\",\"query\":\"a\",\"expect\":[\"d1\"]}\n{\"id\":\"q1\",\"kind\":\"identifier\",\"query\":\"a\",\"expect\":[\"d1\"]}",
	}
	for name, in := range cases {
		if _, err := LoadRecallQueries(strings.NewReader(in), docs); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// The shipped sets must stay loadable, and every kind must have enough questions
// to say something.
func TestShippedRecallSetLoads(t *testing.T) {
	df, err := os.Open("../../eval/recall_corpus.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer df.Close()
	docs, err := LoadRecallDocs(df)
	if err != nil {
		t.Fatal(err)
	}
	qf, err := os.Open("../../eval/recall_queries.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer qf.Close()
	qs, err := LoadRecallQueries(qf, docs)
	if err != nil {
		t.Fatal(err)
	}
	n := map[string]int{}
	for _, q := range qs {
		n[q.Kind]++
	}
	for _, kind := range []string{QueryIdentifier, QueryParaphrase, QueryCrossLanguage, QueryConfuser} {
		if n[kind] < 8 {
			t.Errorf("kind %s has %d questions, want at least 8", kind, n[kind])
		}
	}
	if len(docs) < 50 {
		t.Errorf("corpus has %d memories, want at least 50 so there are distractors", len(docs))
	}
}

func TestScoreRecall(t *testing.T) {
	qs := []RecallQuery{
		{ID: "q1", Kind: QueryIdentifier, Expect: []string{"a"}},
		{ID: "q2", Kind: QueryIdentifier, Expect: []string{"b"}},
		{ID: "q3", Kind: QueryConfuser, Expect: []string{"c"}},
		{ID: "q4", Kind: QueryConfuser, Expect: []string{"d", "e"}},
	}
	rankings := [][]string{
		{"a", "x"},           // rank 1
		{"x", "y", "b"},      // rank 3
		{"x", "y", "z"},      // not found
		{"x", "e", "d", "y"}, // either expected counts: rank 2
	}
	rep := ScoreRecall(qs, rankings, 3)

	if o := rep.Overall; o.Questions != 4 || o.HitAt1 != 0.25 || o.HitAt3 != 0.75 || o.HitAtK != 0.75 {
		t.Errorf("overall = %+v", o)
	}
	wantMRR := (1.0 + 1.0/3 + 0 + 0.5) / 4
	if d := rep.Overall.MRR - wantMRR; d > 1e-9 || d < -1e-9 {
		t.Errorf("MRR = %v, want %v", rep.Overall.MRR, wantMRR)
	}
	if k := rep.Kinds[QueryIdentifier]; k.Questions != 2 || k.HitAt1 != 0.5 {
		t.Errorf("identifier = %+v", k)
	}
	// Misses: everything not first, the one never found leading.
	if len(rep.Misses) != 3 || rep.Misses[0].ID != "q3" || rep.Misses[1].ID != "q2" {
		t.Errorf("misses = %+v", rep.Misses)
	}
	if got := len(rep.Outcomes[3].Top); got != 3 {
		t.Errorf("top is capped at 3, got %d", got)
	}
}

func TestDeleteEvalNamespaceRefusesOtherSlugs(t *testing.T) {
	b := &Brain{} // no pool: the guard must refuse before touching it
	for _, slug := range []string{"/", "/projects/stash", "/eval", "/eval/", "/evaluation/x"} {
		if err := b.deleteEvalNamespace(context.Background(), slug); err == nil {
			t.Errorf("deleteEvalNamespace(%q) should refuse", slug)
		}
	}
}

func TestIntegrationEvalRecallCleansUp(t *testing.T) {
	e := newEnv(t, nil)
	docs := []RecallDoc{
		{ID: "d1", Content: "The embedding_cache table stores vectors per text and model."},
		{ID: "d2", Content: "Backups are written daily to the backups directory."},
		{ID: "d3", Content: "The walkthrough continues in walkthrough2.md after one thousand lines."},
	}
	qs := []RecallQuery{
		{ID: "q1", Kind: QueryIdentifier, Query: "embedding_cache", Expect: []string{"d1"}},
		{ID: "q2", Kind: QueryIdentifier, Query: "walkthrough2.md", Expect: []string{"d3"}},
	}

	count := func() int {
		var n int
		if err := e.b.pool.QueryRow(e.ctx, "SELECT count(*) FROM namespaces WHERE slug LIKE '/eval/recall-%'").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := count()

	rep, err := e.b.EvalRecall(e.ctx, docs, qs, RecallEvalOptions{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Overall.Questions != 2 || rep.Overall.HitAt1 != 1 {
		t.Errorf("overall = %+v, want both identifiers at rank 1", rep.Overall)
	}
	if after := count(); after != before {
		t.Errorf("temporary namespaces: %d before, %d after; the run must clean up", before, after)
	}
}
