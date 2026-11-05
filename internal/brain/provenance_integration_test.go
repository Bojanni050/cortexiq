package brain

// Integration tests for fact provenance: every derived fact says which path
// wrote it and what it rested on, and a derived fact with no source is refused.
// Skipped unless CORTEXIQ_TEST_DSN is set, same as the rest of the integration
// suite.

import (
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/reasoner"
)

type factOrigin struct {
	origin, ref *string
	sources     []int64
}

func (e *testEnv) factOrigin(t *testing.T, factID int64) factOrigin {
	t.Helper()
	var o factOrigin
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT origin, origin_ref FROM facts WHERE id = $1`, factID).Scan(&o.origin, &o.ref); err != nil {
		t.Fatalf("read origin of fact %d: %v", factID, err)
	}
	rows, err := e.b.pool.Query(e.ctx,
		`SELECT episode_id FROM fact_sources WHERE fact_id = $1 ORDER BY episode_id`, factID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		o.sources = append(o.sources, id)
	}
	return o
}

func str(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func (e *testEnv) factCount(t *testing.T, content string) int {
	t.Helper()
	var n int
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT count(*) FROM facts WHERE namespace_id = $1 AND content = $2`, e.nsID, content).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestIntegrationConsolidatedFactRecordsItsOriginAndSources(t *testing.T) {
	e := newEnv(t, nil)
	e.rememberEpisode(t, "the nightly backups are copied to the offsite storage box")

	res, err := e.b.Consolidate(e.ctx, e.ns)
	if err != nil {
		t.Fatal(err)
	}
	if res.FactsCreated == 0 {
		t.Fatalf("consolidation created no fact: %+v", res)
	}
	var factID int64
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT id FROM facts WHERE namespace_id = $1 ORDER BY id LIMIT 1`, e.nsID).Scan(&factID); err != nil {
		t.Fatal(err)
	}
	o := e.factOrigin(t, factID)
	if str(o.origin) != "consolidation" || o.ref != nil {
		t.Fatalf("origin = %s ref = %s; want consolidation with no ref", str(o.origin), str(o.ref))
	}
	if len(o.sources) == 0 {
		t.Fatal("no fact_sources for the consolidated fact")
	}
}

// The check sits in insertFact, so no caller can forget it.
func TestIntegrationDerivedFactWithoutASourceIsRefused(t *testing.T) {
	e := newEnv(t, nil)
	const content = "a fact that rests on nothing at all"
	vec, _ := fakeEmbedder{}.Embed(e.ctx, content)

	for _, origin := range []string{originConsolidation, originHypothesis, originFailure} {
		_, err := e.b.insertFact(e.ctx, e.b.pool, factInsert{
			Origin: origin, NamespaceID: e.nsID, Content: content, Embedding: vec,
			EmbeddingModel: "fake", Confidence: 0.8, Importance: DefaultImportance,
		})
		if !errors.Is(err, ErrNoSource) {
			t.Errorf("origin %s without a source err = %v; want ErrNoSource", origin, err)
		}
	}
	if n := e.factCount(t, content); n != 0 {
		t.Fatalf("%d facts written despite the refusal", n)
	}
}

// Import restores history that has no sources in its file, and the harness
// writes fixtures: neither is asked for a source.
func TestIntegrationImportAndEvalNeedNoSource(t *testing.T) {
	e := newEnv(t, nil)
	vec, _ := fakeEmbedder{}.Embed(e.ctx, "restored from a backup file")
	for _, origin := range []string{originImport, originEval} {
		id, err := e.b.insertFact(e.ctx, e.b.pool, factInsert{
			Origin: origin, NamespaceID: e.nsID, Content: "restored from a backup file " + origin,
			Embedding: vec, EmbeddingModel: "fake", Confidence: 0.8, Importance: DefaultImportance,
		})
		if err != nil {
			t.Fatalf("origin %s err = %v; want it stored", origin, err)
		}
		if got := str(e.factOrigin(t, id).origin); got != origin {
			t.Errorf("origin = %s, want %s", got, origin)
		}
	}
}

// The fact and its sources are one statement. A source that does not exist
// fails the whole write, and no fact is left behind without it.
func TestIntegrationFactAndSourcesAreWrittenTogether(t *testing.T) {
	e := newEnv(t, nil)
	const content = "claimed to come from an episode that is not there"
	vec, _ := fakeEmbedder{}.Embed(e.ctx, content)

	_, err := e.b.insertFact(e.ctx, e.b.pool, factInsert{
		Origin: originConsolidation, NamespaceID: e.nsID, Content: content,
		Embedding: vec, EmbeddingModel: "fake", Confidence: 0.8, Importance: DefaultImportance,
		SourceEpisodeIDs: []int64{999_999_999},
	})
	if err == nil {
		t.Fatal("insert with a missing source episode succeeded")
	}
	if n := e.factCount(t, content); n != 0 {
		t.Fatalf("%d fact rows left behind without their source", n)
	}
}

func TestIntegrationConfirmedHypothesisRecordsItsOriginAndEvidence(t *testing.T) {
	e := newEnv(t, nil)
	ep := e.rememberEpisode(t, "the staging database is rebuilt every friday night")
	evidence := e.insertFact(t, "the staging database is rebuilt every friday night", nil)
	if _, err := e.b.pool.Exec(e.ctx,
		`INSERT INTO fact_sources (fact_id, episode_id) VALUES ($1, $2)`, evidence, ep); err != nil {
		t.Fatal(err)
	}

	h, err := e.b.CreateHypothesis(e.ctx, e.nsID, "staging is rebuilt weekly", "check the cron log", 0.7, []int64{evidence})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.UpdateHypothesisStatus(e.ctx, h.ID, "testing"); err != nil {
		t.Fatal(err)
	}
	_, fact, err := e.b.ConfirmHypothesis(e.ctx, h.ID)
	if err != nil {
		t.Fatalf("ConfirmHypothesis: %v", err)
	}

	o := e.factOrigin(t, fact.ID)
	if str(o.origin) != "hypothesis" || str(o.ref) != "hypothesis:"+itoa(h.ID) {
		t.Fatalf("origin = %s ref = %s", str(o.origin), str(o.ref))
	}
	if !reflect.DeepEqual(o.sources, []int64{ep}) {
		t.Fatalf("sources = %v; want the episode behind the evidence fact, [%d]", o.sources, ep)
	}
}

// A hypothesis made from nothing has no episodes behind it, but it is still
// traceable: the reference names the hypothesis.
func TestIntegrationHypothesisWithoutEvidenceIsStillTraceable(t *testing.T) {
	e := newEnv(t, nil)
	h, err := e.b.CreateHypothesis(e.ctx, e.nsID, "deploys are slower on mondays", "compare the deploy logs", 0.5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.UpdateHypothesisStatus(e.ctx, h.ID, "testing"); err != nil {
		t.Fatal(err)
	}
	_, fact, err := e.b.ConfirmHypothesis(e.ctx, h.ID)
	if err != nil {
		t.Fatalf("ConfirmHypothesis: %v", err)
	}
	o := e.factOrigin(t, fact.ID)
	if str(o.ref) != "hypothesis:"+itoa(h.ID) || len(o.sources) != 0 {
		t.Fatalf("ref = %s sources = %v; want the hypothesis reference and no episodes", str(o.ref), o.sources)
	}
}

func TestIntegrationFailurePatternFactRecordsItsFailuresAndEpisodes(t *testing.T) {
	e := newEnv(t, nil)
	f1, err := e.b.CreateFailure(e.ctx, e.nsID, "restarted the worker during a deploy", "jobs were lost", "drain first", nil)
	if err != nil {
		t.Fatal(err)
	}
	f2, err := e.b.CreateFailure(e.ctx, e.nsID, "restarted the api during a deploy", "requests dropped", "drain first", nil)
	if err != nil {
		t.Fatal(err)
	}
	ep := e.rememberEpisode(t, "we restart services in the middle of a deploy again")
	e.r.failurePats = []*reasoner.FailurePatternResult{
		{Type: "pattern", PatternFact: "services must be drained before a deploy restarts them", Confidence: 0.8},
	}

	res, err := e.b.Consolidate(e.ctx, e.ns)
	if err != nil {
		t.Fatal(err)
	}
	if res.FailurePatternsFound != 1 {
		t.Fatalf("FailurePatternsFound = %d, want 1: %+v", res.FailurePatternsFound, res)
	}

	var factID int64
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT id FROM facts WHERE namespace_id = $1 AND origin = 'failure_pattern'`, e.nsID).Scan(&factID); err != nil {
		t.Fatalf("no failure_pattern fact: %v", err)
	}
	o := e.factOrigin(t, factID)
	want := "failure:" + itoa(f1.ID) + "," + itoa(f2.ID)
	if str(o.ref) != want {
		t.Fatalf("ref = %s, want %s", str(o.ref), want)
	}
	found := false
	for _, s := range o.sources {
		if s == ep {
			found = true
		}
	}
	if !found {
		t.Fatalf("sources = %v; want the episode the pattern was read from (%d)", o.sources, ep)
	}
}

// Recall hands the origin back with the fact, so a reader can tell a fact read
// out of episodes from one a hypothesis was confirmed into.
func TestIntegrationRecallReturnsTheOriginOfAFact(t *testing.T) {
	e := newEnv(t, nil)
	h, err := e.b.CreateHypothesis(e.ctx, e.nsID, "deploys are slower on mondays", "compare the deploy logs", 0.5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.UpdateHypothesisStatus(e.ctx, h.ID, "testing"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.b.ConfirmHypothesis(e.ctx, h.ID); err != nil {
		t.Fatal(err)
	}

	res, err := e.b.Recall(e.ctx, []string{e.ns}, "deploys are slower on mondays", 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Type == "fact" && r.Content == "deploys are slower on mondays" {
			if r.Origin != "hypothesis" || r.OriginRef != "hypothesis:"+itoa(h.ID) {
				t.Fatalf("origin = %q ref = %q; want the hypothesis and its id", r.Origin, r.OriginRef)
			}
			return
		}
	}
	t.Fatalf("the confirmed fact was not recalled: %+v", res)
}
