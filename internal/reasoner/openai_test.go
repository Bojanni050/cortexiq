package reasoner

import "testing"

func TestGroundRelationships(t *testing.T) {
	contents := map[int64]string{
		1: "Alice works at TechCorp",
		2: "Bob manages Carol",
	}
	jrels := []jsonRelationship{
		{FactID: 1, From: "Alice", RelationType: "works_at", To: "TechCorp", Confidence: 0.95},
		{FactID: 2, From: "bob", RelationType: "manages", To: "CAROL", Confidence: 0.2},      // case-insensitive, confidence clamped up
		{FactID: 1, From: "Alice", RelationType: "knows", To: "Bob", Confidence: 0.9},        // Bob is not in fact 1
		{FactID: 99, From: "Alice", RelationType: "works_at", To: "TechCorp", Confidence: 1}, // unknown fact
		{FactID: 2, From: "", RelationType: "manages", To: "Carol", Confidence: 1},           // incomplete
		{FactID: 2, From: "Bob", RelationType: "manages", To: "Carol", Confidence: 5},        // clamped down
	}

	got := groundRelationships(jrels, contents)

	if len(got[1]) != 1 || got[1][0].ToEntity != "TechCorp" {
		t.Fatalf("fact 1: got %+v", got[1])
	}
	if len(got[2]) != 2 {
		t.Fatalf("fact 2: want 2 relationships, got %d", len(got[2]))
	}
	if got[2][0].Confidence != 0.7 || got[2][1].Confidence != 1.0 {
		t.Errorf("confidence not clamped: %v, %v", got[2][0].Confidence, got[2][1].Confidence)
	}
	if _, ok := got[99]; ok {
		t.Error("relationship for unknown fact must be dropped")
	}
}
