// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

// ReflectPattern is one derived regularity the answer was allowed to lean on.
// It is reported apart from the raw memories because it is not evidence: it is
// a statement the consolidator derived over evidence.
type ReflectPattern struct {
	ID         int64   `json:"id"`
	Content    string  `json:"content"`
	Confidence float32 `json:"confidence"`
	Coherence  float32 `json:"coherence"`
	// FromEntity is true when the question named something the regularity is
	// about, which makes it a much stronger match than a shared word.
	FromEntity bool `json:"from_entity"`
}

// patternMatchSQL ranks patterns by two signals: whether the question names an
// entity the regularity covers, and whether its wording shares a term with the
// question. Confidence times coherence breaks the tie, so two equally relevant
// regularities are ordered by how well they are supported.
//
// The entity text is searched as one document rather than compared per entry:
// an entity is often several words ("hindsight mcp setup") and a question names
// one of them.
const patternMatchSQL = `
SELECT p.id, p.content, p.confidence, p.coherence_score,
       to_tsvector('simple', array_to_string(p.source_entities, ' ')) @@ to_tsquery('simple', $2) AS entity_hit
FROM patterns p
WHERE p.namespace_id = ANY($1)
  AND p.deleted_at IS NULL
  AND (to_tsvector('simple', array_to_string(p.source_entities, ' ')) @@ to_tsquery('simple', $2)
       OR to_tsvector('simple', p.content) @@ to_tsquery('simple', $2))
ORDER BY entity_hit DESC, (p.confidence * p.coherence_score) DESC
LIMIT $3`

// matchPatterns finds the regularities worth showing for a question. It returns
// nothing when the question carries no usable term, so a vague question does not
// drag the highest-confidence patterns into every answer.
func (b *Brain) matchPatterns(ctx context.Context, nsIDs []int64, query string, limit int) ([]ReflectPattern, error) {
	if limit <= 0 || len(nsIDs) == 0 {
		return nil, nil
	}
	terms := queryTerms(query)
	if len(terms) == 0 {
		return nil, nil
	}

	rows, err := b.pool.Query(ctx, patternMatchSQL, nsIDs, strings.Join(terms, " | "), limit)
	if err != nil {
		return nil, fmt.Errorf("match patterns: %w", err)
	}
	defer rows.Close()

	var out []ReflectPattern
	for rows.Next() {
		var p ReflectPattern
		if err := rows.Scan(&p.ID, &p.Content, &p.Confidence, &p.Coherence, &p.FromEntity); err != nil {
			return nil, fmt.Errorf("scan pattern match: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// stopWords are dropped from a question before it is matched. The list is short
// on purpose: a word that is only sometimes a stop word ("does", "can") is more
// useful as a term than as noise.
var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "that": true,
	"this": true, "was": true, "are": true, "how": true, "what": true,
	"why": true, "who": true, "when": true, "where": true, "which": true,
	"our": true, "you": true, "does": true, "did": true, "has": true,
	"have": true, "had": true, "but": true, "not": true, "all": true,
	"wat": true, "hoe": true, "waar": true, "wanneer": true, "welke": true,
	"die": true, "dat": true, "een": true, "van": true, "het": true,
	"zijn": true, "wordt": true, "worden": true, "naar": true, "voor": true,
	"met": true, "niet": true, "ook": true, "maar": true, "kan": true,
}

// queryTerms turns a question into the terms a pattern may be matched on. Terms
// shorter than three characters are dropped: they match almost everything.
func queryTerms(query string) []string {
	fields := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	seen := make(map[string]bool, len(fields))
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) < 3 || stopWords[f] || seen[f] {
			continue
		}
		seen[f] = true
		terms = append(terms, f)
		if len(terms) == 12 {
			break
		}
	}
	return terms
}

// formatPatternsForReflect renders regularities for the synthesis prompt. They
// are labelled as derived so the model does not present them as observations.
func formatPatternsForReflect(patterns []ReflectPattern) string {
	if len(patterns) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, p := range patterns {
		fmt.Fprintf(&sb, "%d. [regularity, confidence %.2f] %s\n", i+1, p.Confidence, p.Content)
	}
	return sb.String()
}
