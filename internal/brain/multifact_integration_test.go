package brain

import "testing"

// One episode that states two things yields two facts, each linked to the episode.
func TestIntegrationOneEpisodeCanYieldSeveralFacts(t *testing.T) {
	e := newEnv(t, nil)
	epID, err := e.b.Remember(e.ctx,
		e.ns, "alpha beta gamma delta epsilon || completely different kubernetes cluster upgrade topic", nil)
	if err != nil {
		t.Fatal(err)
	}

	res, err := e.b.ConsolidateByID(e.ctx, e.nsID)
	if err != nil {
		t.Fatal(err)
	}
	if res.FactsCreated != 2 {
		t.Fatalf("FactsCreated = %d, want 2 (%+v)", res.FactsCreated, res)
	}

	var sources int
	err = e.b.pool.QueryRow(e.ctx,
		`SELECT count(*) FROM fact_sources s JOIN facts f ON f.id = s.fact_id
		 WHERE s.episode_id = $1 AND f.namespace_id = $2`, epID, e.nsID).Scan(&sources)
	if err != nil || sources != 2 {
		t.Errorf("both facts should cite the episode: sources=%d err=%v", sources, err)
	}

	// The episode is done: a second run must not process it again.
	again, err := e.b.ConsolidateByID(e.ctx, e.nsID)
	if err != nil || again.EpisodesRead != 0 || again.FactsCreated != 0 {
		t.Errorf("second run should find nothing: %+v err=%v", again, err)
	}
}

// An episode with nothing durable produces no fact, but is still marked done so it
// is not re-sent to the LLM on every run.
func TestIntegrationEpisodeWithoutFactsIsStillDone(t *testing.T) {
	e := newEnv(t, nil)
	if _, err := e.b.Remember(e.ctx, e.ns, "NOFACT", nil); err != nil {
		t.Fatal(err)
	}

	res, err := e.b.ConsolidateByID(e.ctx, e.nsID)
	if err != nil {
		t.Fatal(err)
	}
	if res.FactsCreated != 0 || res.EpisodesRead != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}

	again, err := e.b.ConsolidateByID(e.ctx, e.nsID)
	if err != nil || again.EpisodesRead != 0 {
		t.Errorf("episode should be done after yielding no facts: %+v err=%v", again, err)
	}
}

// A repeated claim inside one note is folded into the first instead of stored twice.
func TestIntegrationDuplicateFactsInOneCallAreMerged(t *testing.T) {
	e := newEnv(t, nil)
	if _, err := e.b.Remember(e.ctx, e.ns, dedupTextA+" || "+dedupTextA, nil); err != nil {
		t.Fatal(err)
	}
	res, err := e.b.ConsolidateByID(e.ctx, e.nsID)
	if err != nil {
		t.Fatal(err)
	}
	if res.FactsCreated != 1 || res.FactsDeduplicated != 1 {
		t.Errorf("want 1 created and 1 deduplicated, got %+v", res)
	}
}
