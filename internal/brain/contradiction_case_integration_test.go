package brain

import (
	"context"
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

// replacingReasoner answers every contradiction check with a confident replacement
// and counts the calls, so a test can see whether the check was reached at all.
type replacingReasoner struct {
	*fakeReasoner
	contradictionCalls int
}

func (r *replacingReasoner) ReasonContradiction(context.Context, string, string, string, string) (*reasoner.ContradictionResult, error) {
	r.contradictionCalls++
	return &reasoner.ContradictionResult{Classification: reasoner.ClassificationReplacement, Confidence: 0.95}, nil
}

// sameEmbedding gives two facts one embedding, so their similarity is exactly 1.
func (e *testEnv) sameEmbedding(t *testing.T, from, to int64) {
	t.Helper()
	if _, err := e.b.pool.Exec(e.ctx,
		`UPDATE facts SET embedding = (SELECT embedding FROM facts WHERE id = $1) WHERE id = $2`, from, to); err != nil {
		t.Fatal(err)
	}
}

func (e *testEnv) endedFact(t *testing.T, id int64) bool {
	t.Helper()
	var ended bool
	if err := e.b.pool.QueryRow(e.ctx, `SELECT valid_until IS NOT NULL FROM facts WHERE id = $1`, id).Scan(&ended); err != nil {
		t.Fatal(err)
	}
	return ended
}

// With the review on, a close fact that the LLM calls a rewording is no
// contradiction: the older fact stays valid and the contradiction check is skipped.
func TestIntegrationContradictionReviewKeepsRewordedFact(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.ContradictionReviewThreshold = 0.85 })
	rr := &replacingReasoner{fakeReasoner: e.r}
	e.b.reasoner = rr
	e.r.sameAnswer = true

	old := e.insertPropertyFact(t, "Claude data export", "packaging format", "behind one-time 24h download URLs")
	fresh := e.insertPropertyFact(t, "Claude data export", "packaging format", "with one-time 24h download URLs")
	e.sameEmbedding(t, fresh, old)

	e.detect(t, fresh, "Claude data export", "packaging format", "with one-time 24h download URLs")
	if e.endedFact(t, old) {
		t.Error("a reworded fact expired the older one; the review must keep it")
	}
	if rr.contradictionCalls != 0 || e.r.sameCalls != 1 {
		t.Errorf("contradiction calls = %d, same calls = %d; want 0 and 1", rr.contradictionCalls, e.r.sameCalls)
	}
}

// When the LLM says the new fact is not a rewording, the old behaviour is intact.
func TestIntegrationContradictionReviewStillSupersedesRealChange(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.ContradictionReviewThreshold = 0.85 })
	rr := &replacingReasoner{fakeReasoner: e.r}
	e.b.reasoner = rr
	e.r.sameAnswer = false

	old := e.insertPropertyFact(t, "CortexIQ", "version", "v0.1.17")
	fresh := e.insertPropertyFact(t, "CortexIQ", "version", "v0.1.20")
	e.sameEmbedding(t, fresh, old)

	e.detect(t, fresh, "CortexIQ", "version", "v0.1.20")
	if !e.endedFact(t, old) || rr.contradictionCalls != 1 {
		t.Errorf("a real change must still supersede: ended=%v contradiction calls=%d", e.endedFact(t, old), rr.contradictionCalls)
	}
}

// Off by default: no review call, same as before.
func TestIntegrationContradictionReviewOffByDefault(t *testing.T) {
	e := newEnv(t, nil)
	rr := &replacingReasoner{fakeReasoner: e.r}
	e.b.reasoner = rr
	e.r.sameAnswer = true

	old := e.insertPropertyFact(t, "CortexIQ", "version", "v0.1.17")
	fresh := e.insertPropertyFact(t, "CortexIQ", "version", "v0.1.20")
	e.sameEmbedding(t, fresh, old)

	e.detect(t, fresh, "CortexIQ", "version", "v0.1.20")
	if e.r.sameCalls != 0 || !e.endedFact(t, old) {
		t.Errorf("same calls = %d, ended = %v; with the review off the old path must run unchanged", e.r.sameCalls, e.endedFact(t, old))
	}
}
