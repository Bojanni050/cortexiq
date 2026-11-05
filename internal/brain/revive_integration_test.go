package brain

// Integration tests for reviving a fact that went quiet: a fact ended only by
// disuse comes back when the same claim is seen again, instead of a second fact
// being created next to it. Skipped unless CORTEXIQ_TEST_DSN is set, same as the
// rest of the integration suite.

import (
	"testing"

	"github.com/Bojanni050/cortexiq/internal/reasoner"
)

const quietClaim = "the nightly backups are copied to the offsite storage box"

// goDormant ends a fact the way the decay sweep does under the split: unused,
// strength under the threshold.
func (e *testEnv) goDormant(t *testing.T, id int64) {
	t.Helper()
	if _, err := e.b.pool.Exec(e.ctx, `UPDATE facts SET strength = 0.05 WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	res, err := e.b.DecayConfidence(e.ctx, e.nsID)
	if err != nil {
		t.Fatal(err)
	}
	if res.FactsExpired != 1 {
		t.Fatalf("expired = %d, want the fact to go dormant", res.FactsExpired)
	}
}

func (e *testEnv) storeAgain(t *testing.T, content string) storeResult {
	t.Helper()
	ep := e.rememberEpisode(t, content+" once more")
	r, err := e.b.storeFact(e.ctx, e.nsID, &reasoner.StructuredFact{Summary: content}, []int64{ep})
	if err != nil {
		t.Fatalf("storeFact: %v", err)
	}
	return r
}

type reviveState struct {
	ended, marked bool
	strength      float32
}

func (e *testEnv) reviveState(t *testing.T, id int64) reviveState {
	t.Helper()
	var s reviveState
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT valid_until IS NOT NULL, expired_by_disuse_at IS NOT NULL, strength FROM facts WHERE id = $1`, id,
	).Scan(&s.ended, &s.marked, &s.strength); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIntegrationDecayMarksADisuseExpiry(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.DecaySplit = true })
	id := e.insertFact(t, quietClaim, nil)
	e.goDormant(t, id)

	if s := e.reviveState(t, id); !s.ended || !s.marked {
		t.Fatalf("state = %+v; want it ended and marked as ended by disuse", s)
	}
}

func TestIntegrationSeeingADormantFactAgainRevivesIt(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.DecaySplit = true })
	id := e.insertFact(t, quietClaim, nil)
	e.goDormant(t, id)

	r := e.storeAgain(t, quietClaim)
	if !r.deduped || r.created {
		t.Fatalf("result = %+v; want it folded into the existing fact, not created", r)
	}

	if s := e.reviveState(t, id); s.ended || s.marked || s.strength != 1 {
		t.Fatalf("state = %+v; want it active again with full strength", s)
	}
	var n int
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT count(*) FROM facts WHERE namespace_id = $1`, e.nsID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("%d facts, want the one that was revived", n)
	}
}

// A fact ended for any other reason stays ended; the same claim is a new fact.
func TestIntegrationAFactEndedByAnotherReasonIsNotRevived(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.DecaySplit = true })
	id := e.insertFact(t, quietClaim, nil)
	if _, err := e.b.pool.Exec(e.ctx,
		`UPDATE facts SET valid_until = now(), known_until = now() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}

	r := e.storeAgain(t, quietClaim)
	if !r.created || r.deduped {
		t.Fatalf("result = %+v; want a new fact, the old one was ended for another reason", r)
	}
	if s := e.reviveState(t, id); !s.ended {
		t.Fatal("the old fact was revived although it was not ended by disuse")
	}
}

// A rejection made while the fact slept still stands: a revival is a way in.
func TestIntegrationARejectionStillBlocksTheRevival(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.DecaySplit = true })
	id := e.insertFact(t, quietClaim, nil)
	e.goDormant(t, id)

	// The same claim is rejected through another row.
	other := e.insertFact(t, quietClaim+" ", nil)
	if _, err := e.b.RetractFact(e.ctx, other, "it moved to a different box", "test"); err != nil {
		t.Fatal(err)
	}

	r := e.storeAgain(t, quietClaim)
	if !r.refused || r.deduped || r.created {
		t.Fatalf("result = %+v; want the revival refused", r)
	}
	if s := e.reviveState(t, id); !s.ended {
		t.Fatal("the dormant fact was revived despite the rejection")
	}
}

// Without the split nothing is marked, so nothing is revived.
func TestIntegrationWithoutTheSplitNothingIsRevived(t *testing.T) {
	e := newEnv(t, nil)
	id := e.insertFact(t, quietClaim, nil)
	if _, err := e.b.pool.Exec(e.ctx, `UPDATE facts SET confidence = 0.05 WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.DecayConfidence(e.ctx, e.nsID); err != nil {
		t.Fatal(err)
	}
	if s := e.reviveState(t, id); !s.ended || s.marked {
		t.Fatalf("state = %+v; want it ended but not marked", s)
	}
	if r := e.storeAgain(t, quietClaim); !r.created {
		t.Fatalf("result = %+v; the default keeps creating a new fact", r)
	}
}
