package brain

import (
	"testing"
	"time"
)

// A fact that was replaced since must show up for a point in time inside its
// knowledge interval, and only there.
func TestIntegrationRecallAsOfFindsSupersededFacts(t *testing.T) {
	e := newEnv(t, nil)
	old := e.insertFact(t, "dashboard port is 8080", nil)
	cur := e.insertFact(t, "dashboard port is 8445", nil)

	day := func(d int) time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, time.UTC) }
	if _, err := e.b.pool.Exec(e.ctx,
		"UPDATE facts SET valid_from = $2, valid_until = $3, known_until = $3, created_at = $2 WHERE id = $1", old, day(1), day(10)); err != nil {
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

func factIDs(t *testing.T, e *testEnv, query string, opts RecallOptions) map[int64]bool {
	t.Helper()
	res, err := e.b.RecallWith(e.ctx, []string{e.ns}, query, 10, opts)
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

// as_of is knowledge time, not validity: a fact recorded after the moment it
// describes was not known then, and a fact whose validity has passed was still
// known afterwards. The valid-time question stays expressible with From/To.
func TestIntegrationRecallAsOfIsKnowledgeTime(t *testing.T) {
	e := newEnv(t, nil)
	date := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	id := e.insertFact(t, "acme contract runs through march", nil)
	// Recorded on 1 October about a period that ended on 15 March.
	if _, err := e.b.pool.Exec(e.ctx,
		"UPDATE facts SET valid_from = $2, valid_until = $3, created_at = $4 WHERE id = $1",
		id, date(time.March, 5), date(time.March, 15), date(time.October, 1)); err != nil {
		t.Fatal(err)
	}
	q := "acme contract"

	endOfMarch := date(time.March, 31)
	if got := factIDs(t, e, q, RecallOptions{AsOf: &endOfMarch}); got[id] {
		t.Errorf("as_of before the fact was recorded must not return it, got %v", got)
	}
	afterRecording := date(time.October, 2)
	if got := factIDs(t, e, q, RecallOptions{AsOf: &afterRecording}); !got[id] {
		t.Errorf("as_of after recording must return the fact even though its validity ended, got %v", got)
	}

	// The same moment narrowed by valid time.
	marFrom, marTo := date(time.March, 1), date(time.April, 1)
	if got := factIDs(t, e, q, RecallOptions{AsOf: &afterRecording, From: &marFrom, To: &marTo}); !got[id] {
		t.Errorf("valid in March, known by October: want the fact, got %v", got)
	}
	julFrom, julTo := date(time.July, 1), date(time.August, 1)
	if got := factIDs(t, e, q, RecallOptions{AsOf: &afterRecording, From: &julFrom, To: &julTo}); got[id] {
		t.Errorf("not valid in July: want nothing, got %v", got)
	}
}

// known_from/known_to select every belief held at some point in the window,
// including ones replaced inside it.
func TestIntegrationRecallKnownWindow(t *testing.T) {
	e := newEnv(t, nil)
	date := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	a := e.insertFact(t, "dashboard port is 8080", nil)
	b := e.insertFact(t, "dashboard port is 8445", nil)
	c := e.insertFact(t, "dashboard port is 9000", nil)
	set := func(id int64, created time.Time, knownUntil *time.Time) {
		t.Helper()
		if _, err := e.b.pool.Exec(e.ctx,
			"UPDATE facts SET created_at = $2, valid_from = $2, known_until = $3, valid_until = $3 WHERE id = $1",
			id, created, knownUntil); err != nil {
			t.Fatal(err)
		}
	}
	replaced := date(time.March, 10)
	set(a, date(time.March, 1), &replaced)
	set(b, date(time.March, 10), nil)
	set(c, date(time.May, 1), nil)

	known := func(from, to *time.Time) map[int64]bool {
		return factIDs(t, e, "dashboard port", RecallOptions{KnownFrom: from, KnownTo: to})
	}
	mar1, apr1, mar15, feb1 := date(time.March, 1), date(time.April, 1), date(time.March, 15), date(time.February, 1)

	if got := known(&mar1, &apr1); !got[a] || !got[b] || got[c] {
		t.Errorf("March window: want a and b, got %v", got)
	}
	if got := known(&mar15, &apr1); got[a] || !got[b] || got[c] {
		t.Errorf("late-March window: a was replaced before it, want only b, got %v", got)
	}
	if got := known(&feb1, &mar1); len(got) != 0 {
		t.Errorf("window ending before anything was recorded: want nothing, got %v", got)
	}

	// The listing agrees with recall.
	facts, err := e.b.QueryFactsWindow(e.ctx, []string{e.ns}, nil, nil, nil, nil, nil, &mar1, &apr1, Pagination{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Errorf("QueryFactsWindow March: want 2 facts, got %d", len(facts))
	}

	// as_of is the point form of the same axis; mixing them is an error.
	if _, err := e.b.RecallWith(e.ctx, []string{e.ns}, "dashboard port", 10, RecallOptions{AsOf: &mar15, KnownTo: &apr1}); err == nil {
		t.Error("as_of together with known_to must be rejected")
	}
	if _, err := e.b.RecallWith(e.ctx, []string{e.ns}, "dashboard port", 10, RecallOptions{KnownFrom: &apr1, KnownTo: &mar1}); err == nil {
		t.Error("reversed knowledge window must be rejected")
	}
}
