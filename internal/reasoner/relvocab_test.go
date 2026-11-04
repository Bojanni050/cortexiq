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

// Real cases from the production audit of 2026-10-30 (relations the extraction model
// had filed under a vocabulary word although the wording says otherwise) next to
// suggestions that are fine.
func TestSuggestionGuard(t *testing.T) {
	refused := []struct{ relType, suggested string }{
		{"lacks", "contains"},
		{"does_not_include", "contains"},
		{"are not merged", "affects"},
		{"cannot_reach", "affects"},
		{"cannot", "configures"},
		{"used_by", "runs_on"},
		{"is_used_by", "supports"},
		{"managed_by", "manages"},
		{"replaced_by", "replaces"},
		{"released_under", "has"},
		{"accessible_at", "hosts"},
		{"accessible_over", "supports"},
		{"reachable_via", "connects_to"},
		{"included_in", "contains"},
		{"part_of_in", "has"},
		{"null", "contains"},
		{"fails_for", "affects"},
	}
	for _, tt := range refused {
		if got := CanonicalRelationType(tt.relType, tt.suggested); got != "" {
			t.Errorf("CanonicalRelationType(%q, %q) = %q, want it refused (empty)", tt.relType, tt.suggested, got)
		}
	}
	allowed := []struct{ relType, suggested, want string }{
		{"installed_in", "located_in", "located_in"}, // inward word, inward vocabulary word
		{"saved_in", "located_in", "located_in"},
		{"registered_in", "part_of", "part_of"},
		{"applied_to", "transforms", "transforms"},
		{"introduces", "creates", "creates"},
		{"operating_with", "uses", "uses"},
		{"configured_for", "configures", "configures"},
		{"CortexIQ uses", "uses", "uses"},
		{"consisted of", "contains", "contains"}, // "of" is not treated as a reversal
	}
	for _, tt := range allowed {
		if got := CanonicalRelationType(tt.relType, tt.suggested); got != tt.want {
			t.Errorf("CanonicalRelationType(%q, %q) = %q, want %q", tt.relType, tt.suggested, got, tt.want)
		}
	}
	// The hand-reviewed mapping is not subject to the guard.
	if got := CanonicalRelationType("derived_from", ""); got != "derived_from" {
		t.Errorf("reviewed mapping must stay: %q", got)
	}
	if got := CanonicalRelationType("is", "uses"); got != "is_a" {
		t.Errorf("reviewed mapping beats the model: %q", got)
	}
	if !SuggestionAllowed("applies", "transforms") || SuggestionAllowed("never_uses", "uses") {
		t.Error("SuggestionAllowed basics")
	}
}

// scripts/clear-unsafe-canonical-types.sql repeats the guard in SQL; fail when the
// patterns or the outward word list differ from the Go constants.
func TestGuardPatternsMatchCleanupScript(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/clear-unsafe-canonical-types.sql")
	if err != nil {
		t.Skipf("cleanup script not available: %v", err)
	}
	sql := string(raw)
	for name, p := range map[string]string{"negation": guardNegationPattern, "passive": guardPassivePattern, "inward": guardInwardPattern} {
		if !strings.Contains(sql, "~ '"+p+"'") {
			t.Errorf("the %s pattern %q is missing from the cleanup script", name, p)
		}
	}
	for c := range outwardCanonicalTypes {
		if !strings.Contains(sql, "'"+c+"'") {
			t.Errorf("outward type %q missing from the cleanup script", c)
		}
	}
	m := regexp.MustCompile(`canonical_type IN \(([^)]*)\)`).FindStringSubmatch(sql)
	if m == nil || strings.Count(m[1], "'")/2 != len(outwardCanonicalTypes) {
		t.Errorf("the cleanup script lists a different number of outward types than the Go table (%d)", len(outwardCanonicalTypes))
	}
}
