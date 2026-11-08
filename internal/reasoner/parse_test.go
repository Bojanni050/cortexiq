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

// The model closed its array, added a fence and then kept thinking out loud. The
// whole reply used to be handed to encoding/json, which reported the backtick as
// "invalid character '`' after top-level value" and lost the causal links.
func TestExtractJSONStopsAtTheEndOfTheValue(t *testing.T) {
	output := "```json\n[{\"id\": 3104, \"confidence\": 0.9}]\n```\n\nWait, I noticed in my internal check that something is off."
	if got := extractJSON(output); got != `[{"id": 3104, "confidence": 0.9}]` {
		t.Fatalf("extractJSON = %q", got)
	}

	var got []struct {
		ID         int64   `json:"id"`
		Confidence float64 `json:"confidence"`
	}
	if err := decodeModelJSON(extractJSON(output), &got); err != nil {
		t.Fatalf("decodeModelJSON: %v", err)
	}
	if len(got) != 1 || got[0].ID != 3104 {
		t.Errorf("parsed %+v", got)
	}
}

// A complete answer without commentary must come through untouched.
func TestFirstJSONValueLeavesCleanInputAlone(t *testing.T) {
	for _, in := range []string{`[{"a": 1}]`, `{"a": [1, 2]}`} {
		if got := firstJSONValue(in); got != in {
			t.Errorf("firstJSONValue(%q) = %q", in, got)
		}
	}
}

// A model that was asked for a bare array answered with a complete document
// instead: {"patterns": [...]}. Every patterns run in production failed on it,
// so nothing was stored and the checkpoint never moved.
func TestUnwrapTopLevelArrayFindsTheWrappedArray(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bare array is untouched", `[{"pattern": "a"}]`, `[{"pattern": "a"}]`},
		{"wrapped under the preferred key", `{"patterns": [{"pattern": "a"}]}`, `[{"pattern": "a"}]`},
		{"wrapped and empty", `{"patterns": []}`, `[]`},
		{"another key still counts", `{"items": [{"pattern": "a"}]}`, `[{"pattern": "a"}]`},
		{"preferred key wins", `{"items": [1], "patterns": [2]}`, `[2]`},
		{"no array at all is left alone", `{"error": "none"}`, `{"error": "none"}`},
		// A cut-off answer still yields its array tail, so the element salvage
		// in decodeModelJSON can keep the elements that did arrive.
		{"truncated wrapper yields the tail", `{"patterns": [{"pattern": "a"}`, `[{"pattern": "a"}`},
		// Production: the answer carried an ids array next to the empty result.
		{"an ids array is not the answer", `{"source_facts": [2605, 2606], "patterns": []}`, `[]`},
		{"only an ids array means no answer", `{"source_facts": [2605]}`, `{"source_facts": [2605]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := unwrapTopLevelArray(c.in, "patterns"); got != c.want {
				t.Fatalf("unwrapTopLevelArray(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// The wrapped shape must survive the whole path, repairs included.
func TestWrappedPatternsDecodeAfterUnwrap(t *testing.T) {
	output := "{\"patterns\": [{\"pattern\": \"Every service binds to 127.0.0.1\", \"coherence\": 0. 9, \"source_facts\": [1, 2], \"source_rels\": [3]}]}"
	raw := unwrapTopLevelArray(extractJSON(output), "patterns")
	var got []jsonPattern
	if err := decodeModelJSON(raw, &got); err != nil {
		t.Fatalf("decodeModelJSON: %v", err)
	}
	if len(got) != 1 || got[0].Coherence != 0.9 || len(got[0].SourceFacts) != 2 {
		t.Errorf("parsed %+v", got)
	}
}
