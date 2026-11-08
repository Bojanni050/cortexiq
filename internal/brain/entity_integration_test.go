// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"testing"

	"github.com/Bojanni050/cortexiq/internal/reasoner"
)

// The extractor may only quote a name verbatim, so the same product arrives under
// several spellings. They must land on one row.
func TestIntegrationStoreRelationshipFoldsNameVariants(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "The working Claude Desktop setup uses the http-first transport", nil)

	created, err := e.b.storeRelationship(e.ctx, e.nsID, factID, &reasoner.StructuredRelationship{
		FromEntity:   "The working Claude Desktop setup",
		RelationType: "uses_transport",
		ToEntity:     "http-first transport",
		Confidence:   0.8,
	})
	if err != nil || !created {
		t.Fatalf("first relationship: created=%v err=%v", created, err)
	}

	created, err = e.b.storeRelationship(e.ctx, e.nsID, factID, &reasoner.StructuredRelationship{
		FromEntity:   "claude desktop",
		RelationType: "uses_transport",
		ToEntity:     "http-first transport",
		Confidence:   0.9,
	})
	if err != nil {
		t.Fatalf("second relationship: %v", err)
	}
	if created {
		t.Error("a folded variant must reinforce the existing edge, not add one")
	}

	var rows int
	var from string
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT count(*), min(from_entity) FROM relationships WHERE namespace_id = $1 AND deleted_at IS NULL`,
		e.nsID).Scan(&rows, &from); err != nil {
		t.Fatalf("read relationships: %v", err)
	}
	if rows != 1 {
		t.Fatalf("want one edge, got %d", rows)
	}
	if from != "claude desktop" {
		t.Errorf("want the canonical spelling stored, got %q", from)
	}
}

// A relationship between descriptions names nothing and would count as a source
// for every cluster that shares its target.
func TestIntegrationStoreRelationshipDropsDescriptions(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "The problem affects the workload", nil)

	created, err := e.b.storeRelationship(e.ctx, e.nsID, factID, &reasoner.StructuredRelationship{
		FromEntity:   "the problem",
		RelationType: "affects",
		ToEntity:     "the workload",
		Confidence:   0.9,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created {
		t.Error("an edge between two descriptions must not be stored")
	}

	var rows int
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT count(*) FROM relationships WHERE namespace_id = $1`, e.nsID).Scan(&rows); err != nil {
		t.Fatalf("count relationships: %v", err)
	}
	if rows != 0 {
		t.Errorf("want no edges, got %d", rows)
	}
}

// The dry run is the point of the command: a heuristic that rewrites evidence has
// to be readable before it runs.
func TestIntegrationCanonicalizeEntitiesDryRunChangesNothing(t *testing.T) {
	e := newEnv(t, nil)
	insertEdge := func(from, to string) {
		if _, err := e.b.pool.Exec(e.ctx,
			`INSERT INTO relationships (namespace_id, from_entity, relation_type, to_entity, confidence)
			 VALUES ($1, $2, 'uses_transport', $3, 0.8)`, e.nsID, from, to); err != nil {
			t.Fatalf("insert edge %s -> %s: %v", from, to, err)
		}
	}
	insertEdge("Claude Desktop Setup", "http-first transport")
	insertEdge("claude desktop", "http-first transport")
	insertEdge("the problem", "the workload")

	report, err := e.b.CanonicalizeEntities(e.ctx, []int64{e.nsID}, true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if report.Changed != 1 || report.Dropped != 2 {
		t.Fatalf("want 1 rename and 2 dropped names, got %d and %d", report.Changed, report.Dropped)
	}
	if len(report.Pairs) != 1 || report.Pairs[0].New != "claude desktop" {
		t.Fatalf("unexpected mapping: %+v", report.Pairs)
	}

	var rows int
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT count(*) FROM relationships WHERE namespace_id = $1 AND deleted_at IS NULL`,
		e.nsID).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 3 {
		t.Errorf("a dry run must not touch the data, rows are %d", rows)
	}
}

// The real pass folds the variants, drops the descriptions and removes the rows
// that became identical.
func TestIntegrationCanonicalizeEntitiesFoldsAndDrops(t *testing.T) {
	e := newEnv(t, nil)
	insertEdge := func(from, to string) {
		if _, err := e.b.pool.Exec(e.ctx,
			`INSERT INTO relationships (namespace_id, from_entity, relation_type, to_entity, confidence)
			 VALUES ($1, $2, 'uses_transport', $3, 0.8)`, e.nsID, from, to); err != nil {
			t.Fatalf("insert edge %s -> %s: %v", from, to, err)
		}
	}
	insertEdge("Claude Desktop Setup", "http-first transport")
	insertEdge("claude desktop", "http-first transport")
	insertEdge("the problem", "the workload")

	report, err := e.b.CanonicalizeEntities(e.ctx, []int64{e.nsID}, false)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if report.Duplicates != 1 {
		t.Errorf("want the two identical rows folded into one, removed %d", report.Duplicates)
	}

	var rows int
	var from string
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT count(*), min(from_entity) FROM relationships WHERE namespace_id = $1 AND deleted_at IS NULL`,
		e.nsID).Scan(&rows, &from); err != nil {
		t.Fatalf("read: %v", err)
	}
	if rows != 1 {
		t.Fatalf("want one surviving edge, got %d", rows)
	}
	if from != "claude desktop" {
		t.Errorf("want the canonical spelling, got %q", from)
	}
}
