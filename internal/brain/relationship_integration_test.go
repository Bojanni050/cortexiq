package brain

import (
	"testing"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/Bojanni050/cortexiq/internal/reasoner"
)

func rel(from, typ, canonical, to string, conf float32) *reasoner.StructuredRelationship {
	return &reasoner.StructuredRelationship{FromEntity: from, RelationType: typ, CanonicalType: canonical, ToEntity: to, Confidence: conf}
}

// The same edge stated by several facts, in other casing or wording, is stored once;
// the canonical type is stored next to the extractor's own relation_type.
func TestIntegrationRelationshipsAreDeduplicatedAndCanonical(t *testing.T) {
	e := newEnv(t, nil)
	f1 := e.insertFact(t, "Bo uses Kilo Code", nil)
	f2 := e.insertFact(t, "bo uses kilo code daily", nil)
	f3 := e.insertFact(t, "The kit includes a battery", nil)
	f4 := e.insertFact(t, "The kit contains a battery", nil)
	f5 := e.insertFact(t, "The port is reachable at 8445", nil)
	e.r.rels = map[int64][]*reasoner.StructuredRelationship{
		f1: {rel("Bo", "uses", "uses", "Kilo Code", 0.8)},
		f2: {rel("bo", "uses", "uses", "KILO CODE", 0.95)},
		f3: {rel("kit", "includes", "contains", "battery", 0.8)},
		f4: {rel("kit", "contains", "contains", "battery", 0.8)},
		f5: {rel("port", "reachable_at", "", "8445", 0.8)},
	}

	count, _, errs := e.b.consolidateFactsToRelationships(e.ctx, e.nsID, &models.ConsolidationProgress{})
	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	if count != 3 {
		t.Errorf("created %d relationships, want 3 (uses, contains, reachable_at)", count)
	}

	var n int
	var conf float32
	var src int64
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT count(*), max(confidence), min(source_fact_id) FROM relationships
		 WHERE namespace_id = $1 AND canonical_type = 'uses'`, e.nsID).Scan(&n, &conf, &src); err != nil {
		t.Fatal(err)
	}
	if n != 1 || conf != 0.95 || src != f1 {
		t.Errorf("uses: rows=%d confidence=%v source=%d, want 1 row, confidence 0.95, source %d", n, conf, src, f1)
	}

	var original string
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT relation_type FROM relationships WHERE namespace_id = $1 AND canonical_type = 'contains'`, e.nsID).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if original != "includes" {
		t.Errorf("relation_type = %q, want the first wording %q kept", original, "includes")
	}

	var canonical *string
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT canonical_type FROM relationships WHERE namespace_id = $1 AND relation_type = 'reachable_at'`, e.nsID).Scan(&canonical); err != nil {
		t.Fatal(err)
	}
	if canonical != nil {
		t.Errorf("unmapped relation must keep canonical_type NULL, got %q", *canonical)
	}

	// A second pass over the same facts adds nothing.
	count, _, errs = e.b.consolidateFactsToRelationships(e.ctx, e.nsID, &models.ConsolidationProgress{})
	if len(errs) != 0 || count != 0 {
		t.Errorf("second run: created %d, errors %v; want 0 and none", count, errs)
	}
}
