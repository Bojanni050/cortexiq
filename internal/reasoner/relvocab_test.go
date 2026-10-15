package reasoner

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestCanonicalRelationType(t *testing.T) {
	tests := []struct {
		name, relType, suggested, want string
	}{
		{"reviewed mapping wins over the model", "includes", "uses", "contains"},
		{"spelling is normalised", "Located-In", "", "located_in"},
		{"spaces too", "located in", "", "located_in"},
		{"vocabulary word itself", "depends_on", "", "depends_on"},
		{"model suggestion accepted when valid", "performs_checks_on", "validates", "validates"},
		{"model suggestion normalised", "performs_checks_on", " Validates ", "validates"},
		{"invented suggestion rejected", "performs_checks_on", "inspects_deeply", ""},
		{"free text without suggestion stays NULL", "reachable_at_port", "", ""},
		{"excluded type is not mapped by the table", "works_on", "", ""},
		{"empty", "", "", ""},
	}
	for _, tt := range tests {
		if got := CanonicalRelationType(tt.relType, tt.suggested); got != tt.want {
			t.Errorf("%s: CanonicalRelationType(%q, %q) = %q, want %q", tt.name, tt.relType, tt.suggested, got, tt.want)
		}
	}
}

func TestGroundRelationshipsSetsCanonicalType(t *testing.T) {
	contents := map[int64]string{1: "Bo uses Kilo Code and Trae"}
	got := groundRelationships([]jsonRelationship{
		{FactID: 1, From: "Bo", RelationType: "uses", To: "Kilo Code", Confidence: 0.9},
		{FactID: 1, From: "Bo", RelationType: "tinkers_with", CanonicalType: "uses", To: "Trae", Confidence: 0.9},
		{FactID: 1, From: "Bo", RelationType: "tinkers_with", CanonicalType: "bogus", To: "Trae", Confidence: 0.9},
	}, contents)[1]
	if len(got) != 3 || got[0].CanonicalType != "uses" || got[1].CanonicalType != "uses" || got[2].CanonicalType != "" {
		t.Fatalf("unexpected canonical types: %+v", got)
	}
	if got[0].RelationType != "uses" || got[1].RelationType != "tinkers_with" {
		t.Errorf("relation_type must stay the extractor's own wording: %+v", got)
	}
}

func TestVocabularyIsConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, v := range RelationVocabulary {
		if seen[v] || v != normalizeRelationType(v) {
			t.Errorf("vocabulary entry %q is duplicated or not normalised", v)
		}
		seen[v] = true
	}
	for rt, c := range knownRelationTypes {
		if rt != normalizeRelationType(rt) {
			t.Errorf("known type %q is not normalised", rt)
		}
		if !IsCanonicalRelationType(c) {
			t.Errorf("known type %q maps to %q which is not in the vocabulary", rt, c)
		}
	}
}

// The production backfill script and the Go table are generated from the same
// review; fail when someone edits only one of them.
func TestKnownRelationTypesMatchBackfillScript(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/backfill-canonical-types.sql")
	if err != nil {
		t.Skipf("backfill script not available: %v", err)
	}
	re := regexp.MustCompile(`(?m)^    \('((?:[^']|'')*)', '([a-z_]+)'\)`)
	matches := re.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatal("no mapping rows found in the backfill script")
	}
	fromSQL := map[string]string{}
	for _, m := range matches {
		fromSQL[normalizeRelationType(strings.ReplaceAll(m[1], "''", "'"))] = m[2]
	}
	if len(fromSQL) != len(knownRelationTypes) {
		t.Errorf("script has %d normalised types, Go table has %d", len(fromSQL), len(knownRelationTypes))
	}
	for rt, c := range fromSQL {
		if knownRelationTypes[rt] != c {
			t.Errorf("%q: script says %q, Go table says %q", rt, c, knownRelationTypes[rt])
		}
	}
}
