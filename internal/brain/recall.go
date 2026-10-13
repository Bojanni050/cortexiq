package brain

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/pgvector/pgvector-go"
)

// RecallResult is a unified result from semantic and keyword search across episodes and facts.
type RecallResult struct {
	ID          int64   `json:"id"`
	NamespaceID int64   `json:"namespace_id"`
	Content     string  `json:"content"`
	Confidence  float32 `json:"confidence,omitempty"`
	// Score ranks results; higher is better. With hybrid recall it is the
	// reciprocal-rank-fusion score normalised to 0..1 (1 = top of both lists).
	Score float32 `json:"score"`
	// Similarity is the cosine similarity of a vector hit (absent for keyword-only hits).
	Similarity float32 `json:"similarity,omitempty"`
	// Match says how the result was found: "vector", "keyword" or "hybrid" (both).
	Match      string `json:"match,omitempty"`
	Type       string `json:"type"`
	OccurredAt string `json:"occurred_at,omitempty"`
	ValidFrom  string `json:"valid_from,omitempty"`
	CreatedAt  string `json:"created_at"`
}

const (
	// rrfK is the reciprocal-rank-fusion constant; 60 is the value from the
	// original RRF paper and is robust across rankers.
	rrfK = 60
	// maxKeywordTerms bounds the tsquery size for very long queries.
	maxKeywordTerms = 12
	// maxCandidates bounds how many rows each ranker contributes to the fusion.
	maxCandidates = 300
)

// Recall searches episodes and facts across the given namespaces. It always runs a
// semantic (vector) search and, when hybrid recall is enabled, a full-text search
// as well; both rankings are merged with reciprocal rank fusion, so exact terms
// (names, versions, ports) are found even when embeddings miss them.
// Each namespace path matches itself and all descendants. Namespaces is required.
func (b *Brain) Recall(ctx context.Context, namespaces []string, query string, limit int) ([]RecallResult, error) {
	if err := validateContent(query); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	vec, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}

	pgVec := pgvector.NewVector(vec)

	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaces)
	if err != nil {
		return nil, err
	}

	kwQuery := ""
	if b.config.HybridRecall {
		kwQuery = keywordQuery(query)
	}
	candidates := limit
	if kwQuery != "" {
		candidates = limit * 3
		if candidates > maxCandidates {
			candidates = maxCandidates
		}
	}

	// Search facts first (higher quality, consolidated)
	factSQL, factArgs, err := b.queries.RecallFacts(nsIDs, pgVec, candidates)
	if err != nil {
		return nil, fmt.Errorf("build fact query: %w", err)
	}
	factVec, err := b.runRecall(ctx, factSQL, factArgs, "fact")
	if err != nil {
		return nil, fmt.Errorf("query facts: %w", err)
	}

	var factKw []RecallResult
	if kwQuery != "" {
		kwSQL, kwArgs, err := b.queries.RecallFactsKeyword(nsIDs, kwQuery, candidates)
		if err != nil {
			return nil, fmt.Errorf("build fact keyword query: %w", err)
		}
		if factKw, err = b.runRecall(ctx, kwSQL, kwArgs, "fact"); err != nil {
			return nil, fmt.Errorf("keyword query facts: %w", err)
		}
	}
	results := fuse(factVec, factKw, limit, kwQuery != "", b.config.MinRecallSimilarity)

	// Search episodes for remaining slots
	if episodeLimit := limit - len(results); episodeLimit > 0 {
		epCandidates := episodeLimit
		if kwQuery != "" {
			epCandidates = episodeLimit * 3
			if epCandidates > maxCandidates {
				epCandidates = maxCandidates
			}
		}
		epSQL, epArgs, err := b.queries.RecallEpisodes(nsIDs, pgVec, epCandidates)
		if err != nil {
			return nil, fmt.Errorf("build episode query: %w", err)
		}
		epVec, err := b.runRecall(ctx, epSQL, epArgs, "episode")
		if err != nil {
			return nil, fmt.Errorf("query episodes: %w", err)
		}

		var epKw []RecallResult
		if kwQuery != "" {
			kwSQL, kwArgs, err := b.queries.RecallEpisodesKeyword(nsIDs, kwQuery, epCandidates)
			if err != nil {
				return nil, fmt.Errorf("build episode keyword query: %w", err)
			}
			if epKw, err = b.runRecall(ctx, kwSQL, kwArgs, "episode"); err != nil {
				return nil, fmt.Errorf("keyword query episodes: %w", err)
			}
		}
		results = append(results, fuse(epVec, epKw, episodeLimit, kwQuery != "", b.config.MinRecallSimilarity)...)
	}

	// Sort all results by score descending
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	b.touchRecalledFacts(ctx, results)

	return results, nil
}

