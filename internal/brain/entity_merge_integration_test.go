package brain

import (
	"errors"
	"testing"
)

func (e *testEnv) liveEdges(t *testing.T) int {
	t.Helper()
	var n int
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT count(*) FROM relationships WHERE namespace_id = $1 AND deleted_at IS NULL`, e.nsID).Scan(&n); err != nil {
		t.Fatalf("count edges: %v", err)
	}
	return n
}

func (e *testEnv) addEdge(t *testing.T, from, typ, to string, conf float32) {
	t.Helper()
	if _, err := e.b.pool.Exec(e.ctx,
		`INSERT INTO relationships (namespace_id, from_entity, relation_type, to_entity, confidence)
		 VALUES ($1, $2, $3, $4, $5)`, e.nsID, from, typ, to, conf); err != nil {
		t.Fatalf("insert edge: %v", err)
	}
}

// A dry run is the confirmation dialog's source of numbers, so it must report
// exactly what the real run does and change nothing itself.
func TestIntegrationMergeEntitiesDryRunMatchesRealRun(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "the gateway handles auth", nil)
	if _, err := e.b.pool.Exec(e.ctx, `UPDATE facts SET entity = 'api-gateway' WHERE id = $1`, factID); err != nil {
		t.Fatal(err)
	}
	e.addEdge(t, "api-gateway", "uses", "Redis", 0.9)
	e.addEdge(t, "API Gateway", "uses", "Redis", 0.6)           // becomes a duplicate
	e.addEdge(t, "API Gateway", "talks_to", "api-gateway", 0.7) // becomes a self-loop
	e.addEdge(t, "Billing", "calls", "api-gateway", 0.8)

	ns := []string{e.ns}
	dry, err := e.b.MergeEntities(e.ctx, ns, "API Gateway", []string{"api-gateway"}, true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if e.liveEdges(t) != 4 {
		t.Fatalf("a dry run must change nothing, live edges = %d", e.liveEdges(t))
	}
	var ent string
	if err := e.b.pool.QueryRow(e.ctx, `SELECT entity FROM facts WHERE id = $1`, factID).Scan(&ent); err != nil || ent != "api-gateway" {
		t.Fatalf("fact entity after dry run = %q, %v", ent, err)
	}

	real, err := e.b.MergeEntities(e.ctx, ns, "API Gateway", []string{"api-gateway"}, false)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if dry.Facts != real.Facts || dry.Relations != real.Relations || dry.Duplicates != real.Duplicates || dry.SelfLoops != real.SelfLoops {
		t.Fatalf("dry run %+v differs from real run %+v", dry, real)
	}
	if real.Facts != 1 || real.SelfLoops != 1 || real.Duplicates != 1 {
		t.Fatalf("want 1 fact, 1 self-loop, 1 duplicate, got %+v", real)
	}
	if err := e.b.pool.QueryRow(e.ctx, `SELECT entity FROM facts WHERE id = $1`, factID).Scan(&ent); err != nil || ent != "API Gateway" {
		t.Fatalf("fact entity after merge = %q, %v", ent, err)
	}
	// The more confident of the two identical edges survives, plus Billing's.
	if e.liveEdges(t) != 2 {
		t.Fatalf("want 2 live edges after the merge, got %d", e.liveEdges(t))
	}
	var best float32
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT max(confidence) FROM relationships WHERE namespace_id = $1 AND deleted_at IS NULL AND to_entity = 'Redis'`, e.nsID).Scan(&best); err != nil || best != 0.9 {
		t.Fatalf("surviving Redis edge confidence = %v, %v", best, err)
	}
}

func TestIntegrationMergeEntitiesRejectsEmptyInput(t *testing.T) {
	e := newEnv(t, nil)
	for _, tc := range []struct {
		keep     string
		variants []string
	}{
		{"", []string{"a"}},
		{"  ", []string{"a"}},
		{"keep", nil},
		{"keep", []string{" "}},
	} {
		if _, err := e.b.MergeEntities(e.ctx, []string{e.ns}, tc.keep, tc.variants, true); !errors.Is(err, ErrMergeInvalid) {
			t.Errorf("keep=%q variants=%q: want ErrMergeInvalid, got %v", tc.keep, tc.variants, err)
		}
	}
}
