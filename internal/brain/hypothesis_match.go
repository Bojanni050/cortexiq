// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"sort"
	"strings"
	"unicode"

	"github.com/Bojanni050/cortexiq/internal/models"
)

const (
	// matchHypothesisLimit is how many open beliefs one answer may consider.
	// Five is enough to cover a contested topic and short enough that the
	// prompt still reads as evidence rather than a list of everything.
	matchHypothesisLimit = 5
	// matchHypothesisScan bounds the candidate query. Open hypotheses are
	// already capped per namespace (maxOpenInferred for inferred ones), so this
	// only guards against a namespace that accumulated a lot of asserted ones.
	matchHypothesisScan = 200
)

// factMatchWeight and termMatchWeight rank a belief that shares a recalled fact
// far above one that merely shares a word. Sharing a fact is evidence that the
// question is about the same thing; sharing a word can be coincidence.
const (
	factMatchWeight = 100
	termMatchWeight = 10
)

// ReflectHypothesis is an open belief that reflect took into account, so a
// caller can see what the answer was warned about.
type ReflectHypothesis struct {
	ID              int64   `json:"id"`
	NamespaceID     int64   `json:"namespace_id"`
	Content         string  `json:"content"`
	Status          string  `json:"status"`
	Confidence      float32 `json:"confidence"`
	SupportFacts    int     `json:"support_facts"`
	SupportEpisodes int     `json:"support_episodes"`
	CounterOf       *int64  `json:"counter_of,omitempty"`
}

