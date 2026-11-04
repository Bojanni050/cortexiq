package brain

// Integration tests for retract and purge. Skipped unless CORTEXIQ_TEST_DSN is
// set, same as the rest of the integration suite.

import (
	"errors"
	"strings"
	"testing"
)

func TestIntegrationRetractFactRecordsAndHides(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "the queue is kafka", nil)

	got, err := e.b.RetractFact(e.ctx, factID, "the queue is redis, we migrated in March", "test")
	if err != nil {
		t.Fatalf("RetractFact: %v", err)
	}
	if got.Content != "the queue is kafka" {
		t.Fatalf("retraction content = %q; want the withdrawn claim", got.Content)
	}
	if got.Reason == "" || got.RetractedBy != "test" {
		t.Fatalf("retraction = %+v; want reason and actor recorded", got)
	}

	f, err := e.b.GetFact(e.ctx, factID)
	if err != nil {
		t.Fatalf("GetFact after retract: %v", err)
	}
	if f.DeletedAt == nil {
		t.Fatal("retracted fact still has no deleted_at; it would come back in recall")
	}

	list, err := e.b.Retractions(e.ctx, []string{e.ns}, 10)
	if err != nil {
		t.Fatalf("Retractions: %v", err)
	}
	if len(list) != 1 || list[0].TargetID != factID {
		t.Fatalf("Retractions = %+v; want exactly the retracted fact", list)
	}

	if _, err := e.b.RetractFact(e.ctx, factID, "again", "test"); !errors.Is(err, ErrAlreadyRetracted) {
		t.Fatalf("second retract err = %v; want ErrAlreadyRetracted", err)
	}
}

func TestIntegrationRetractRequiresReason(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "the deploy runs on fridays", nil)

	if _, err := e.b.RetractFact(e.ctx, factID, "   ", "test"); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("retract without reason err = %v; want ErrReasonRequired", err)
	}
	if _, err := e.b.RetractFact(e.ctx, factID, strings.Repeat("x", maxReasonLen+1), "test"); err == nil {
		t.Fatal("retract with an over-long reason was accepted")
	}
}

func TestIntegrationRetractEpisodeByQuery(t *testing.T) {
	e := newEnv(t, nil)
	const claim = "the staging database lives on the old VPS"
	if _, err := e.b.Remember(e.ctx, e.ns, claim, nil); err != nil {
		t.Fatalf("Remember: %v", err)
	}

	r, err := e.b.RetractEpisode(e.ctx, []string{e.ns}, claim, "it moved to the new VPS in October", "test")
	if err != nil {
		t.Fatalf("RetractEpisode: %v", err)
	}
	if r.TargetType != "episode" || r.TargetID == 0 {
		t.Fatalf("retraction = %+v; want an episode target", r)
	}

	results, err := e.b.Recall(e.ctx, []string{e.ns}, claim, 10)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	for _, res := range results {
		if strings.Contains(res.Content, "old VPS") {
			t.Fatalf("recall still returns the retracted episode: %+v", res)
		}
	}
}