// touchRecalledFacts records that facts were returned by recall. Decay treats a
// recalled fact as recently seen, so frequently used knowledge does not fade.
// Confidence is deliberately untouched: being asked about is not new evidence.
// Best effort — a failure here must never fail the recall itself.
func (b *Brain) touchRecalledFacts(ctx context.Context, results []RecallResult) {
	var ids []int64
	for _, r := range results {
		if r.Type == "fact" {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	_, _ = b.pool.Exec(ctx,
		`UPDATE facts SET last_recalled_at = now(), recall_count = recall_count + 1
		 WHERE id = ANY($1)`,
		ids,
	)
}

// runRecall executes one recall query and scans its rows. kind is "fact" or
// "episode"; both vector and keyword templates return the same columns per kind.
func (b *Brain) runRecall(ctx context.Context, sql string, args []any, kind string) ([]RecallResult, error) {
	rows, err := b.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RecallResult
	for rows.Next() {
		r := RecallResult{Type: kind}
		var createdAt time.Time
		if kind == "fact" {
			if err := rows.Scan(&r.ID, &r.NamespaceID, &r.Content, &r.Confidence, &createdAt, &r.Score); err != nil {
				return nil, fmt.Errorf("scan fact: %w", err)
			}
		} else {
			var occurredAt time.Time
			if err := rows.Scan(&r.ID, &r.NamespaceID, &r.Content, &occurredAt, &createdAt, &r.Score); err != nil {
				return nil, fmt.Errorf("scan episode: %w", err)
			}
			r.OccurredAt = occurredAt.Format(time.RFC3339)
		}
		r.CreatedAt = createdAt.Format(time.RFC3339)
		out = append(out, r)
	}
	return out, rows.Err()
}

// fuse merges a vector ranking and a keyword ranking (each best-first) with
// reciprocal rank fusion and returns at most limit results. When hybrid is false
// the vector ranking is returned unchanged (cosine score), which keeps
// non-hybrid recall behaving exactly as before. When hybrid is true scores are
// always on the fused 0..1 scale, even if the keyword search found nothing, so
// the scale does not depend on the query.
func fuse(vec, kw []RecallResult, limit int, hybrid bool, minSimilarity float32) []RecallResult {
	if !hybrid {
		for i := range vec {
			vec[i].Similarity = vec[i].Score
			vec[i].Match = "vector"
		}
		return truncate(relevant(vec, minSimilarity), limit)
	}

	type entry struct {
		res   RecallResult
		score float64
	}
	entries := make(map[int64]*entry, len(vec)+len(kw))
	var order []int64

	add := func(list []RecallResult, match string) {
		for rank, r := range list {
			e, ok := entries[r.ID]
			if !ok {
				e = &entry{res: r}
				entries[r.ID] = e
				order = append(order, r.ID)
			}
			e.score += 1.0 / float64(rrfK+rank+1)
			switch {
			case e.res.Match == "":
				e.res.Match = match
			case e.res.Match != match:
				e.res.Match = "hybrid"
			}
			if match == "vector" {
				e.res.Similarity = r.Score
			}
		}
	}
	add(vec, "vector")
	add(kw, "keyword")

	// Best possible score: rank 1 in both lists.
	best := 2.0 / float64(rrfK+1)
	out := make([]RecallResult, 0, len(order))
	for _, id := range order {
		e := entries[id]
		e.res.Score = float32(e.score / best)
		out = append(out, e.res)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return truncate(relevant(out, minSimilarity), limit)
}

// relevant drops results that are only a weak semantic match. A result confirmed
// by the keyword search ("keyword" or "hybrid") always stays; a vector-only result
// must reach minSimilarity (cosine). The cutoff is model dependent, so 0 (or less)
// disables it. Without it recall always returns the top N, relevant or not.
func relevant(rs []RecallResult, minSimilarity float32) []RecallResult {
	if minSimilarity <= 0 {
		return rs
	}
	out := rs[:0:0]
	for _, r := range rs {
		if r.Match != "vector" || r.Similarity >= minSimilarity {
			out = append(out, r)
		}
	}
	return out
}

func truncate(rs []RecallResult, limit int) []RecallResult {
	if len(rs) > limit {
		return rs[:limit]
	}
	return rs
}

// keywordQuery turns free text into a websearch_to_tsquery string that matches
// documents containing ANY of the terms ("a or b or c"): recall queries are
// natural-language questions, so requiring every word would find almost nothing.
// Punctuation is stripped (which also makes the result safe from tsquery syntax),
// single characters are dropped, and terms are deduplicated. Returns "" when no
// usable term remains.
func keywordQuery(q string) string {
	words := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	seen := make(map[string]bool, len(words))
	var terms []string
	for _, w := range words {
		// "or" is also dropped: websearch_to_tsquery reads it as the operator.
		if len([]rune(w)) < 2 || seen[w] || stopwords[w] {
			continue
		}
		seen[w] = true
		terms = append(terms, w)
		if len(terms) == maxKeywordTerms {
			break
		}
	}
	return strings.Join(terms, " or ")
}

// stopwords are Dutch and English function words that match nearly every
// document and would drown out the terms that actually discriminate. Embeddings
// still see the full query; this only trims the keyword side.
var stopwords = func() map[string]bool {
	m := make(map[string]bool)
	for _, w := range strings.Fields(`or the an of to in on at by for with and is are was were be been it its this that
		these those as from what which who how when where why do does did can could should would
		de het een van en in op te dat die dit deze is zijn was waren er ook maar voor met aan om
		bij naar uit als dan wat wie hoe waar wanneer waarom wordt worden heeft hebben`) {
		m[w] = true
	}
	return m
}()
