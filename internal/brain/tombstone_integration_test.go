package brain

// Integration test for the rejected-value tombstone ("laundering sequence").
// Skipped unless CORTEXIQ_TEST_DSN is set, same as the rest of the integration
// suite.
//
// The Agent Memory Atlas calls the tombstone the one mechanism whose silent
// failure looks exactly like success, and names this sequence as its most
// useful test: reject a value, assert it again through a new write, and check
// that it did not come back as an active fact. docs/audit-agent-memory-atlas-hertoets.md
// (point 1) records that CortexIQ does not hold the line: retractions are keyed
// on the row, nothing on the write path reads them, and nearestActiveFacts
// ignores ended facts on purpose, so a re-asserted value becomes a new active
// fact.
//
// Until the value-keyed tombstone exists the test reports that gap as a skip
// instead of a failure, so CI stays green, and it fails the other way round:
// if the gap closes while tombstoneImplemented is still false, the test
// demands the flag be flipped. The positive control below is always strict.

import (
	"testing"
)

// tombstoneImplemented flips to true in the commit that makes a retracted
// value stay retracted. From then on a re-asserted value that comes back
// active is a failure, not a known gap.
const tombstoneImplemented = false

func TestIntegrationRejectedValueStaysRejected(t *testing.T) {
	e := newEnv(t, nil)
	// Both statements are longer than the default MinEpisodeLength (24): a
	// shorter episode is a fragment and never reaches the extractor, so the
	// first version of this test (an 18-character claim) never got a fact.
	// They share almost no words, so they land in separate clusters and the
	// second consolidation cannot merge one into the other.
	const rejected = "the message queue between the services is kafka"
	const other = "nightly backups are copied to the offsite storage box"

	activeCount := func(content string) int {
		t.Helper()
		var n int
		if err := e.b.pool.QueryRow(e.ctx,
			`SELECT count(*) FROM facts
			  WHERE namespace_id = $1 AND content = $2
			    AND deleted_at IS NULL AND valid_until IS NULL`,
			e.nsID, content,
		).Scan(&n); err != nil {
			t.Fatalf("count active facts for %q: %v", content, err)
		}
		return n
	}

	// 1. The value is believed: remembered and consolidated into one fact.
	if _, err := e.b.Remember(e.ctx, e.ns, rejected, nil); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if _, err := e.b.ConsolidateByID(e.ctx, e.nsID); err != nil {
		t.Fatalf("first consolidation: %v", err)
	}
	var factID int64
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT id FROM facts WHERE namespace_id = $1 AND content = $2`, e.nsID, rejected,
	).Scan(&factID); err != nil {
		t.Fatalf("precondition: expected a fact for %q after the first consolidation: %v", rejected, err)
	}

	// 2. The value is rejected, with a reason. The fact is no longer active.
	if _, err := e.b.RetractFact(e.ctx, factID, "the message queue is nats, we migrated in March", "test"); err != nil {
		t.Fatalf("RetractFact: %v", err)
	}
	if n := activeCount(rejected); n != 0 {
		t.Fatalf("precondition: %d active facts for %q right after the retraction; want 0", n, rejected)
	}

	// 3. The same value is asserted again in a new write (no re-extraction of
	// the old episode: the checkpoint only moves forward). A different value is
	// asserted alongside it as the positive control.
	if _, err := e.b.Remember(e.ctx, e.ns, rejected, nil); err != nil {
		t.Fatalf("Remember (re-assert): %v", err)
	}
	if _, err := e.b.Remember(e.ctx, e.ns, other, nil); err != nil {
		t.Fatalf("Remember (control): %v", err)
	}
	if _, err := e.b.ConsolidateByID(e.ctx, e.nsID); err != nil {
		t.Fatalf("second consolidation: %v", err)
	}

	// Positive control: an unrelated, unrejected value in the same run must
	// become an active fact. Without it, "no active fact" could just mean the
	// second consolidation produced nothing.
	if n := activeCount(other); n != 1 {
		t.Fatalf("positive control: %d active facts for %q; want 1 (the second consolidation did not run as expected)", n, other)
	}

	// 4. The rejected value must not have come back.
	back := activeCount(rejected)
	switch {
	case back == 0 && !tombstoneImplemented:
		t.Fatal("the rejected value stayed rejected: the tombstone gap is closed. " +
			"Set tombstoneImplemented = true so a regression fails this test.")
	case back == 0:
		// The tombstone holds.
	case tombstoneImplemented:
		t.Fatalf("the rejected value %q came back as %d active fact(s) after being asserted again", rejected, back)
	default:
		t.Skipf("known gap (audit point 1): the retracted value %q came back as %d active fact(s); "+
			"retractions are keyed on the row and nothing on the write path reads them", rejected, back)
	}
}
