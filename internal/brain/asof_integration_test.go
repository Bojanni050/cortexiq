package brain

import (
	"testing"
	"time"
)

// A fact that was valid in the past and is expired now must show up for a point
// in time inside its validity window, and only there.
func TestIntegrationRecallAsOfFindsSupersededFacts(t *testing.T) {
	e := newEnv(t, nil)
	old := e.insertFact(t, "dashboard port is 8080", nil)
	cur := e.insertFact(t, "dashboard port is 8445", nil)

	day := func(d int) time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, time.UTC) }
	if _, err := e.b.pool.Exec(e.ctx,
		"UPDATE facts SET valid_from = $2, valid_until = $3, created_at = $2 WHERE id = $1", old, day(1), day(10)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.pool.Exec(e.ctx,
		"UPDATE facts SET valid_from = $2, created_at = $2 WHERE id = $1", cur, day(10)); err != nil {
		t.Fatal(err)
	}

	ids := func(asOf *time.Time) map[int64]bool {
		res, err := e.b.RecallAsOf(e.ctx, []string{e.ns}, "dashboard port", 10, asOf)
		if err != nil {
			t.Fatal(err)
		}
		m := map[int64]bool{}
		for _, r := range res {
			if r.Type == "fact" {
				m[r.ID] = true
			}
		}
		return m
	}

	if got := ids(nil); got[old] || !got[cur] {
		t.Errorf("now: want only the current fact, got %v", got)
	}
	mid := day(5)
	if got := ids(&mid); !got[old] || got[cur] {
		t.Errorf("mid-September: want only the superseded fact, got %v", got)
	}
	before := day(1).Add(-time.Hour)
	if got := ids(&before); len(got) != 0 {
		t.Errorf("before anything existed: want nothing, got %v", got)
	}

	// The listing agrees with recall.
	facts, err := e.b.QueryFactsAsOf(e.ctx, []string{e.ns}, nil, nil, &mid, Pagination{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || facts[0].ID != old {
		t.Errorf("QueryFactsAsOf mid-September: want [%d], got %+v", old, facts)
	}
}

// Looking at the past must not count as using a fact (it would delay decay).
func TestIntegrationRecallAsOfDoesNotTouchFacts(t *testing.T) {
	e := newEnv(t, nil)
	id := e.insertFact(t, "kubernetes cluster upgrade planned", nil)
	asOf := time.Now().Add(time.Hour)
	if _, err := e.b.RecallAsOf(e.ctx, []string{e.ns}, "kubernetes cluster upgrade", 5, &asOf); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := e.b.pool.QueryRow(e.ctx, "SELECT recall_count FROM facts WHERE id = $1", id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("as-of recall must not bump recall_count, got %d", n)
	}
}
