package reasoner

import (
	"regexp"
	"strings"
)

var relationTypeSeparators = regexp.MustCompile(`[\s\-]+`)

// normalizeRelationType lowercases and turns spaces and hyphens into underscores,
// so "located in", "Located-In" and "located_in" are the same type.
func normalizeRelationType(s string) string {
	return relationTypeSeparators.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "_")
}

var relationVocabularySet = func() map[string]bool {
	m := make(map[string]bool, len(RelationVocabulary))
	for _, v := range RelationVocabulary {
		m[v] = true
	}
	return m
}()

// IsCanonicalRelationType reports whether s is in the fixed vocabulary.
func IsCanonicalRelationType(s string) bool { return relationVocabularySet[s] }

// CanonicalRelationType decides the canonical_type of an extracted relationship, or
// "" when there is no confident answer. Precedence: (1) a hand-reviewed known
// mapping for the extractor's relation_type, (2) the relation_type itself when it
// already is a vocabulary entry, (3) the extractor's own suggestion when it is in
// the vocabulary. Anything else (unknown names, free text) yields "" so the
// canonical_type stays NULL instead of holding a guess.
func CanonicalRelationType(relationType, suggested string) string {
	rt := normalizeRelationType(relationType)
	if c, ok := knownRelationTypes[rt]; ok {
		return c
	}
	if relationVocabularySet[rt] {
		return rt
	}
	if s := normalizeRelationType(suggested); relationVocabularySet[s] && SuggestionAllowed(rt, s) {
		return s
	}
	return ""
}

// The extractor is told to answer null for a negated or reversed relationship, but
// it does not always do so (an audit of production found "lacks" and
// "does_not_include" filed as contains, "used_by" as runs_on, "accessible_at" as
// hosts). These patterns are the deterministic backstop: a suggestion by the model
// is dropped when the wording says it cannot be right. They only apply to the
// model's suggestions, never to the hand-reviewed mapping. scripts/clear-unsafe-
// canonical-types.sql repeats them; TestGuardPatternsMatchCleanupScript keeps the
// two in step.
const (
	// guardNegationPattern: any negation or failure word anywhere in the type.
	guardNegationPattern = `(^|_)(not|no|never|without|cannot|cant|non|nothing|none|null|lack|lacks|lacking|missing|unable|fail|fails|failed)(_|$)`
	// guardPassivePattern: passive ("used_by") and reachability ("accessible_at")
	// wording, whose direction or meaning a vocabulary word does not carry.
	guardPassivePattern = `(_by|_under)$|^(accessible|reachable)`
	// guardInwardPattern: a trailing in/within reverses the direction ("X included_in
	// Y" means Y contains X), which is wrong for a vocabulary word that points outwards.
	// "_of" is left out on purpose: "consisted_of" is a correct contains, "part_of" never
	// conflicts, and the rare "owner_of" is not worth refusing the common case.
	guardInwardPattern = `_(in|within)$`
)

var (
	guardNegation = regexp.MustCompile(guardNegationPattern)
	guardPassive  = regexp.MustCompile(guardPassivePattern)
	guardInward   = regexp.MustCompile(guardInwardPattern)
)

// outwardCanonicalTypes are the vocabulary words that read from -> to as "from does
// something to / holds to". located_in, part_of, derived_from, is_a, runs_on and
// depends_on already point back at the second entity, so "_in" suits them.
var outwardCanonicalTypes = map[string]bool{
	"contains": true, "has": true, "owns": true, "hosts": true, "manages": true, "creates": true,
	"produces": true, "calls": true, "sends": true, "supports": true, "deletes": true, "writes": true,
	"reads": true, "replaces": true, "validates": true, "logs": true, "displays": true, "describes": true,
	"configures": true, "transforms": true, "accepts": true, "implements": true, "affects": true,
	"prevents": true, "targets": true, "prefers": true, "uses": true,
}

// SuggestionAllowed reports whether the extractor's suggested canonical type may be
// kept for a relation of this (normalised) type. false means: leave canonical_type NULL.
func SuggestionAllowed(relationType, suggested string) bool {
	if guardNegation.MatchString(relationType) || guardPassive.MatchString(relationType) {
		return false
	}
	if guardInward.MatchString(relationType) && outwardCanonicalTypes[suggested] {
		return false
	}
	return true
}

// relationVocabularyList is the vocabulary as shown to the extractor.
func relationVocabularyList() string { return strings.Join(RelationVocabulary, ", ") }
