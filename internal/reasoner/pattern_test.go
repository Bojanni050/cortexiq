// Modifications copyright (c) 2026 Bojan van der Heide
package reasoner

import (
	"context"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/models"
)

func patternFacts() ([]models.Fact, []models.Relationship) {
	facts := []models.Fact{
		{ID: 1, Content: "a", Confidence: 0.8},
		{ID: 2, Content: "b", Confidence: 0.8},
	}
	rels := []models.Relationship{{ID: 3, FromEntity: "a", RelationType: "requires", ToEntity: "b", Confidence: 0.8}}
	return facts, rels
}

// A candidate the model itself scores at 0.0 coherence says the sources only
// share a subject. That is not a recurrence, and turning the verdict into 0.5 is
// how a description gets stored as a pattern — production did exactly that.
func TestReasonPatternsDropsZeroCoherence(t *testing.T) {
	srv, _ := triageServer(t, `{"pattern": "mnemoria requires a CLI", "coherence": 0.0, "source_facts": [1, 2], "source_rels": [3]}`)
	r := newTriage(t, srv.URL)
	facts, rels := patternFacts()

	got, err := r.ReasonPatterns(context.Background(), facts, rels)
	if err != nil {
		t.Fatalf("ReasonPatterns: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want no pattern, got %+v", got[0])
	}
}

// One candidate sometimes arrives as the element itself instead of an array
// holding it. It must still count.
func TestReasonPatternsAcceptsASingleObject(t *testing.T) {
	srv, _ := triageServer(t, `{"pattern": "every service sits behind Caddy", "coherence": 0.8, "source_facts": [1, 2], "source_rels": [3]}`)
	r := newTriage(t, srv.URL)
	facts, rels := patternFacts()

	got, err := r.ReasonPatterns(context.Background(), facts, rels)
	if err != nil {
		t.Fatalf("ReasonPatterns: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 pattern, got %d", len(got))
	}
	if got[0].CoherenceScore != 0.8 {
		t.Errorf("coherence = %v, want 0.8", got[0].CoherenceScore)
	}
	if len(got[0].SourceFactIDs) != 2 || len(got[0].SourceRelIDs) != 1 {
		t.Errorf("sources = %v / %v", got[0].SourceFactIDs, got[0].SourceRelIDs)
	}
}

// A missing coherence keeps the middle of the scale, so an older prompt that
// never mentioned the field still yields usable candidates.
func TestReasonPatternsDefaultsMissingCoherence(t *testing.T) {
	srv, _ := triageServer(t, `[{"pattern": "every service sits behind Caddy", "source_facts": [1, 2], "source_rels": [3]}]`)
	r := newTriage(t, srv.URL)
	facts, rels := patternFacts()

	got, err := r.ReasonPatterns(context.Background(), facts, rels)
	if err != nil {
		t.Fatalf("ReasonPatterns: %v", err)
	}
	if len(got) != 1 || got[0].CoherenceScore != 0.5 {
		t.Fatalf("want one candidate at 0.5, got %+v", got)
	}
}

// The threshold is a recurrence over both axes; a candidate resting on facts
// alone is a restatement, not a pattern.
func TestReasonPatternsDropsCandidatesWithoutRelationships(t *testing.T) {
	srv, _ := triageServer(t, `[{"pattern": "x", "coherence": 0.9, "source_facts": [1, 2], "source_rels": []}]`)
	r := newTriage(t, srv.URL)
	facts, rels := patternFacts()

	got, err := r.ReasonPatterns(context.Background(), facts, rels)
	if err != nil {
		t.Fatalf("ReasonPatterns: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want no pattern, got %+v", got[0])
	}
}
