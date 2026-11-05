package brain

// Integration tests for the decay split: with CORTEXIQ_DECAY_SPLIT on, disuse
// lowers a fact's strength and leaves its confidence alone; with it off the old
// behaviour stays exactly as it was. Skipped unless CORTEXIQ_TEST_DSN is set,
// same as the rest of the integration suite.

import "testing"

type decayState struct {
	confidence, strength float32
	ended                bool
}

func (e *testEnv) decayState(t *testing.T, id int64) decayState {
	t.Helper()
	var s decayState
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT confidence, strength, valid_until IS NOT NULL FROM facts WHERE id = $1`, id,
	).Scan(&s.confidence, &s.strength, &s.ended); err != nil {
		t.Fatal(err)
	}
	return s
}

// staleFact is a fact nobody has touched for a month.
func (e *testEnv) staleFact(t *testing.T, content string) int64 {
	t.Helper()
	id := e.insertFact(t, content, nil)
	if _, err := e.b.pool.Exec(e.ctx,
		`UPDATE facts SET updated_at = now() - interval '30 days' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestIntegrationDecayDefaultStillLowersConfidence(t *testing.T) {
	e := newEnv(t, nil)
	id := e.staleFact(t, "an old fact that nobody mentions any more")

	if _, err := e.b.DecayConfidence(e.ctx, e.nsID); err != nil {
		t.Fatal(err)
	}
	s := e.decayState(t, id)
	if s.confidence >= 0.8 || s.strength != 1 {
		t.Fatalf("state = %+v; the default must keep decaying confidence and leave strength at 1", s)
	}
}

func TestIntegrationSplitDecayLowersStrengthNotConfidence(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.DecaySplit = true })
	id := e.staleFact(t, "an old fact that nobody mentions any more")

	res, err := e.b.DecayConfidence(e.ctx, e.nsID)
	if err != nil {
		t.Fatal(err)
	}
	s := e.decayState(t, id)
	if s.confidence != 0.8 {
		t.Fatalf("confidence = %v; disuse must not touch it", s.confidence)
	}
	if s.strength >= 1 || res.FactsDecayed != 1 {
		t.Fatalf("strength = %v decayed = %d; want it lowered", s.strength, res.FactsDecayed)
	}
}

// A true fact with little support is not the same as a fact that fell out of use.
// Under the split, low confidence alone does not end a fact.
func TestIntegrationSplitDecayExpiresOnStrengthNotOnConfidence(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.DecaySplit = true })
	weak := e.insertFact(t, "a fact with little support but in daily use", nil)
	faded := e.insertFact(t, "a fact that faded out of use entirely", nil)
	if _, err := e.b.pool.Exec(e.ctx,
		`UPDATE facts SET confidence = 0.05 WHERE id = $1`, weak); err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.pool.Exec(e.ctx,
		`UPDATE facts SET strength = 0.05 WHERE id = $1`, faded); err != nil {
		t.Fatal(err)
	}

	res, err := e.b.DecayConfidence(e.ctx, e.nsID)
	if err != nil {
		t.Fatal(err)
	}
	if e.decayState(t, weak).ended {
		t.Error("a low-confidence fact in use was ended; under the split only disuse ends a fact")
	}
	if !e.decayState(t, faded).ended || res.FactsExpired != 1 {
		t.Errorf("the faded fact was not ended (expired = %d)", res.FactsExpired)
	}
}

func TestIntegrationDefaultDecayStillExpiresOnConfidence(t *testing.T) {
	e := newEnv(t, nil)
	weak := e.insertFact(t, "a fact with little support", nil)
	if _, err := e.b.pool.Exec(e.ctx,
		`UPDATE facts SET confidence = 0.05 WHERE id = $1`, weak); err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.DecayConfidence(e.ctx, e.nsID); err != nil {
		t.Fatal(err)
	}
	if !e.decayState(t, weak).ended {
		t.Fatal("the default must keep expiring on confidence")
	}
}

// Seeing a fact again is what brings its strength back.
func TestIntegrationReinforcementRestoresStrength(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.DecaySplit = true })
	id := e.insertFact(t, "the nightly backups go to the offsite storage box", nil)
	if _, err := e.b.pool.Exec(e.ctx, `UPDATE facts SET strength = 0.4 WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}

	ep := e.rememberEpisode(t, "the nightly backups go to the offsite storage box again")
	e.b.reinforceFact(e.ctx, id, []int64{ep})

	s := e.decayState(t, id)
	if s.strength != 1 || s.confidence <= 0.8 {
		t.Fatalf("state = %+v; want strength back to 1 and confidence nudged up", s)
	}
}

// Recall returns the strength once a fact has started to fade, and omits it at 1.
func TestIntegrationRecallReturnsStrengthOfAFadingFact(t *testing.T) {
	e := newEnv(t, nil)
	const fading = "the staging database is rebuilt every friday night"
	const fresh = "the cache in front of the database is redis"
	fadingID := e.insertFact(t, fading, nil)
	e.insertFact(t, fresh, nil)
	if _, err := e.b.pool.Exec(e.ctx, `UPDATE facts SET strength = 0.5 WHERE id = $1`, fadingID); err != nil {
		t.Fatal(err)
	}

	res, err := e.b.Recall(e.ctx, []string{e.ns}, "database", 10)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, r := range res {
		switch r.Content {
		case fading:
			seen++
			if r.Strength != 0.5 {
				t.Errorf("fading fact strength = %v, want 0.5", r.Strength)
			}
		case fresh:
			seen++
			if r.Strength != 0 {
				t.Errorf("fresh fact strength = %v, want it omitted", r.Strength)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("recall returned %d of the two facts: %+v", seen, res)
	}
}
