package reasoner

import (
	"strings"
	"testing"
)

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

func TestStructuredPromptSubjectAndSelfContainedRules(t *testing.T) {
	with := structuredPrompt([]string{"it now supports X"}, "CortexIQ — Go memory server")
	for _, want := range []string{
		"Subject context (what the Events belong to): CortexIQ — Go memory server",
		"Use it ONLY to name the project",
		"understandable on its own",
		"output an empty array: []",
		"- it now supports X",
	} {
		if !strings.Contains(with, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}

	without := structuredPrompt([]string{"x"}, "  ")
	if strings.Contains(without, "Subject context") {
		t.Error("an empty subject must not add a subject block")
	}
	if !strings.Contains(without, "understandable on its own") {
		t.Error("self-contained rule must be present without a subject too")
	}
}

// The pattern prompt is the only thing that separates "a regularity that recurs"
// from "a tidy summary of what several facts say". The stage used to ask for the
// second one without meaning to, and the model obliged.
func TestPatternPromptAsksForRecurrenceNotSummary(t *testing.T) {
	p := patternPrompt("[Fact 1] a", "[Rel 1] a --b--> c")

	for _, want := range []string{
		"regularity across separate items",
		"It is NOT a summary",
		`"These facts together say X about one system"`,
		"Cite at least 2 facts AND at least 1 relationship",
		"coherence is how consistently the cited sources say the same thing",
		"Do not use it as a confidence number",
		"Every noun in the statement must be traceable",
		"If nothing recurs, output: []",
		"Answer with the array itself, not wrapped in an object",
		"[Fact 1] a",
		"[Rel 1] a --b--> c",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("pattern prompt is missing %q", want)
		}
	}
	if strings.Contains(p, "OR at least 2 items in source_rels") {
		t.Error("pattern prompt still allows a pattern without facts (OR rule)")
	}
}

// A pattern has to rest on a recurrence. One fact with many relationships is a
// restatement of connected items; many facts and no relationship is a summary.
func TestPatternSourcesAreEnough(t *testing.T) {
	cases := []struct {
		facts, rels int
		want        bool
	}{
		{2, 1, true},
		{5, 3, true},
		{1, 9, false},
		{9, 0, false},
		{0, 0, false},
	}
	for _, c := range cases {
		if got := patternSourcesAreEnough(c.facts, c.rels); got != c.want {
			t.Errorf("patternSourcesAreEnough(%d facts, %d rels) = %v, want %v", c.facts, c.rels, got, c.want)
		}
	}
}
