package brain

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func (e *testEnv) knownUntil(t *testing.T, id int64) *time.Time {
	t.Helper()
	var k *time.Time
	if err := e.b.pool.QueryRow(e.ctx, "SELECT known_until FROM facts WHERE id = $1", id).Scan(&k); err != nil {
		t.Fatal(err)
	}
	return k
}

// Superseding a fact ends its knowledge interval at the same instant as its validity.
func TestIntegrationSupersedeSetsKnownUntil(t *testing.T) {
	e := newEnv(t, nil)
	old := e.insertFact(t, "dashboard port is 8080", nil)
	cur := e.insertFact(t, "dashboard port is 8445", nil)

	if err := e.b.autoSupersede(e.ctx, e.nsID, old, cur, "dashboard", "port", "8080", "8445", 0.9); err != nil {
		t.Fatal(err)
	}
	var validUntil *time.Time
	if err := e.b.pool.QueryRow(e.ctx, "SELECT valid_until FROM facts WHERE id = $1", old).Scan(&validUntil); err != nil {
		t.Fatal(err)
	}
	got := e.knownUntil(t, old)
	if got == nil || validUntil == nil || !got.Equal(*validUntil) {
		t.Errorf("superseded fact: known_until = %v, want %v", got, validUntil)
	}
	if k := e.knownUntil(t, cur); k != nil {
		t.Errorf("current fact must stay known, got known_until %v", k)
	}
}

// Decay expiry withdraws the belief, so it ends the knowledge interval too.
func TestIntegrationDecayExpirySetsKnownUntil(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.ExpiryThreshold = 0.95 })
	id := e.insertFact(t, "legacy cron job runs nightly", nil)

	res, err := e.b.DecayConfidence(e.ctx, e.nsID)
	if err != nil {
		t.Fatal(err)
	}
	if res.FactsExpired != 1 {
		t.Fatalf("want 1 expired fact, got %+v", res)
	}
	if e.knownUntil(t, id) == nil {
		t.Error("expired fact must have known_until set")
	}
}

// A caller-supplied valid_until describes the world, not a withdrawn belief:
// it must not end the knowledge interval, in the write path or in the backfill.
func TestIntegrationBackfillOnlyEndsSystemStampedFacts(t *testing.T) {
	e := newEnv(t, nil)
	end := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	userDated := e.insertFact(t, "contract with acme runs until march", &end)
	superseded := e.insertFact(t, "queue limit is 100", nil)
	replacement := e.insertFact(t, "queue limit is 500", nil)
	if err := e.b.autoSupersede(e.ctx, e.nsID, superseded, replacement, "queue", "limit", "100", "500", 0.9); err != nil {
		t.Fatal(err)
	}
	if k := e.knownUntil(t, userDated); k != nil {
		t.Fatalf("caller-dated fact must keep known_until NULL, got %v", k)
	}

	// Reset to the pre-migration state, then run the migration's own UPDATE.
	if _, err := e.b.pool.Exec(e.ctx, "UPDATE facts SET known_until = NULL WHERE namespace_id = $1", e.nsID); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../db/migrations/00030_add_fact_known_until.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	start := strings.Index(sql, "UPDATE facts")
	end2 := strings.Index(sql[start:], ";")
	if start < 0 || end2 < 0 {
		t.Fatal("backfill statement not found in migration")
	}
	if _, err := e.b.pool.Exec(e.ctx, sql[start:start+end2]); err != nil {
		t.Fatal(err)
	}

	if k := e.knownUntil(t, userDated); k != nil {
		t.Errorf("backfill must not touch caller-dated fact, got %v", k)
	}
	if k := e.knownUntil(t, superseded); k == nil {
		t.Error("backfill must end the knowledge interval of a superseded fact")
	}
	if k := e.knownUntil(t, replacement); k != nil {
		t.Errorf("backfill must leave the current fact known, got %v", k)
	}
}

func TestIntegrationExportImportKeepsKnownUntil(t *testing.T) {
	e := newEnv(t, nil)
	old := e.insertFact(t, "backup runs at midnight", nil)
	cur := e.insertFact(t, "backup runs at four", nil)
	if err := e.b.autoSupersede(e.ctx, e.nsID, old, cur, "backup", "schedule", "midnight", "four", 0.9); err != nil {
		t.Fatal(err)
	}
	want := e.knownUntil(t, old)
	if want == nil {
		t.Fatal("setup: superseded fact has no known_until")
	}

	var buf bytes.Buffer
	if _, err := e.b.Export(e.ctx, &buf, []string{e.ns}, nil); err != nil {
		t.Fatal(err)
	}
	data := buf.String()
	if _, err := e.b.PurgeFact(e.ctx, old, PurgeOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.PurgeFact(e.ctx, cur, PurgeOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.Import(e.ctx, strings.NewReader(data), ImportOptions{}); err != nil {
		t.Fatal(err)
	}

	var got *time.Time
	if err := e.b.pool.QueryRow(e.ctx,
		"SELECT known_until FROM facts WHERE namespace_id = $1 AND content = 'backup runs at midnight'", e.nsID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got == nil || !got.Equal(*want) {
		t.Errorf("imported known_until = %v, want %v", got, *want)
	}
}
