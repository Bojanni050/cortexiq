package brain

// Integration tests for the rebuild report and the checkpoint reset. Skipped
// unless CORTEXIQ_TEST_DSN is set, same as the rest of the integration suite.

import "testing"

func TestIntegrationRebuildReportsTheDerivedLayer(t *testing.T) {
	e := newEnv(t, nil)
	ep := e.rememberEpisode(t, "the nightly backups are copied to the offsite storage box")
	if _, err := e.b.Consolidate(e.ctx, e.ns); err != nil {
		t.Fatal(err)
	}
	orphan := e.insertFact(t, "a fact that rests on nothing a reader can follow", nil)

	rep, err := e.b.Rebuild(e.ctx, e.ns, false, false)
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	if rep.Episodes != 1 || rep.Applied {
		t.Fatalf("report = %+v; want one episode and nothing applied", rep)
	}
	if rep.FactsByOrigin["consolidation"] < 1 || rep.FactsByOrigin["unknown"] != 1 {
		t.Fatalf("by origin = %v; want the consolidated fact and the one without origin", rep.FactsByOrigin)
	}
	if rep.FactsWithoutSource != 1 {
		t.Fatalf("FactsWithoutSource = %d, want 1 (fact %d, episode %d)", rep.FactsWithoutSource, orphan, ep)
	}
	if rep.EpisodesBehind != 1 || rep.CheckpointEpisodeID == 0 {
		t.Fatalf("checkpoint = %d behind = %d; consolidation should have read the episode", rep.CheckpointEpisodeID, rep.EpisodesBehind)
	}
}

// A report alone changes nothing.
func TestIntegrationRebuildWithoutApplyChangesNothing(t *testing.T) {
	e := newEnv(t, nil)
	e.rememberEpisode(t, "the nightly backups are copied to the offsite storage box")
	if _, err := e.b.Consolidate(e.ctx, e.ns); err != nil {
		t.Fatal(err)
	}
	before, _ := e.b.GetOrCreateConsolidationProgress(e.ctx, e.nsID)

	if _, err := e.b.Rebuild(e.ctx, e.ns, false, false); err != nil {
		t.Fatal(err)
	}
	after, _ := e.b.GetOrCreateConsolidationProgress(e.ctx, e.nsID)
	if before.LastEpisodeID != after.LastEpisodeID {
		t.Fatalf("checkpoint moved from %d to %d without --apply", before.LastEpisodeID, after.LastEpisodeID)
	}
}

// Applying resets the checkpoint, and re-reading the same episode neither
// duplicates the fact nor counts as new evidence for it.
func TestIntegrationRebuildReadsEpisodesAgainWithoutDuplicatesOrInflation(t *testing.T) {
	e := newEnv(t, nil)
	e.rememberEpisode(t, "the nightly backups are copied to the offsite storage box")
	if _, err := e.b.Consolidate(e.ctx, e.ns); err != nil {
		t.Fatal(err)
	}
	var factID int64
	var conf0 float32
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT id, confidence FROM facts WHERE namespace_id = $1 ORDER BY id LIMIT 1`, e.nsID).Scan(&factID, &conf0); err != nil {
		t.Fatal(err)
	}

	rep, err := e.b.Rebuild(e.ctx, e.ns, true, false)
	if err != nil || !rep.Applied {
		t.Fatalf("Rebuild(apply) = %+v, %v", rep, err)
	}
	cp, _ := e.b.GetOrCreateConsolidationProgress(e.ctx, e.nsID)
	if cp.LastEpisodeID != 0 {
		t.Fatalf("checkpoint = %d after apply, want 0", cp.LastEpisodeID)
	}

	if _, err := e.b.Consolidate(e.ctx, e.ns); err != nil {
		t.Fatal(err)
	}
	var n int
	var conf1 float32
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT count(*) FROM facts WHERE namespace_id = $1`, e.nsID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT confidence FROM facts WHERE id = $1`, factID).Scan(&conf1); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("%d facts after the rebuild, want the one that was there", n)
	}
	if conf1 != conf0 {
		t.Fatalf("confidence %v -> %v; re-reading the same episode is not new evidence", conf0, conf1)
	}
}

func TestIntegrationRebuildEndsFactsOnRetiredSourcesOnlyWhenAsked(t *testing.T) {
	e := newEnv(t, nil)
	ep := e.rememberEpisode(t, "the nightly backups are copied to the offsite storage box")
	if _, err := e.b.Consolidate(e.ctx, e.ns); err != nil {
		t.Fatal(err)
	}
	if err := e.b.ForgetEpisode(e.ctx, []string{e.ns}, "the nightly backups are copied to the offsite storage box", "test"); err != nil {
		t.Fatal(err)
	}

	rep, err := e.b.Rebuild(e.ctx, e.ns, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.FactsOnRetiredSources < 1 || rep.FactsEnded != 0 {
		t.Fatalf("report = %+v; want the retired-source facts counted and none ended (episode %d)", rep, ep)
	}

	rep, err = e.b.Rebuild(e.ctx, e.ns, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if rep.FactsEnded < 1 {
		t.Fatalf("FactsEnded = %d, want the retired-source facts ended", rep.FactsEnded)
	}
	after, err := e.b.Rebuild(e.ctx, e.ns, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if after.FactsOnRetiredSources != 0 {
		t.Fatalf("%d facts still on retired sources after ending them", after.FactsOnRetiredSources)
	}
}