func TestIntegrationRetractEpisodeByID(t *testing.T) {
	e := newEnv(t, nil)
	const claim = "the backup runs at 03:00"
	epID, err := e.b.Remember(e.ctx, e.ns, claim, nil)
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	r, err := e.b.RetractEpisodeByID(e.ctx, epID, "it runs at 03:43", "dashboard")
	if err != nil {
		t.Fatalf("RetractEpisodeByID: %v", err)
	}
	if r.TargetID != epID || r.TargetType != "episode" {
		t.Fatalf("retraction = %+v; want episode #%d", r, epID)
	}
	if r.Content != claim {
		t.Fatalf("retraction content = %q; want the withdrawn claim", r.Content)
	}

	// The dashboard has ids, so the id path must not fall back to a search.
	var deleted bool
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT deleted_at IS NOT NULL FROM episodes WHERE id = $1`, epID).Scan(&deleted); err != nil {
		t.Fatalf("read episode: %v", err)
	}
	if !deleted {
		t.Fatal("episode retracted by id is still live")
	}

	if _, err := e.b.RetractEpisodeByID(e.ctx, 1<<40, "nope", "dashboard"); !errors.Is(err, ErrEpisodeNotFound) {
		t.Fatalf("retract of a missing episode err = %v; want ErrEpisodeNotFound", err)
	}
}

func TestIntegrationPurgeEpisodeLeavesNoTrace(t *testing.T) {
	e := newEnv(t, nil)
	const claim = "the vendor contract renews in January"
	epID, err := e.b.Remember(e.ctx, e.ns, claim, nil)
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	// The embedder caches by text; the episode write may or may not have
	// populated it, so write a row explicitly to prove purge clears it.
	if _, err := e.b.pool.Exec(e.ctx,
		`INSERT INTO embedding_cache (text_hash, model, text, embedding)
		 VALUES ('test-hash', 'fake', $1, (SELECT embedding FROM episodes WHERE id = $2))
		 ON CONFLICT DO NOTHING`, claim, epID); err != nil {
		t.Fatalf("seed embedding cache: %v", err)
	}

	counts, err := e.b.PurgeEpisode(e.ctx, epID, PurgeOptions{Reason: "test cleanup", Actor: "test"})
	if err != nil {
		t.Fatalf("PurgeEpisode: %v", err)
	}
	if counts.Episodes != 1 {
		t.Fatalf("counts = %+v; want one episode", counts)
	}

	if _, err := e.b.GetEpisode(e.ctx, epID); !errors.Is(err, ErrEpisodeNotFound) {
		t.Fatalf("GetEpisode after purge err = %v; want ErrEpisodeNotFound", err)
	}

	var cached int
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT count(*) FROM embedding_cache WHERE text = $1`, claim).Scan(&cached); err != nil {
		t.Fatalf("count cache: %v", err)
	}
	if cached != 0 {
		t.Fatalf("embedding_cache still holds %d copies of the purged text", cached)
	}

	var logged int
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT count(*) FROM purge_log WHERE target_table = 'episodes' AND target_id = $1 AND reason = 'test cleanup'`,
		epID).Scan(&logged); err != nil {
		t.Fatalf("count purge_log: %v", err)
	}
	if logged != 1 {
		t.Fatalf("purge_log rows = %d; want 1", logged)
	}
}

func TestIntegrationPurgeFactWithConfirmedHypothesis(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "the API rate limit is 100 requests per minute", nil)

	if _, err := e.b.pool.Exec(e.ctx,
		`INSERT INTO hypotheses (namespace_id, content, status, confirmed_fact_id, confirmed_at)
		 VALUES ($1, 'the rate limit is 100/min', 'confirmed', $2, now())`, e.nsID, factID); err != nil {
		t.Fatalf("insert hypothesis: %v", err)
	}

	// This is the regression: the reference had no ON DELETE rule, so purging
	// the confirming fact failed with a foreign-key violation.
	if _, err := e.b.PurgeFact(e.ctx, factID, PurgeOptions{Reason: "test", Actor: "test"}); err != nil {
		t.Fatalf("PurgeFact with a confirming hypothesis: %v", err)
	}

	var confirmed *int64
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT confirmed_fact_id FROM hypotheses WHERE namespace_id = $1`, e.nsID,
	).Scan(&confirmed); err != nil {
		t.Fatalf("load hypothesis: %v", err)
	}
	if confirmed != nil {
		t.Fatalf("confirmed_fact_id = %d; want NULL after the fact was purged", *confirmed)
	}
}

func TestIntegrationPurgeDryRunDeletesNothing(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "the office wifi password rotates monthly", nil)

	counts, err := e.b.PurgeFact(e.ctx, factID, PurgeOptions{Reason: "test", DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if counts.Facts != 1 {
		t.Fatalf("dry run counts = %+v; want one fact", counts)
	}
	if _, err := e.b.GetFact(e.ctx, factID); err != nil {
		t.Fatalf("dry run deleted the fact: %v", err)
	}
}

func TestIntegrationPurgeNamespaceRefusesChildren(t *testing.T) {
	e := newEnv(t, nil)
	child := e.ns + "-child"
	if _, err := e.b.CreateNamespace(e.ctx, child, child, ""); err != nil {
		t.Fatalf("CreateNamespace(child): %v", err)
	}

	if _, err := e.b.PurgeNamespace(e.ctx, e.ns, PurgeOptions{Reason: "test", Actor: "test"}); err == nil {
		t.Fatal("purged a namespace that still has children")
	}
	if _, err := e.b.PurgeNamespace(e.ctx, "/", PurgeOptions{Reason: "test"}); err == nil {
		t.Fatal("purged the root namespace")
	}
}
