package brain

// Integration tests for unjudged audit entries: a forget, and an invalidate
// without a reason, leave a record of who withdrew what, and that record blocks
// nothing. Skipped unless CORTEXIQ_TEST_DSN is set, same as the rest of the
// integration suite.

import (
	"errors"
	"testing"
)

const forgetClaim = "the staging database is rebuilt every friday night"

// retractionsFor returns the retraction rows of one episode, oldest first.
func (e *testEnv) retractionsFor(t *testing.T, episodeID int64) []Retraction {
	t.Helper()
	rows, err := e.b.pool.Query(e.ctx,
		`SELECT `+retractionColumns+` FROM retractions
		  WHERE target_type = 'episode' AND target_id = $1 ORDER BY id`, episodeID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []Retraction
	for rows.Next() {
		r, err := scanRetraction(rows)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func (e *testEnv) rememberEpisode(t *testing.T, content string) int64 {
	t.Helper()
	out, err := e.b.RememberWithOutcome(e.ctx, e.ns, content, RememberOptions{})
	if err != nil {
		t.Fatalf("RememberWithOutcome(%q): %v", content, err)
	}
	return out.ID
}

func TestIntegrationForgetLeavesAnUnjudgedEntry(t *testing.T) {
	e := newEnv(t, nil)
	id := e.rememberEpisode(t, forgetClaim)

	if err := e.b.ForgetEpisode(e.ctx, []string{e.ns}, forgetClaim, "test-actor"); err != nil {
		t.Fatalf("ForgetEpisode: %v", err)
	}

	rs := e.retractionsFor(t, id)
	if len(rs) != 1 {
		t.Fatalf("got %d retraction rows, want exactly one audit entry", len(rs))
	}
	r := rs[0]
	if r.Judged || r.RetractedBy != "test-actor" || r.Content != forgetClaim || r.Reason != noJudgement {
		t.Fatalf("entry = %+v; want an unjudged record of who forgot what", r)
	}
	if key := e.retractionKey(t, r.ID); key != nil {
		t.Fatalf("value_key = %q; an unjudged entry must carry no key", *key)
	}
}

// A forget made no claim about the world, so the value may be stored again.
func TestIntegrationForgetDoesNotBlockTheValue(t *testing.T) {
	e := newEnv(t, nil)
	e.rememberEpisode(t, forgetClaim)
	if err := e.b.ForgetEpisode(e.ctx, []string{e.ns}, forgetClaim, "test"); err != nil {
		t.Fatal(err)
	}

	if err := e.tryInsertFact(t, e.nsID, forgetClaim); err != nil {
		t.Fatalf("insert after a forget err = %v; a forget must not tombstone the value", err)
	}
}

func TestIntegrationInvalidateWithoutReasonLeavesAnUnjudgedEntry(t *testing.T) {
	e := newEnv(t, nil)
	if _, err := e.b.RememberWithOutcome(e.ctx, e.ns, forgetClaim, RememberOptions{ExternalKey: "mirror:claim:u1"}); err != nil {
		t.Fatal(err)
	}

	out, err := e.b.InvalidateKeyed(e.ctx, e.ns, "mirror:claim:u1", "", "mirror")
	if err != nil {
		t.Fatalf("InvalidateKeyed: %v", err)
	}
	if out.Retraction == nil || out.Retraction.Judged || out.Retraction.RetractedBy != "mirror" {
		t.Fatalf("retraction = %+v; want an unjudged entry naming the actor", out.Retraction)
	}
	if err := e.tryInsertFact(t, e.nsID, forgetClaim); err != nil {
		t.Fatalf("insert after an unjudged invalidate err = %v; it must block nothing", err)
	}

	// A replay writes nothing new.
	again, err := e.b.InvalidateKeyed(e.ctx, e.ns, "mirror:claim:u1", "", "mirror")
	if err != nil || !again.AlreadyInvalidated || again.Retraction != nil {
		t.Fatalf("replay = %+v, %v; want already_invalidated with no new row", again, err)
	}
	if n := len(e.retractionsFor(t, out.ID)); n != 1 {
		t.Fatalf("%d rows after a replay, want still one", n)
	}
}

func TestIntegrationInvalidateWithReasonIsJudged(t *testing.T) {
	e := newEnv(t, nil)
	if _, err := e.b.RememberWithOutcome(e.ctx, e.ns, forgetClaim, RememberOptions{ExternalKey: "mirror:claim:u2"}); err != nil {
		t.Fatal(err)
	}
	out, err := e.b.InvalidateKeyed(e.ctx, e.ns, "mirror:claim:u2", "the job moved to thursday", "mirror")
	if err != nil {
		t.Fatal(err)
	}
	if out.Retraction == nil || !out.Retraction.Judged {
		t.Fatalf("retraction = %+v; a reason makes it a judgement", out.Retraction)
	}
}

// A forget is not a verdict, so a later retract of the same episode must still
// be possible: the audit entry does not count as "already retracted".
func TestIntegrationRetractAfterForgetIsAllowed(t *testing.T) {
	e := newEnv(t, nil)
	id := e.rememberEpisode(t, forgetClaim)
	if err := e.b.ForgetEpisode(e.ctx, []string{e.ns}, forgetClaim, "test"); err != nil {
		t.Fatal(err)
	}

	r, err := e.b.RetractEpisodeByID(e.ctx, id, "the job runs on thursday", "test")
	if err != nil {
		t.Fatalf("RetractEpisodeByID after a forget: %v", err)
	}
	if !r.Judged {
		t.Fatalf("retraction = %+v; want a judged one", r)
	}
	if rows := e.retractionsFor(t, id); len(rows) != 2 {
		t.Fatalf("%d rows, want the audit entry and the judgement", len(rows))
	}
	if err := e.tryInsertFact(t, e.nsID, forgetClaim); !errors.Is(err, ErrRejectedValue) {
		t.Fatalf("insert after the judgement err = %v; want ErrRejectedValue", err)
	}

	// A second retract is still refused.
	if _, err := e.b.RetractEpisodeByID(e.ctx, id, "again", "test"); !errors.Is(err, ErrAlreadyRetracted) {
		t.Fatalf("second retract err = %v; want ErrAlreadyRetracted", err)
	}
}

func TestIntegrationUnjudgedEntryCannotBeLifted(t *testing.T) {
	e := newEnv(t, nil)
	id := e.rememberEpisode(t, forgetClaim)
	if err := e.b.ForgetEpisode(e.ctx, []string{e.ns}, forgetClaim, "test"); err != nil {
		t.Fatal(err)
	}
	entry := e.retractionsFor(t, id)[0]

	if _, err := e.b.LiftRetraction(e.ctx, entry.ID, "because", "test"); !errors.Is(err, ErrNotJudged) {
		t.Fatalf("lift of an audit entry err = %v; want ErrNotJudged", err)
	}
}

// The backfill reads rows with no key. An unjudged entry has none by design and
// must stay that way, or the next start would turn every forget into a tombstone.
func TestIntegrationBackfillLeavesUnjudgedEntriesAlone(t *testing.T) {
	e := newEnv(t, nil)
	id := e.rememberEpisode(t, forgetClaim)
	if err := e.b.ForgetEpisode(e.ctx, []string{e.ns}, forgetClaim, "test"); err != nil {
		t.Fatal(err)
	}

	if _, err := e.b.BackfillValueKeys(e.ctx); err != nil {
		t.Fatalf("BackfillValueKeys: %v", err)
	}
	entry := e.retractionsFor(t, id)[0]
	if key := e.retractionKey(t, entry.ID); key != nil {
		t.Fatalf("backfill keyed an unjudged entry: %q", *key)
	}
}

// The table itself refuses a key on an unjudged row.
func TestIntegrationUnjudgedRowWithAKeyIsRefused(t *testing.T) {
	e := newEnv(t, nil)
	id := e.rememberEpisode(t, forgetClaim)

	_, err := e.b.pool.Exec(e.ctx,
		`INSERT INTO retractions (namespace_id, target_type, target_id, content, reason, judged, value_key)
		 VALUES ($1, 'episode', $2, $3, $4, false, 'v1:abc')`,
		e.nsID, id, forgetClaim, noJudgement)
	if err == nil {
		t.Fatal("inserted an unjudged row with a value_key; the CHECK should refuse it")
	}
}
