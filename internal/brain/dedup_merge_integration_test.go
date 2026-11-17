package brain

// Integration test: merging duplicate facts must not leave relationships pointing
// at a fact that is soft-deleted by the merge. Skipped unless CORTEXIQ_TEST_DSN
// is set, same as the rest of the integration suite.

import "testing"

func TestIntegrationMergeFactsMovesRelationshipSources(t *testing.T) {
	e := newEnv(t, nil)
	keep := e.insertFact(t, "the nightly backups go to the offsite storage box", nil)
	dup := e.insertFact(t, "nightly backups are written to the offsite storage box", nil)
	other := e.insertFact(t, "the cache in front of the database is redis", nil)

	var fromDup, fromOther int64
	for _, r := range []struct {
		src  int64
		to   string
		dest *int64
	}{{dup, "offsite-box", &fromDup}, {other, "redis", &fromOther}} {
		if err := e.b.pool.QueryRow(e.ctx,
			`INSERT INTO relationships (namespace_id, from_entity, relation_type, to_entity, confidence, source_fact_id)
			 VALUES ($1, 'backups', 'stored_in', $2, 0.8, $3) RETURNING id`,
			e.nsID, r.to, r.src).Scan(r.dest); err != nil {
			t.Fatal(err)
		}
	}

	if err := e.b.mergeFacts(e.ctx, dedupeMember{id: keep, confidence: 0.8}, []int64{dup}); err != nil {
		t.Fatal(err)
	}

	source := func(id int64) (src int64) {
		t.Helper()
		if err := e.b.pool.QueryRow(e.ctx,
			`SELECT source_fact_id FROM relationships WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&src); err != nil {
			t.Fatal(err)
		}
		return src
	}
	if got := source(fromDup); got != keep {
		t.Errorf("relationship source = %d, want the surviving fact %d", got, keep)
	}
	if got := source(fromOther); got != other {
		t.Errorf("an unrelated relationship moved: source = %d, want %d", got, other)
	}
}
