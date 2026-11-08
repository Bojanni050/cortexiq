// Modifications copyright (c) 2026 Bojan van der Heide
package reasoner

import (
	"strings"
	"testing"
)

// The failure this covers: a model answered with `"confidence": 0. 9` and the
// whole batch of facts lost its relationships over one space. The parser has to
// survive that, because asking the same model again often produces the same
// answer.
func TestDecodeModelJSONRepairsSpaceInsideNumbers(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want float64
	}{
		{"after the dot", `{"confidence": 0. 9}`, 0.9},
		{"before the dot", `{"confidence": 0 .5}`, 0.5},
		{"both sides", `{"confidence": 0 . 7}`, 0.7},
		{"negative", `{"confidence": - 1}`, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got struct {
				Confidence float64 `json:"confidence"`
			}
			if err := decodeModelJSON(c.in, &got); err != nil {
				t.Fatalf("decodeModelJSON(%s): %v", c.in, err)
			}
			if got.Confidence != c.want {
				t.Errorf("confidence = %v, want %v", got.Confidence, c.want)
			}
		})
	}
}

// A repair that rewrites strings would corrupt memories: the text of a note may
// legitimately contain "0. 9", and that has to survive byte for byte.
func TestDecodeModelJSONLeavesStringsAlone(t *testing.T) {
	in := `{"note": "version 0. 9 is old", "confidence": 0. 9}`
	var got struct {
		Note       string  `json:"note"`
		Confidence float64 `json:"confidence"`
	}
	if err := decodeModelJSON(in, &got); err != nil {
		t.Fatalf("decodeModelJSON: %v", err)
	}
	if got.Note != "version 0. 9 is old" {
		t.Errorf("note = %q, want it unchanged", got.Note)
	}
	if got.Confidence != 0.9 {
		t.Errorf("confidence = %v, want 0.9", got.Confidence)
	}
}

// Well-formed output must come back untouched: the repair is not a reformatter.
func TestRepairNumberSpacingIsANoOpOnValidJSON(t *testing.T) {
	valid := "{\n  \"a\": 1.5,\n  \"b\": [1, 2, 3],\n  \"c\": \"x  y\"\n}"
	if got := repairNumberSpacing(valid); got != valid {
		t.Errorf("valid JSON was rewritten:\n%q", got)
	}
}

// When the answer cannot be repaired, the error has to say where it broke:
// "parse json" alone made this incident a guessing game.
func TestDecodeModelJSONQuotesTheProblem(t *testing.T) {
	err := decodeModelJSON(`{"a": 1, "b": }`, &struct{}{})
	if err == nil {
		t.Fatal("a malformed answer was accepted")
	}
	if !strings.Contains(err.Error(), "near") {
		t.Errorf("error does not show the neighbourhood: %v", err)
	}
	if !strings.Contains(err.Error(), `b\": }`) {
		t.Errorf("error does not quote the broken part: %v", err)
	}
}

// A caller that wraps the error keeps its own context, and a glitched but
// repairable answer must not surface as a parse failure at all.
func TestParseStructuredFactsSurvivesTheGlitch(t *testing.T) {
	facts, err := parseStructuredFacts(`[{"entity": "CortexIQ", "property": "dedup threshold", "value": "0. 92", "summary": "CortexIQ reviews merges above 0.92."}]`)
	if err != nil {
		t.Fatalf("structured facts: %v", err)
	}
	if len(facts) != 1 || facts[0].Value != "0. 92" {
		t.Errorf("facts = %+v; a value the model wrote as text must survive verbatim", facts)
	}

	if _, err := parseStructuredFacts(`[{"summary": ]`); err == nil || !strings.Contains(err.Error(), "parse json") {
		t.Errorf("an unrepairable answer must keep the caller's prefix, got %v", err)
	}
}

// The relationships parser is the one that actually failed in production, so the
// repair has to be wired into that path and not only into structured facts.
func TestStructParsersAcceptTheSpaceInsideNumber(t *testing.T) {
	// The batch shape the relationships stage decodes into, with the glitch that
	// broke facts 3028-3032.
	var rels []jsonRelationship
	if err := decodeModelJSON(`[{"from": "CortexIQ", "relation": "uses", "to": "pgvector", "confidence": 0. 95}]`, &rels); err != nil {
		t.Fatalf("decode relationships: %v", err)
	}
	if len(rels) != 1 || rels[0].Confidence != 0.95 {
		t.Errorf("relationships = %+v", rels)
	}
}

// A model that runs out of output tokens stops mid-object. Losing the whole batch
// because the last element is incomplete throws away relationships that were
// already written out in full.
func TestDecodeModelJSONSalvagesATruncatedArray(t *testing.T) {
	raw := `[{"fact_id": 3028, "from": "CortexIQ", "relation_type": "uses", "to": "pgvector", "confidence": 0.9},
  {"fact_id": 3031, "from": "Goal log", "relation_type": "belongs_to", "to": "CortexI`
	var got []jsonRelationship
	if err := decodeModelJSON(raw, &got); err != nil {
		t.Fatalf("a truncated array was rejected outright: %v", err)
	}
	if len(got) != 1 || got[0].FactID != 3028 {
		t.Errorf("salvaged %d relationships, want only the complete first one: %+v", len(got), got)
	}
}

// The salvage must not touch a complete array, however it is formatted.
func TestRepairTruncatedArrayLeavesCompleteInputAlone(t *testing.T) {
	for _, valid := range []string{`[]`, `[{"a": 1}]`, "[\n  {\"a\": 1},\n  {\"b\": 2}\n]"} {
		if got := repairTruncatedArray(valid); got != valid {
			t.Errorf("complete array rewritten:\n in: %q\nout: %q", valid, got)
		}
	}
}
