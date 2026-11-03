package brain

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/reasoner"
)

func TestNormalizeExternalKey(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		err  bool
	}{
		{name: "empty is not keyed", in: "", want: ""},
		{name: "blank is not keyed", in: "   ", want: ""},
		{name: "trimmed", in: "  gaia:hypothesis:42 ", want: "gaia:hypothesis:42"},
		{name: "inner space", in: "gaia hypothesis", err: true},
		{name: "control character", in: "gaia\x00key", err: true},
		{name: "too long", in: strings.Repeat("k", MaxExternalKeyLength+1), err: true},
		{name: "exactly max", in: strings.Repeat("k", MaxExternalKeyLength), want: strings.Repeat("k", MaxExternalKeyLength)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeExternalKey(tc.in)
			if tc.err {
				if !errors.Is(err, ErrInvalidExternalKey) {
					t.Fatalf("expected ErrInvalidExternalKey, got %v (key %q)", err, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("NormalizeExternalKey(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeExternalRefs(t *testing.T) {
	tooMany := make([]string, MaxExternalRefs+1)
	for i := range tooMany {
		tooMany[i] = "ref:" + strings.Repeat("x", i+1)
	}
	cases := []struct {
		name string
		in   []string
		want []string
		err  bool
	}{
		{name: "nil", in: nil, want: nil},
		{name: "only blanks", in: []string{"", "  "}, want: nil},
		{name: "trim and dedupe keep first", in: []string{" commit:bf575f8", "commit:bf575f8", "https://example.com/a"}, want: []string{"commit:bf575f8", "https://example.com/a"}},
		{name: "inner space", in: []string{"two words"}, err: true},
		{name: "too long ref", in: []string{strings.Repeat("r", MaxExternalRefLength+1)}, err: true},
		{name: "too many", in: tooMany, err: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeExternalRefs(tc.in)
			if tc.err {
				if !errors.Is(err, ErrInvalidExternalKey) {
					t.Fatalf("expected ErrInvalidExternalKey, got %v (refs %v)", err, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("NormalizeExternalRefs(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// Supersedes without a key has nothing to supersede; refusing it up front keeps
// a typo from silently becoming a plain duplicate write.
func TestSupersedesNeedsKey(t *testing.T) {
	e := newEnv(t, nil)
	_, err := e.b.RememberWithOutcome(e.ctx, e.ns, "no key here", RememberOptions{Supersedes: true})
	if !errors.Is(err, ErrSupersedesNeedsKey) {
		t.Fatalf("want ErrSupersedesNeedsKey, got %v", err)
	}
}

func (e *testEnv) episodeRow(t *testing.T, id int64) (live bool, key string, supersededBy int64, refs []string) {
	t.Helper()
	var deleted *string
	var k *string
	var sb *int64
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT deleted_at::text, external_key, superseded_by, external_refs FROM episodes WHERE id = $1`, id,
	).Scan(&deleted, &k, &sb, &refs); err != nil {
		t.Fatalf("read episode %d: %v", id, err)
	}
	if k != nil {
		key = *k
	}
	if sb != nil {
		supersededBy = *sb
	}
	return deleted == nil, key, supersededBy, refs
}

// Writing the same content under the same key again is a replay: it returns the
// existing episode and writes nothing, so a rebuild from scratch is idempotent.
func TestIntegrationKeyedReplayIsIdempotent(t *testing.T) {
	e := newEnv(t, nil)
	opts := RememberOptions{ExternalKey: "gaia:hypothesis:42", ExternalRefs: []string{"foundation:9f3a"}}

	first, err := e.b.RememberWithOutcome(e.ctx, e.ns, "hypotheses need human confirmation only", opts)
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	if first.Unchanged || first.ID == 0 {
		t.Fatalf("first write = %+v, want a new episode", first)
	}

	again, err := e.b.RememberWithOutcome(e.ctx, e.ns, "hypotheses need human confirmation only", opts)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !again.Unchanged || again.ID != first.ID {
		t.Fatalf("replay = %+v, want Unchanged on episode %d", again, first.ID)
	}

	var n int
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT count(*) FROM episodes WHERE namespace_id = $1 AND external_key = $2`, e.nsID, "gaia:hypothesis:42",
	).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("rows for the key = %d, want 1", n)
	}
}

// Different content under a live key is refused without Supersedes, and the
// refusal must leave the old episode live and untouched.
func TestIntegrationKeyConflictLeavesOldEpisodeLive(t *testing.T) {
	e := newEnv(t, nil)
	old, err := e.b.RememberWithOutcome(e.ctx, e.ns, "status is proposed", RememberOptions{ExternalKey: "gaia:hypothesis:7"})
	if err != nil {
		t.Fatalf("first write: %v", err)
	}

	_, err = e.b.RememberWithOutcome(e.ctx, e.ns, "status is confirmed", RememberOptions{ExternalKey: "gaia:hypothesis:7"})
	if !errors.Is(err, ErrExternalKeyExists) {
		t.Fatalf("want ErrExternalKeyExists, got %v", err)
	}

	live, key, supersededBy, _ := e.episodeRow(t, old.ID)
	if !live || key != "gaia:hypothesis:7" || supersededBy != 0 {
		t.Errorf("old episode after refusal: live=%v key=%q superseded_by=%d, want live, keyed, not superseded", live, key, supersededBy)
	}
}

// A superseding write retires the old episode, links it to the new one, frees
// the key for the successor, and closes facts that rested only on the old
// episode — in one step. A fact that also has another live source stays open.
func TestIntegrationSupersedeRetiresOldAndEndsOnlySourcedFacts(t *testing.T) {
	e := newEnv(t, nil)

	old, err := e.b.RememberWithOutcome(e.ctx, e.ns, "pgvector stores embeddings inside postgres", RememberOptions{
		ExternalKey: "doc:storage", ExternalRefs: []string{"commit:aaa1111"},
	})
	if err != nil {
		t.Fatalf("old write: %v", err)
	}
	other, err := e.b.RememberWith(e.ctx, e.ns, "an unrelated second observation about postgres", RememberOptions{})
	if err != nil {
		t.Fatalf("other write: %v", err)
	}

	onlyOld, err := e.b.storeFact(e.ctx, e.nsID, &reasoner.StructuredFact{
		Entity: "pgvector", Property: "stores", Value: "embeddings", Summary: "pgvector stores embeddings inside postgres",
	}, []int64{old.ID})
	if err != nil || !onlyOld.created {
		t.Fatalf("storeFact(only old): %+v %v", onlyOld, err)
	}
	shared, err := e.b.storeFact(e.ctx, e.nsID, &reasoner.StructuredFact{
		Entity: "postgres", Property: "is", Value: "a database", Summary: "postgres is a relational database",
	}, []int64{old.ID, other})
	if err != nil || !shared.created {
		t.Fatalf("storeFact(shared): %+v %v", shared, err)
	}

	next, err := e.b.RememberWithOutcome(e.ctx, e.ns, "pgvector stores embeddings in a dedicated column", RememberOptions{
		ExternalKey: "doc:storage", ExternalRefs: []string{"commit:bbb2222"}, Supersedes: true,
	})
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}
	if next.SupersededID != old.ID || next.ID == old.ID || next.Unchanged {
		t.Fatalf("supersede outcome = %+v, want it to retire episode %d", next, old.ID)
	}
	if next.FactsEnded != 1 {
		t.Errorf("FactsEnded = %d, want 1 (only the fact resting solely on the old episode)", next.FactsEnded)
	}

	live, _, supersededBy, _ := e.episodeRow(t, old.ID)
	if live || supersededBy != next.ID {
		t.Errorf("old episode: live=%v superseded_by=%d, want retired and linked to %d", live, supersededBy, next.ID)
	}
	liveNew, key, _, refs := e.episodeRow(t, next.ID)
	if !liveNew || key != "doc:storage" || !reflect.DeepEqual(refs, []string{"commit:bbb2222"}) {
		t.Errorf("new episode: live=%v key=%q refs=%v", liveNew, key, refs)
	}

	var endedOnlyOld, endedShared bool
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT known_until IS NOT NULL FROM facts WHERE content = $1 AND namespace_id = $2`,
		"pgvector stores embeddings inside postgres", e.nsID,
	).Scan(&endedOnlyOld); err != nil {
		t.Fatalf("read fact: %v", err)
	}
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT known_until IS NOT NULL FROM facts WHERE content = $1 AND namespace_id = $2`,
		"postgres is a relational database", e.nsID,
	).Scan(&endedShared); err != nil {
		t.Fatalf("read fact: %v", err)
	}
	if !endedOnlyOld {
		t.Error("the fact resting only on the superseded episode should be closed")
	}
	if endedShared {
		t.Error("a fact with another live source must stay open")
	}
}

// External refs written on an episode must reach the fact consolidation derives
// from it (the union over its sources) and come back in recall results.
func TestIntegrationExternalRefsInheritAndRecall(t *testing.T) {
	e := newEnv(t, nil)

	ep1, err := e.b.RememberWithOutcome(e.ctx, e.ns, "the deploy pipeline builds the foundation artifacts first", RememberOptions{
		ExternalKey: "ops:deploy-order", ExternalRefs: []string{"commit:bf575f8", "foundation:9f3a"},
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	ep2, err := e.b.RememberWithOutcome(e.ctx, e.ns, "the deploy pipeline also rebuilds cognition afterwards", RememberOptions{
		ExternalRefs: []string{"commit:ccc3333", "foundation:9f3a"},
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := e.b.storeFact(e.ctx, e.nsID, &reasoner.StructuredFact{
		Entity: "deploy pipeline", Property: "order", Value: "foundation then cognition",
		Summary: "the deploy pipeline builds foundation artifacts and then rebuilds cognition",
	}, []int64{ep1.ID, ep2.ID}); err != nil {
		t.Fatalf("storeFact: %v", err)
	}

	var factRefs []string
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT external_refs FROM facts WHERE namespace_id = $1 ORDER BY id DESC LIMIT 1`, e.nsID,
	).Scan(&factRefs); err != nil {
		t.Fatalf("read fact refs: %v", err)
	}
	if want := []string{"commit:bf575f8", "commit:ccc3333", "foundation:9f3a"}; !reflect.DeepEqual(factRefs, want) {
		t.Errorf("fact refs = %v, want the union %v", factRefs, want)
	}

	res, err := e.b.RecallWith(e.ctx, []string{e.ns}, "deploy pipeline foundation cognition", 10, RecallOptions{})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	var sawKeyed, sawRefs bool
	for _, r := range res {
		if r.Type == "episode" && r.EpisodeID != nil && *r.EpisodeID == ep1.ID {
			sawKeyed = r.ExternalKey == "ops:deploy-order" && reflect.DeepEqual(r.ExternalRefs, []string{"commit:bf575f8", "foundation:9f3a"})
		}
		if r.Type == "fact" && len(r.ExternalRefs) == 3 {
			sawRefs = true
		}
	}
	if !sawKeyed {
		t.Error("recall did not return the keyed episode with its key and refs")
	}
	if !sawRefs {
		t.Error("recall did not return the fact with its inherited refs")
	}
}
