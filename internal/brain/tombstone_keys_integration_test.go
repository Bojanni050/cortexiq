package brain

// Integration tests for the value key on retractions. Skipped unless
// CORTEXIQ_TEST_DSN is set, same as the rest of the integration suite.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (e *testEnv) retractionKey(t *testing.T, id int64) *string {
	t.Helper()
	var key *string
	if err := e.b.pool.QueryRow(e.ctx, `SELECT value_key FROM retractions WHERE id = $1`, id).Scan(&key); err != nil {
		t.Fatalf("read value_key of retraction %d: %v", id, err)
	}
	return key
}

func TestIntegrationRetractionStoresValueKey(t *testing.T) {
	e := newEnv(t, nil)
	const claim = "the message queue between the services is kafka"
	factID := e.insertFact(t, claim, nil)

	r, err := e.b.RetractFact(e.ctx, factID, "it is nats since March", "test")
	if err != nil {
		t.Fatalf("RetractFact: %v", err)
	}
	want, _ := valueKey(claim)
	got := e.retractionKey(t, r.ID)
	if got == nil || *got != want {
		t.Fatalf("value_key = %v; want %q", got, want)
	}
}

// A claim that normalises to nothing must be stored without a key, not with the
// hash of the empty string.
func TestIntegrationRetractionOfAnEmptyClaimHasNoKey(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "...", nil)

	r, err := e.b.RetractFact(e.ctx, factID, "it was only punctuation", "test")
	if err != nil {
		t.Fatalf("RetractFact: %v", err)
	}
	if got := e.retractionKey(t, r.ID); got != nil {
		t.Fatalf("value_key = %q; want NULL for a claim with nothing to match", *got)
	}
}

func TestIntegrationBackfillValueKeys(t *testing.T) {
	e := newEnv(t, nil)
	const claim = "the message queue between the services is kafka"
	keyed, err := e.b.RetractFact(e.ctx, e.insertFact(t, claim, nil), "old rejection", "test")
	if err != nil {
		t.Fatalf("RetractFact: %v", err)
	}
	empty, err := e.b.RetractFact(e.ctx, e.insertFact(t, "...", nil), "only punctuation", "test")
	if err != nil {
		t.Fatalf("RetractFact (empty): %v", err)
	}

	// A retraction as it looked before migration 00043: no key.
	if _, err := e.b.pool.Exec(e.ctx,
		`UPDATE retractions SET value_key = NULL WHERE id = ANY($1)`, []int64{keyed.ID, empty.ID}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.b.BackfillValueKeys(e.ctx); err != nil {
		t.Fatalf("BackfillValueKeys: %v", err)
	}
	want, _ := valueKey(claim)
	if got := e.retractionKey(t, keyed.ID); got == nil || *got != want {
		t.Fatalf("after backfill value_key = %v; want %q", got, want)
	}
	if got := e.retractionKey(t, empty.ID); got != nil {
		t.Fatalf("after backfill the empty claim got key %q; want it left NULL", *got)
	}

	// Idempotent: a second pass finds nothing left to key for this namespace's
	// rows and changes nothing.
	if _, err := e.b.BackfillValueKeys(e.ctx); err != nil {
		t.Fatalf("second BackfillValueKeys: %v", err)
	}
	if got := e.retractionKey(t, keyed.ID); got == nil || *got != want {
		t.Fatalf("after a second backfill value_key = %v; want %q", got, want)
	}
}

// Both write paths for retractions (retract and keyed invalidate) must go
// through insertRetractionRow, or one of them would write rows without a key.
func TestRetractionsAreInsertedInOnePlace(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "tombstone.go" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(src)), "insert into retractions") {
			t.Errorf("%s inserts into retractions directly; use insertRetractionRow", f)
		}
	}
}
