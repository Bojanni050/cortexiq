package reasoner

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseStructuredFactsArray(t *testing.T) {
	out := "```json\n" + `[
  {"entity": "CortexIQ", "property": "interval", "value": "5m", "summary": "CortexIQ consolidates every 5 minutes."},
  {"entity": null, "property": null, "value": null, "summary": "CortexIQ decays facts by 5 percent per 168 hours."}
]` + "\n```"
	facts, err := parseStructuredFacts(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("want 2 facts, got %d: %+v", len(facts), facts)
	}
	if facts[0].Entity != "CortexIQ" || facts[0].Value != "5m" || facts[1].Entity != "" {
		t.Errorf("fields not mapped: %+v %+v", facts[0], facts[1])
	}
}

func TestParseStructuredFactsAcceptsSingleObject(t *testing.T) {
	facts, err := parseStructuredFacts(`{"entity": "CortexIQ", "property": null, "value": null, "summary": "CortexIQ uses pgvector."}`)
	if err != nil || len(facts) != 1 || facts[0].Summary != "CortexIQ uses pgvector." {
		t.Fatalf("single object should still parse: %+v err=%v", facts, err)
	}
}

func TestParseStructuredFactsEmptyMeansNothingDurable(t *testing.T) {
	for _, out := range []string{
		`[]`,
		`{"entity": null, "property": null, "value": null, "summary": null}`,
		`[{"entity": null, "property": null, "value": null, "summary": "   "}]`,
	} {
		facts, err := parseStructuredFacts(out)
		if err != nil || len(facts) != 0 {
			t.Errorf("%s: want no facts, got %+v err=%v", out, facts, err)
		}
	}
}

func TestParseStructuredFactsDedupesAndCaps(t *testing.T) {
	facts, err := parseStructuredFacts(`[{"summary": "Same thing."}, {"summary": "same thing."}, {"summary": "Other."}]`)
	if err != nil || len(facts) != 2 {
		t.Errorf("identical summaries must collapse: %+v err=%v", facts, err)
	}

	var parts []string
	for i := 0; i < maxFactsPerCall+4; i++ {
		parts = append(parts, fmt.Sprintf(`{"summary": "Fact number %d."}`, i))
	}
	facts, err = parseStructuredFacts("[" + strings.Join(parts, ",") + "]")
	if err != nil || len(facts) != maxFactsPerCall {
		t.Errorf("want cap of %d facts, got %d err=%v", maxFactsPerCall, len(facts), err)
	}
}

func TestParseStructuredFactsRejectsGarbage(t *testing.T) {
	for _, out := range []string{``, `I cannot do that.`, `[{"summary": `, `{nope`} {
		if _, err := parseStructuredFacts(out); err == nil {
			t.Errorf("%q: expected an error", out)
		}
	}
}

func TestStructuredPromptAsksForOneFactPerTopic(t *testing.T) {
	p := structuredPrompt([]string{"x"}, "")
	for _, want := range []string{
		"Output ONLY a JSON array",
		"ONE fact for EACH distinct claim or topic",
		"two topics in the Events means two facts",
		"output an empty array: []",
		fmt.Sprintf("At most %d facts", maxFactsPerCall),
		"commands, ports, configuration names",
	} {
		if !strings.Contains(strings.ToLower(p), strings.ToLower(want)) {
			t.Errorf("prompt is missing %q", want)
		}
	}
}
