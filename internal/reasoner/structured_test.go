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

func TestParseStructuredFactsAcceptsScalarValue(t *testing.T) {
	// Models sometimes emit a bare number (or boolean) where the schema wants a
	// string; that must not cost us the whole reply.
	facts, err := parseStructuredFacts(`[{"entity": "CortexIQ", "property": "response limit", "value": 612, "summary": "CortexIQ limits assistant responses to 612 characters."}, {"entity": "CortexIQ", "property": "graph recall", "value": true, "summary": "CortexIQ enables graph recall by default."}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("want 2 facts, got %d: %+v", len(facts), facts)
	}
	if facts[0].Value != "612" {
		t.Errorf("numeric value: want %q, got %q", "612", facts[0].Value)
	}
	if facts[1].Value != "true" {
		t.Errorf("boolean value: want %q, got %q", "true", facts[1].Value)
	}
}

func TestParseStructuredFactsRepairsRawNewlinesInStrings(t *testing.T) {
	// A model that writes a multi-line summary without escaping the line break
	// must not cost us the whole reply.
	out := "[{\"entity\": \"CortexIQ\", \"property\": \"summary lines\",\n  \"value\": \"two lines\",\n  \"summary\": \"CortexIQ writes a summary\nacross two lines.\"}]"
	facts, err := parseStructuredFacts(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("want 1 fact, got %d: %+v", len(facts), facts)
	}
	if facts[0].Summary != "CortexIQ writes a summary\nacross two lines." {
		t.Errorf("summary: got %q", facts[0].Summary)
	}
	if facts[0].Property != "summary lines" || facts[0].Value != "two lines" {
		t.Errorf("neighbouring fields must survive the repair: %+v", facts[0])
	}
}

func TestRepairJSONStringsKeepsValidEscapes(t *testing.T) {
	in := `{"summary": "a \"quoted\" word, a tab\t and a backslash \\ end"}`
	out := repairJSONStrings(in)
	if out != in {
		t.Errorf("valid JSON must be left untouched:\nwant %q\ngot  %q", in, out)
	}
	valid := `{"summary": "caf\u00e9 \u0000 \/ done"}`
	if out := repairJSONStrings(valid); out != valid {
		t.Errorf("valid JSON must be left untouched:\nwant %q\ngot  %q", valid, out)
	}
}

func TestParseStructuredFactsRepairsInvalidEscapes(t *testing.T) {
	// A backslash followed by a character JSON never allows (a markdown-escaped
	// backtick, a regex) must keep both of its characters rather than cost us
	// the reply.
	for _, tc := range []struct {
		name string
		out  string
		want string
	}{
		{
			name: "escaped backtick",
			out:  "{\"summary\": \"CortexIQ calls the flag \\`recall\\` in the config.\"}",
			want: "CortexIQ calls the flag \\`recall\\` in the config.",
		},
		{
			name: "regex class",
			out:  "{\"summary\": \"CortexIQ matches \\d+ digits with the pattern \\w+.\"}",
			want: "CortexIQ matches \\d+ digits with the pattern \\w+.",
		},
		{
			name: "escaped single quote",
			out:  "{\"summary\": \"CortexIQ keeps \\'quoted\\' text.\"}",
			want: "CortexIQ keeps \\'quoted\\' text.",
		},
	} {
		facts, err := parseStructuredFacts(tc.out)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if len(facts) != 1 || facts[0].Summary != tc.want {
			t.Errorf("%s: want %q, got %+v", tc.name, tc.want, facts)
		}
	}
}

func TestParseStructuredFactsLeavesWindowsPathToRetry(t *testing.T) {
	// "C:\users\bojan" cannot be told apart from the escapes the model may have
	// meant (\u, \b), so it must not be silently turned into control
	// characters: it stays an error and the retry path owns it.
	_, err := parseStructuredFacts(`{"summary": "CortexIQ reads C:\users\bojan\config."}`)
	if err == nil {
		t.Error("an ambiguous backslash escape must still fail")
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

func TestStructuredPromptRejectsConversationalTurns(t *testing.T) {
	p := structuredPrompt([]string{"2"}, "")
	for _, want := range []string{
		"keep the conversation going",
		"never make the text of a turn the subject of a fact",
		"a conversational turn read as a system feature",
	} {
		if !strings.Contains(strings.ToLower(p), strings.ToLower(want)) {
			t.Errorf("prompt is missing %q", want)
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
