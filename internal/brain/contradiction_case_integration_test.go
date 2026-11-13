package brain

import (
	"testing"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/Bojanni050/cortexiq/internal/reasoner"
	"github.com/pgvector/pgvector-go"
)

// insertPropertyFact inserts a fact with an entity, a property and a value
// (bypassing consolidation) and returns its ID.
func (e *testEnv) insertPropertyFact(t *testing.T, entity, property, value string) int64 {
	t.Helper()
	content := entity + " " + property + " " + value
	vec, _ := fakeEmbedder{}.Embed(e.ctx, content)
	var id int64
	err := e.b.pool.QueryRow(e.ctx,
		`INSERT INTO facts (namespace_id, content, embedding, embedding_model, confidence, entity, property, value, valid_from)
		 VALUES ($1, $2, $3, 'fake', 0.8, $4, $5, $6, now()) RETURNING id`,
		e.nsID, content, pgvector.NewVector(vec), entity, property, value,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert property fact: %v", err)
	}
	return id
}

// detect runs contradiction detection for a fact as the consolidator would.
func (e *testEnv) detect(t *testing.T, id int64, entity, property, value string) int {
	t.Helper()
	detected, _, err := e.b.DetectContradictions(e.ctx, e.nsID, &models.Fact{ID: id, Entity: &entity, Property: &property, Value: &value})
	if err != nil {
		t.Fatalf("DetectContradictions: %v", err)
	}
	return detected
}

// The extractor quotes names as it finds them, so one entity arrives as "CortexIQ"
// and "cortexiq". An exact match let two facts about it disagree unnoticed.
func TestIntegrationContradictionLookupIgnoresCapitalsAndSpaces(t *testing.T) {
	e := newEnv(t, nil)
	e.insertPropertyFact(t, "CortexIQ", "Status", "alpha")

	fresh := e.insertPropertyFact(t, "  cortexiq ", "status ", "beta")
	if got := e.detect(t, fresh, "  cortexiq ", "status ", "beta"); got != 1 {
		t.Errorf("a fact about the same entity and property in other capitals must be compared with the old one: detected %d, want 1", got)
	}
}

// What the folding must not do: join different entities or different properties,
// or flag two facts that agree.
func TestIntegrationContradictionLookupStaysSpecific(t *testing.T) {
	e := newEnv(t, nil)
	e.insertPropertyFact(t, "CortexIQ", "status", "alpha")

	other := e.insertPropertyFact(t, "Cortex", "status", "beta")
	if got := e.detect(t, other, "Cortex", "status", "beta"); got != 0 {
		t.Errorf("a different entity must not be compared: detected %d, want 0", got)
	}
	prop := e.insertPropertyFact(t, "cortexiq", "version", "beta")
	if got := e.detect(t, prop, "cortexiq", "version", "beta"); got != 0 {
		t.Errorf("a different property must not be compared: detected %d, want 0", got)
	}
	same := e.insertPropertyFact(t, "CORTEXIQ", "STATUS", "alpha")
	if got := e.detect(t, same, "CORTEXIQ", "STATUS", "alpha"); got != 0 {
		t.Errorf("two facts that agree are not a contradiction: detected %d, want 0", got)
	}
}

// A blank entity or property has nothing to compare, spaces included.
func TestIntegrationContradictionLookupSkipsBlankKeys(t *testing.T) {
	e := newEnv(t, nil)
	e.insertPropertyFact(t, "CortexIQ", "status", "alpha")
	id := e.insertPropertyFact(t, "   ", "status", "beta")
	if got := e.detect(t, id, "   ", "status", "beta"); got != 0 {
		t.Errorf("a blank entity must not match anything: detected %d, want 0", got)
	}
}

// storeRelationship is the one write path for relations. A placeholder type must
// not become an edge, a real one must.
func TestIntegrationStoreRelationshipRejectsPlaceholderTypes(t *testing.T) {
	e := newEnv(t, nil)
	fact := e.insertFact(t, "bo prefers summaries", nil)

	count := func() int {
		var n int
		if err := e.b.pool.QueryRow(e.ctx, `SELECT count(*) FROM relationships WHERE namespace_id = $1 AND deleted_at IS NULL`, e.nsID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, typ := range []string{"null", "None", " n/a ", ""} {
		stored, err := e.b.storeRelationship(e.ctx, e.nsID, fact, &reasoner.StructuredRelationship{FromEntity: "Bo", RelationType: typ, ToEntity: "summaries", Confidence: 0.9})
		if err != nil || stored {
			t.Errorf("type %q must be dropped without error: stored=%v err=%v", typ, stored, err)
		}
	}
	if n := count(); n != 0 {
		t.Fatalf("no placeholder edge may be stored, found %d", n)
	}
	stored, err := e.b.storeRelationship(e.ctx, e.nsID, fact, &reasoner.StructuredRelationship{FromEntity: "Bo", RelationType: "prefers", ToEntity: "summaries", Confidence: 0.9})
	if err != nil || !stored || count() != 1 {
		t.Errorf("a real relation must still be stored: stored=%v err=%v count=%d", stored, err, count())
	}
}