// matchHypotheses picks the open beliefs that bear on a question: any hypothesis
// resting on a fact that recall already returned, plus any whose wording shares
// a distinctive term with the question.
//
// Deliberately lexical rather than semantic. A vector match would need an
// embedding per hypothesis and a second index to keep in step with model
// changes, and the payoff over "shares a fact, or shares a word the question
// bothered to use" is small: recall has done the semantic work already, and the
// reasoner reading the prompt is the final judge of relevance.
func (b *Brain) matchHypotheses(ctx context.Context, nsIDs []int64, question string, recalledFactIDs []int64, limit int) ([]models.Hypothesis, error) {
	if len(nsIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > matchHypothesisLimit {
		limit = matchHypothesisLimit
	}

	rows, err := b.pool.Query(ctx, `
		SELECT id, namespace_id, content, confidence, status, verification_plan, method,
		       confirmed_fact_id, rejection_reason, source_fact_ids, tested_at, confirmed_at, rejected_at,
		       created_at, updated_at, deleted_at, counter_of, supported_by
		FROM hypotheses
		WHERE namespace_id = ANY($1) AND status IN ('proposed', 'testing') AND deleted_at IS NULL
		ORDER BY confidence DESC, id DESC
		LIMIT $2`, nsIDs, matchHypothesisScan)
	if err != nil {
		return nil, err
	}
	candidates, err := scanHypothesisRows(rows)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// Support counts come from the same helper the dashboard uses, so the
	// prompt and the UI never disagree about how well a belief is backed.
	if err := b.attachSourceFacts(ctx, candidates); err != nil {
		return nil, err
	}

	recalled := make(map[int64]struct{}, len(recalledFactIDs))
	for _, id := range recalledFactIDs {
		recalled[id] = struct{}{}
	}
	asked := matchTerms(question)

	type scored struct {
		h     models.Hypothesis
		score int
	}
	var survivors []scored
	for _, h := range candidates {
		score := hypothesisScore(h, asked, recalled)
		if score > 0 {
			survivors = append(survivors, scored{h: h, score: score})
		}
	}
	sort.SliceStable(survivors, func(i, j int) bool {
		if survivors[i].score != survivors[j].score {
			return survivors[i].score > survivors[j].score
		}
		if survivors[i].h.Confidence != survivors[j].h.Confidence {
			return survivors[i].h.Confidence > survivors[j].h.Confidence
		}
		return survivors[i].h.ID > survivors[j].h.ID
	})

	if len(survivors) > limit {
		survivors = survivors[:limit]
	}
	out := make([]models.Hypothesis, 0, len(survivors))
	for _, s := range survivors {
		out = append(out, s.h)
	}
	return out, nil
}

// hypothesisScore is how strongly one belief bears on the question. The weights
// are compared, not added up across calls, so the absolute value only matters
// for ordering.
func hypothesisScore(h models.Hypothesis, asked map[string]struct{}, recalled map[int64]struct{}) int {
	score := 0
	for _, id := range hypothesisFactIDs(h) {
		if _, ok := recalled[id]; ok {
			score += factMatchWeight
		}
	}
	shared := 0
	for term := range matchTerms(h.Content) {
		if _, ok := asked[term]; ok {
			shared++
		}
	}
	score += termMatchWeight * shared
	return score
}

// hypothesisFactIDs is every fact a belief stands on: what it was built from,
// what has since supported it, and the fact it became.
func hypothesisFactIDs(h models.Hypothesis) []int64 {
	seen := make(map[int64]struct{}, len(h.SourceFactIDs)+len(h.SupportedBy)+1)
	var ids []int64
	for _, group := range [][]int64{h.SourceFactIDs, h.SupportedBy} {
		for _, id := range group {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	if h.ConfirmedFactID != nil {
		if _, ok := seen[*h.ConfirmedFactID]; !ok {
			ids = append(ids, *h.ConfirmedFactID)
		}
	}
	return ids
}

// matchTerms splits text into the words worth matching on. Short words are
// dropped, and so are the question words that appear in every sentence — left
// in, "hoe" would make every belief look relevant to every question.
func matchTerms(text string) map[string]struct{} {
	terms := make(map[string]struct{})
	for _, field := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len([]rune(field)) < 4 {
			continue
		}
		if _, stop := matchStopwords[field]; stop {
			continue
		}
		terms[field] = struct{}{}
	}
	return terms
}

// matchStopwords covers the Dutch and English filler that carries no subject.
// It is a filter, not a stemmer: the words kept are the ones a question uses to
// name what it is about.
var matchStopwords = map[string]struct{}{
	"aangaande": {}, "alvast": {}, "andere": {}, "binnen": {}, "buiten": {},
	"daarna": {}, "daarvan": {}, "denken": {}, "dezen": {}, "dezelfde": {},
	"doordat": {}, "eigenlijk": {}, "ergens": {}, "gebeurt": {}, "gewoon": {},
	"hadden": {}, "hebben": {}, "heeft": {}, "hier": {}, "hierbij": {},
	"hoeveel": {}, "iemand": {}, "inderdaad": {}, "inmiddels": {}, "kunnen": {},
	"misschien": {}, "mogelijk": {}, "meer": {}, "mijn": {}, "natuurlijk": {}, "niet": {},
	"omdat": {}, "ondanks": {}, "ongeveer": {}, "ontstaat": {}, "ooit": {},
	"precies": {}, "samen": {}, "simpelweg": {}, "tegen": {}, "tussen": {},
	"uiteindelijk": {}, "vanavond": {}, "vanuit": {}, "veel": {}, "verder": {},
	"volgens": {}, "waarschijnlijk": {}, "wanneer": {}, "waarom": {}, "welke": {},
	"werkt": {}, "worden": {}, "wordt": {}, "zijn": {}, "zonder": {},
	"zouden": {}, "doen": {}, "gaat": {}, "daar": {}, "deze": {},
	"about": {}, "after": {}, "again": {}, "against": {}, "already": {},
	"always": {}, "another": {}, "because": {}, "been": {}, "before": {},
	"being": {}, "between": {}, "could": {}, "does": {}, "doing": {},
	"during": {}, "each": {}, "from": {}, "have": {}, "here": {},
	"into": {}, "just": {}, "like": {}, "made": {}, "make": {},
	"many": {}, "more": {}, "most": {}, "much": {}, "must": {},
	"need": {}, "only": {}, "other": {}, "over": {}, "same": {},
	"should": {}, "some": {}, "such": {}, "than": {}, "that": {},
	"their": {}, "them": {}, "then": {}, "there": {}, "these": {},
	"they": {}, "this": {}, "those": {}, "through": {}, "under": {},
	"until": {}, "very": {}, "want": {}, "well": {}, "were": {},
	"what": {}, "when": {}, "where": {}, "which": {}, "while": {},
	"will": {}, "with": {}, "would": {}, "your": {},
}
