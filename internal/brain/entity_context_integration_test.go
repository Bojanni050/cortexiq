package brain

import (
	"errors"
	"testing"
)

// The Clean-up tab lists spellings separately, so the context column must show
// exactly one spelling: facts, relations and episodes of "Hermes agent" must not
// leak in from "hermes-agent".
func TestIntegrationEntityContextIsPerSpelling(t *testing.T) {
	e := newEnv(t, nil)
	mine := e.insertFact(t, "Hermes agent runs the nightly sweep", nil)
	other := e.insertFact(t, "hermes-agent is the same service", nil)
	for id, ent := range map[int64]string{mine: "Hermes agent", other: "hermes-agent"} {
		if _, err := e.b.pool.Exec(e.ctx, `UPDATE facts SET entity = $2 WHERE id = $1`, id, ent); err != nil {
			t.Fatal(err)
		}
	}
	var epMine, epOther int64
	for content, dst := range map[string]*int64{"episode about Hermes agent": &epMine, "episode about hermes-agent": &epOther} {
		if err := e.b.pool.QueryRow(e.ctx,
			`INSERT INTO episodes (namespace_id, content) VALUES ($1, $2) RETURNING id`, e.nsID, content).Scan(dst); err != nil {
			t.Fatalf("insert episode: %v", err)
		}
	}
	for fact, ep := range map[int64]int64{mine: epMine, other: epOther} {
		if _, err := e.b.pool.Exec(e.ctx, `INSERT INTO fact_sources (fact_id, episode_id) VALUES ($1, $2)`, fact, ep); err != nil {
			t.Fatalf("link source: %v", err)
		}
	}
	e.addEdge(t, "Hermes agent", "uses", "Redis", 0.9)
	e.addEdge(t, "hermes-agent", "uses", "Postgres", 0.9)

	got, err := e.b.EntityContext(e.ctx, []string{e.ns}, " Hermes agent ", 0)
	if err != nil {
		t.Fatalf("context: %v", err)
	}
	if len(got.Facts) != 1 || got.Facts[0].ID != mine || got.FactsTotal != 1 {
		t.Fatalf("want only fact %d, got %+v (total %d)", mine, got.Facts, got.FactsTotal)
	}
	if len(got.Relations) != 1 || got.Relations[0].To != "Redis" {
		t.Fatalf("want only the Redis relation, got %+v", got.Relations)
	}
	if len(got.Episodes) != 1 || got.Episodes[0].ID != epMine {
		t.Fatalf("want only episode %d, got %+v", epMine, got.Episodes)
	}

	if _, err := e.b.EntityContext(e.ctx, []string{e.ns}, "  ", 0); !errors.Is(err, ErrEntityNameRequired) {
		t.Fatalf("empty name: want ErrEntityNameRequired, got %v", err)
	}
}
