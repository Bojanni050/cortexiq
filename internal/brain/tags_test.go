package brain

import (
	"reflect"
	"testing"
	"time"

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

	tags, err := e.b.ListTags(e.ctx, []string{e.ns}, nil, 10)
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

	// A period that starts after the writes must count nothing: since filters on
	// created_at, so it reports what was stored in that period.
	future := time.Now().UTC().Add(time.Hour)
	none, err := e.b.ListTags(e.ctx, []string{e.ns}, &future, 10)
	if err != nil {
		t.Fatalf("ListTags(since future): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("ListTags(since future) = %v, want no tags", none)
	}

	// A period that started an hour ago must still see everything written now.
	past := time.Now().UTC().Add(-time.Hour)
	all, err := e.b.ListTags(e.ctx, []string{e.ns}, &past, 10)
	if err != nil {
		t.Fatalf("ListTags(since past): %v", err)
	}
	if len(all) != len(tags) {
		t.Errorf("ListTags(since past) returned %d tags, want %d", len(all), len(tags))
	}
}

// The vocabulary can only report tags that exist, so on its own it cannot tell
// an untagged namespace from an empty one. Coverage is what shows the gap, and
// it must count facts and episodes separately: a fact inherits its episode's
// tags, so the two counts move for different reasons.
func TestIntegrationTagCoverage(t *testing.T) {
	e := newEnv(t, nil)

	tagged, err := e.b.RememberWith(e.ctx, e.ns, "pgvector stores embeddings inside postgres", RememberOptions{
		Tags: []string{"beslissing", "pgvector"},
	})
	if err != nil {
		t.Fatalf("RememberWith(tagged): %v", err)
	}
	untagged, err := e.b.RememberWith(e.ctx, e.ns, "the queue is kafka not rabbitmq", RememberOptions{})
	if err != nil {
		t.Fatalf("RememberWith(untagged): %v", err)
	}
	if _, err := e.b.storeFact(e.ctx, e.nsID, &reasoner.StructuredFact{
		Entity:   "pgvector",
		Property: "stores",
		Value:    "embeddings",
		Summary:  "pgvector stores embeddings inside postgres",
	}, []int64{tagged}); err != nil {
		t.Fatalf("storeFact(tagged): %v", err)
	}
	if _, err := e.b.storeFact(e.ctx, e.nsID, &reasoner.StructuredFact{
		Entity:   "queue",
		Property: "is",
		Value:    "kafka",
		Summary:  "the queue is kafka not rabbitmq",
	}, []int64{untagged}); err != nil {
		t.Fatalf("storeFact(untagged): %v", err)
	}

	got, err := e.b.TagCoverage(e.ctx, []string{e.ns})
	if err != nil {
		t.Fatalf("TagCoverage: %v", err)
	}
	if got.FactsTagged != 1 || got.FactsUntagged != 1 {
		t.Errorf("facts = %d tagged / %d untagged, want 1 / 1", got.FactsTagged, got.FactsUntagged)
	}
	if got.EpisodesTagged != 1 || got.EpisodesUntagged != 1 {
		t.Errorf("episodes = %d tagged / %d untagged, want 1 / 1", got.EpisodesTagged, got.EpisodesUntagged)
	}
	if got.Tagged() != 2 || got.Untagged() != 2 {
		t.Errorf("totals = %d tagged / %d untagged, want 2 / 2", got.Tagged(), got.Untagged())
	}

	// Across every namespace the same rows are still counted, whatever else the
	// database holds, so the numbers can only grow.
	all, err := e.b.TagCoverage(e.ctx, nil)
	if err != nil {
		t.Fatalf("TagCoverage(all): %v", err)
	}
	if all.Tagged() < got.Tagged() || all.Untagged() < got.Untagged() {
		t.Errorf("coverage over all namespaces = %+v, smaller than %+v", all, got)
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

// A tag-scoped consolidation must read only the tagged episodes and must leave
// the checkpoint alone, so the untagged episodes it skipped are still picked up
// by the next normal run instead of being lost in the gap.
func TestIntegrationTagScopedConsolidation(t *testing.T) {
	e := newEnv(t, nil)

	tagged, err := e.b.RememberWith(e.ctx, e.ns, "we chose pgvector for embeddings", RememberOptions{
		Tags: []string{"beslissing"},
	})
	if err != nil {
		t.Fatalf("RememberWith: %v", err)
	}
	untagged, err := e.b.RememberWith(e.ctx, e.ns, "the queue is kafka not rabbitmq", RememberOptions{})
	if err != nil {
		t.Fatalf("RememberWith: %v", err)
	}

	before, err := e.b.GetOrCreateConsolidationProgress(e.ctx, e.nsID)
	if err != nil {
		t.Fatalf("progress before: %v", err)
	}

	res, err := e.b.ConsolidateWithTags(e.ctx, e.ns, []string{"beslissing"})
	if err != nil {
		t.Fatalf("ConsolidateWithTags: %v", err)
	}
	if len(res.TagFilter) != 1 || res.TagFilter[0] != "beslissing" {
		t.Errorf("tag filter = %v, want [beslissing]", res.TagFilter)
	}
	if res.EpisodesRead != 1 {
		t.Errorf("episodes read = %d, want 1 (only the tagged one)", res.EpisodesRead)
	}

	sources := func() map[int64]bool {
		t.Helper()
		rows, err := e.b.pool.Query(e.ctx,
			`SELECT DISTINCT fs.episode_id FROM fact_sources fs
			 JOIN facts f ON f.id = fs.fact_id WHERE f.namespace_id = $1`, e.nsID)
		if err != nil {
			t.Fatalf("read fact sources: %v", err)
		}
		defer rows.Close()
		out := map[int64]bool{}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				t.Fatalf("scan fact source: %v", err)
			}
			out[id] = true
		}
		return out
	}

	got := sources()
	if !got[tagged] {
		t.Errorf("tagged episode %d did not become a fact source", tagged)
	}
	if got[untagged] {
		t.Errorf("untagged episode %d was consolidated by a tag-scoped run", untagged)
	}

	after, err := e.b.GetOrCreateConsolidationProgress(e.ctx, e.nsID)
	if err != nil {
		t.Fatalf("progress after: %v", err)
	}
	if after.LastEpisodeID != before.LastEpisodeID {
		t.Errorf("checkpoint moved from %d to %d on a tag-scoped run", before.LastEpisodeID, after.LastEpisodeID)
	}

	// The skipped episode must still be reachable by a normal run.
	if _, err := e.b.Consolidate(e.ctx, e.ns); err != nil {
		t.Fatalf("Consolidate: %v", err)
	}
	if !sources()[untagged] {
		t.Errorf("untagged episode %d was lost: a normal run after a tag run did not consolidate it", untagged)
	}
}

// TagTimeline reports one count per tag per month, which is what the dashboard's
// trend chart draws. Months without memories are absent rather than zero rows.
func TestIntegrationTagTimeline(t *testing.T) {
	e := newEnv(t, nil)

	if _, err := e.b.RememberWith(e.ctx, e.ns, "pgvector stores embeddings inside postgres", RememberOptions{
		Tags: []string{"beslissing", "pgvector"},
	}); err != nil {
		t.Fatalf("RememberWith: %v", err)
	}
	if _, err := e.b.RememberWith(e.ctx, e.ns, "the queue is kafka not rabbitmq", RememberOptions{
		Tags: []string{"bugfix"},
	}); err != nil {
		t.Fatalf("RememberWith: %v", err)
	}

	trend, err := e.b.TagTimeline(e.ctx, []string{e.ns}, nil, 12)
	if err != nil {
		t.Fatalf("TagTimeline: %v", err)
	}
	month := time.Now().UTC().Format("2006-01")
	got := make(map[string]TagTrend, len(trend))
	for _, p := range trend {
		if p.Month != month {
			t.Errorf("month = %q, want the current month %q", p.Month, month)
		}
		got[p.Tag] = p
	}
	if p := got["beslissing"]; p.Episodes != 1 || p.Total != 1 {
		t.Errorf("beslissing = %+v, want 1 episode", p)
	}
	if p := got["bugfix"]; p.Episodes != 1 || p.Total != 1 {
		t.Errorf("bugfix = %+v, want 1 episode", p)
	}

	// The tag filter narrows the series to one tag; unknown tags give nothing.
	only, err := e.b.TagTimeline(e.ctx, []string{e.ns}, []string{"BUGFIX"}, 12)
	if err != nil {
		t.Fatalf("TagTimeline filtered: %v", err)
	}
	if len(only) != 1 || only[0].Tag != "bugfix" {
		t.Errorf("filtered trend = %+v, want only bugfix", only)
	}
	none, err := e.b.TagTimeline(e.ctx, []string{e.ns}, []string{"no-such-tag"}, 12)
	if err != nil {
		t.Fatalf("TagTimeline unknown tag: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("unknown tag returned %+v, want none", none)
	}

	if _, err := e.b.TagTimeline(e.ctx, []string{e.ns}, []string{"-bad"}, 12); err == nil {
		t.Error("invalid tag should be rejected, not silently ignored")
	}
}
