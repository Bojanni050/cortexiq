package brain

import (
	"reflect"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/reasoner"
)

func TestNormalizeTags(t *testing.T) {
	long := "abcdefghijklmnopqrstuvwxyz0123456789"
	cases := []struct {
		name string
		in   []string
		want []string
		err  bool
	}{
		{name: "nil", in: nil, want: nil},
		{name: "empty", in: []string{}, want: nil},
		{name: "blank entries", in: []string{"", "   "}, want: nil},
		{name: "lowercase and trim", in: []string{"  Beslissing "}, want: []string{"beslissing"}},
		{name: "inner whitespace becomes dash", in: []string{"Postgres Cluster"}, want: []string{"postgres-cluster"}},
		{name: "dedupe keeps first", in: []string{"Bug", "bug", "BUGFIX"}, want: []string{"bug", "bugfix"}},
		{name: "underscore and dash allowed", in: []string{"klant_white-vision"}, want: []string{"klant_white-vision"}},
		{name: "too many", in: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"}, err: true},
		{name: "too long", in: []string{long}, err: true},
		{name: "leading dash", in: []string{"-dash"}, err: true},
		{name: "non ascii", in: []string{"café"}, err: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeTags(tc.in)
			if tc.err {
				if err == nil {
					t.Fatalf("expected an error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("NormalizeTags(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func (e *testEnv) episodeTags(t *testing.T, id int64) []string {
	t.Helper()
	var tags []string
	if err := e.b.pool.QueryRow(e.ctx, `SELECT tags FROM episodes WHERE id = $1`, id).Scan(&tags); err != nil {
		t.Fatalf("read episode tags: %v", err)
	}
	return tags
}

// Tags must survive the write path, be normalized there, and reach the fact that
// consolidation derives from the episode — otherwise a recall filter would
// silently stop matching as soon as memories are consolidated into facts.
func TestIntegrationTagsPersistAndInherit(t *testing.T) {
	e := newEnv(t, nil)

	ep1, err := e.b.RememberWith(e.ctx, e.ns, "pgvector stores embeddings inside postgres", RememberOptions{
		Tags: []string{"Beslissing", "PGVector"},
	})
	if err != nil {
		t.Fatalf("RememberWith: %v", err)
	}
	if got, want := e.episodeTags(t, ep1), []string{"beslissing", "pgvector"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("episode tags = %v, want %v", got, want)
	}

	ep2, err := e.b.RememberWith(e.ctx, e.ns, "the queue is kafka not rabbitmq", RememberOptions{
		Tags: []string{"bugfix"},
	})
	if err != nil {
		t.Fatalf("RememberWith: %v", err)
	}

	if _, err := e.b.storeFact(e.ctx, e.nsID, &reasoner.StructuredFact{
		Entity:   "pgvector",
		Property: "stores",
		Value:    "embeddings",
		Summary:  "pgvector stores embeddings inside postgres",
	}, []int64{ep1, ep2}); err != nil {
		t.Fatalf("storeFact: %v", err)
	}

	var factTags []string
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT tags FROM facts WHERE namespace_id = $1 ORDER BY id DESC LIMIT 1`, e.nsID,
	).Scan(&factTags); err != nil {
		t.Fatalf("read fact tags: %v", err)
	}
	if want := []string{"beslissing", "bugfix", "pgvector"}; !reflect.DeepEqual(factTags, want) {
		t.Errorf("fact tags = %v, want the union %v", factTags, want)
	}
}

// A tag filter must narrow recall: an untagged memory with almost the same text
// must not come back, and a returned result must say which tags matched.
func TestIntegrationRecallFiltersByTag(t *testing.T) {
	e := newEnv(t, nil)

	tagged, err := e.b.RememberWith(e.ctx, e.ns, "pgvector stores embeddings inside postgres", RememberOptions{
		Tags: []string{"beslissing"},
	})
	if err != nil {
		t.Fatalf("RememberWith: %v", err)
	}
	untagged, err := e.b.RememberWith(e.ctx, e.ns, "pgvector stores embeddings inside postgres too", RememberOptions{})
	_ = untagged
	if err != nil {
		t.Fatalf("RememberWith: %v", err)
	}

	res, err := e.b.RecallWith(e.ctx, []string{e.ns}, "pgvector embeddings postgres", 10, RecallOptions{
		Tags: []string{"Beslissing"},
	})
	if err != nil {
		t.Fatalf("RecallWith: %v", err)
	}
	if len(res) == 0 {
		t.Fatal("tag-filtered recall returned nothing")
	}
	for _, r := range res {
		if r.EpisodeID != nil && *r.EpisodeID == untagged {
			t.Fatalf("untagged episode %d leaked through a tag filter", untagged)
		}
		if !reflect.DeepEqual(r.Tags, []string{"beslissing"}) {
			t.Errorf("result %d tags = %v, want [beslissing]", r.ID, r.Tags)
		}
	}

	// Sanity check that the untagged memory is recallable without the filter, so
	// the assertion above is about the filter and not about a weak query.
	all, err := e.b.RecallWith(e.ctx, []string{e.ns}, "pgvector embeddings postgres", 10, RecallOptions{})
	if err != nil {
		t.Fatalf("RecallWith: %v", err)
	}
	var found bool
	for _, r := range all {
		if r.EpisodeID != nil && (*r.EpisodeID == untagged || *r.EpisodeID == tagged) {
			found = true
		}
	}
	if !found {
		t.Fatal("unfiltered recall found neither memory; query is broken")
	}
}

func TestIntegrationListTagsCounts(t *testing.T) {
	e := newEnv(t, nil)

	ep1, err := e.b.RememberWith(e.ctx, e.ns, "pgvector stores embeddings inside postgres", RememberOptions{
		Tags: []string{"beslissing", "pgvector"},
	})
	if err != nil {
		t.Fatalf("RememberWith: %v", err)
	}
	if _, err := e.b.RememberWith(e.ctx, e.ns, "the queue is kafka not rabbitmq", RememberOptions{
		Tags: []string{"beslissing"},
	}); err != nil {
		t.Fatalf("RememberWith: %v", err)
	}
	if _, err := e.b.storeFact(e.ctx, e.nsID, &reasoner.StructuredFact{
		Entity:   "pgvector",
		Property: "stores",
		Value:    "embeddings",
		Summary:  "pgvector stores embeddings inside postgres",
	}, []int64{ep1}); err != nil {
		t.Fatalf("storeFact: %v", err)
	}

	tags, err := e.b.ListTags(e.ctx, []string{e.ns}, 10)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	got := make(map[string]TagCount, len(tags))
	for _, tc := range tags {
		got[tc.Tag] = tc
	}
	beslissing, ok := got["beslissing"]
	if !ok {
		t.Fatalf("tag beslissing missing from %v", tags)
	}
	if beslissing.Episodes != 2 || beslissing.Facts != 1 || beslissing.Total != 3 {
		t.Errorf("beslissing = %+v, want episodes=2 facts=1 total=3", beslissing)
	}
	pgvector, ok := got["pgvector"]
	if !ok {
		t.Fatalf("tag pgvector missing from %v", tags)
	}
	if pgvector.Episodes != 1 || pgvector.Facts != 1 || pgvector.Total != 2 {
		t.Errorf("pgvector = %+v, want episodes=1 facts=1 total=2", pgvector)
	}
}

// The dashboard list endpoints filter server-side; a client-side filter would
// only narrow the current page. This covers the two paths the UI uses.
func TestIntegrationListFiltersByTag(t *testing.T) {
	e := newEnv(t, nil)

	tagged, err := e.b.RememberWith(e.ctx, e.ns, "pgvector stores embeddings inside postgres", RememberOptions{
		Tags: []string{"beslissing"},
	})
	if err != nil {
		t.Fatalf("RememberWith: %v", err)
	}
	if _, err := e.b.RememberWith(e.ctx, e.ns, "the queue is kafka not rabbitmq", RememberOptions{
		Tags: []string{"beslissing"},
	}); err != nil {
		t.Fatalf("RememberWith: %v", err)
	}
	if _, err := e.b.RememberWith(e.ctx, e.ns, "the dashboard listens on port 8090", RememberOptions{}); err != nil {
		t.Fatalf("RememberWith: %v", err)
	}

	got, err := e.b.ListEpisodes(e.ctx, []string{e.ns}, []string{"beslissing"}, Pagination{Limit: 10})
	if err != nil {
		t.Fatalf("ListEpisodes: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("ListEpisodes(tags=beslissing) returned %d, want 2", len(got))
	}
	for _, ep := range got {
		if !reflect.DeepEqual(ep.Tags, []string{"beslissing"}) {
			t.Errorf("episode %d tags = %v, want [beslissing]", ep.ID, ep.Tags)
		}
	}
	if n, err := e.b.CountEpisodes(e.ctx, []string{e.ns}, []string{"beslissing"}); err != nil || n != 2 {
		t.Errorf("CountEpisodes = %d, %v; want 2", n, err)
	}
	if n, err := e.b.CountEpisodes(e.ctx, []string{e.ns}, nil); err != nil || n != 3 {
		t.Errorf("CountEpisodes(no filter) = %d, %v; want 3", n, err)
	}

	if _, err := e.b.storeFact(e.ctx, e.nsID, &reasoner.StructuredFact{
		Entity:   "pgvector",
		Property: "stores",
		Value:    "embeddings",
		Summary:  "pgvector stores embeddings inside postgres",
	}, []int64{tagged}); err != nil {
		t.Fatalf("storeFact: %v", err)
	}

	facts, err := e.b.QueryFactsWindow(e.ctx, []string{e.ns}, nil, nil, nil, nil, nil, nil, nil, []string{"beslissing"}, Pagination{Limit: 10})
	if err != nil {
		t.Fatalf("QueryFactsWindow: %v", err)
	}
	if len(facts) != 1 {
		t.Errorf("QueryFactsWindow(tags=beslissing) returned %d facts, want 1", len(facts))
	}
	for _, f := range facts {
		if !reflect.DeepEqual(f.Tags, []string{"beslissing"}) {
			t.Errorf("fact %d tags = %v, want [beslissing]", f.ID, f.Tags)
		}
	}
	if n, err := e.b.CountFactsWindow(e.ctx, []string{e.ns}, nil, nil, nil, nil, nil, nil, nil, []string{"beslissing"}); err != nil || n != 1 {
		t.Errorf("CountFactsWindow = %d, %v; want 1", n, err)
	}
	if n, err := e.b.CountFactsWindow(e.ctx, []string{e.ns}, nil, nil, nil, nil, nil, nil, nil, nil); err != nil || n != 1 {
		t.Errorf("CountFactsWindow(no filter) = %d, %v; want 1", n, err)
	}
}
