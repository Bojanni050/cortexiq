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
	if s := normalizeRelationType(suggested); relationVocabularySet[s] {
		return s
	}
	return ""
}

// relationVocabularyList is the vocabulary as shown to the extractor.
func relationVocabularyList() string { return strings.Join(RelationVocabulary, ", ") }
