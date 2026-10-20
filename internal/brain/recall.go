// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/Bojanni050/cortexiq/internal/reranker"
	"github.com/pgvector/pgvector-go"
)

// RecallResult is a unified result from semantic and keyword search across episodes and facts.
type RecallResult struct {
	// ID is the row id inside its own table. Fact ids and episode ids are
	// independent sequences that overlap, so a client must read Type (or the
	// typed field below) before acting on ID: feeding an episode id to
	// DELETE /api/v1/facts/{id} would delete an unrelated fact.
	ID int64 `json:"id"`
	// Exactly one of these is set, naming the same id but saying which table it
	// belongs to, so a client never has to guess.
	FactID      *int64  `json:"fact_id,omitempty"`
	EpisodeID   *int64  `json:"episode_id,omitempty"`
	FailureID   *int64  `json:"failure_id,omitempty"`
	GoalID      *int64  `json:"goal_id,omitempty"`
	NamespaceID int64   `json:"namespace_id"`
	Content     string  `json:"content"`
	Confidence  float32 `json:"confidence,omitempty"`
	// Score ranks results; higher is better. With hybrid recall it is the
	// reciprocal-rank-fusion score normalised to 0..1 (1 = top of both lists).
	Score float32 `json:"score"`
	// Similarity is the cosine similarity of a vector hit (absent for keyword-only
	// hits). It is model dependent and not comparable across queries, so rank and
	// filter on Score: identifiers matching a fact exactly have measured 0.51-0.74
	// here while their Score was 0.81-0.89.
	Similarity float32 `json:"similarity,omitempty"`
	// Match says which recall arm(s) found the result: vector, keyword, graph,
	// temporal, or hybrid when multiple arms contributed.
	Match string `json:"match,omitempty"`
	// Importance (0..1, 0.5 neutral) is what the caller said this memory is worth;
	// it only influences Score when importance weighting is enabled.
	Importance float32 `json:"importance"`
	// Feedback is the net of the callers' verdicts on this fact (+1 helpful,
	// -1 wrong), counting only each principal's latest vote. It is absent when
	// nobody voted or the votes cancel out, and it scales Score by a bounded
	// factor — it never changes Confidence.
	Feedback   int    `json:"feedback,omitempty"`
	Type       string `json:"type"`
	OccurredAt string `json:"occurred_at,omitempty"`
	ValidFrom  string `json:"valid_from,omitempty"`
	CreatedAt  string `json:"created_at"`
	// Trace says how the result got its rank. It is only set by RecallDetailed.
	Trace *ResultTrace `json:"trace,omitempty"`
	// refTime is the moment recency decay counts from: occurred_at for episodes,
	// created_at for facts.
	refTime time.Time
	// Rank bookkeeping for Trace, filled in by fuse and applyBoosts.
	vecRank, kwRank       int
	graphRank             int
	temporalRank          int
	kwScore, graphScore   float32
	fusedScore            float32
	recencyF, importanceF float64
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
	return b.RecallAsOf(ctx, namespaces, query, limit, nil)
}

// RecallAsOf is Recall for a point in time: it answers "what did we know at asOf".
// Facts count when they were valid then (valid_from <= asOf < valid_until, a
// missing valid_from falling back to created_at), so a fact that was later
// superseded or expired is still found for a moment before that. Episodes count
// when they had been stored by then. Recency decay is measured from asOf, and the
// recall is not counted as use of a fact (historical lookups must not keep facts
// from decaying). A nil asOf is the normal "valid now" recall.
func (b *Brain) RecallAsOf(ctx context.Context, namespaces []string, query string, limit int, asOf *time.Time) ([]RecallResult, error) {
	return b.recall(ctx, namespaces, query, limit, RecallOptions{AsOf: asOf}, nil)
}

// recall is the shared implementation of RecallAsOf and RecallDetailed. A nil
// trace records nothing.
func (b *Brain) recall(ctx context.Context, namespaces []string, query string, limit int, opts RecallOptions, tr *RecallTrace) ([]RecallResult, error) {
	if opts.AsOf == nil && b.config.TemporalRecall && (opts.From == nil || opts.To == nil) {
		parsedFrom, parsedTo := temporalWindow(query, time.Now())
		if opts.From == nil {
			opts.From = parsedFrom
		}
		if opts.To == nil {
			opts.To = parsedTo
		}
	}
	if opts.From != nil && opts.To != nil && !opts.From.Before(*opts.To) {
		return nil, fmt.Errorf("invalid time window: from must be before to")
	}
	asOf := opts.AsOf
	temporal := b.config.TemporalArm && (opts.From != nil || opts.To != nil)
	var routed []string
	routeOn := b.config.RouteRecall
	if opts.Route != nil {
		routeOn = *opts.Route
	}
	if routeOn && len(opts.Types) == 0 {
		if routed = routeTypes(query); len(routed) > 0 {
			opts.Types = append([]string{"fact", "episode"}, routed...)
		}
	}
	facts, episodes := opts.includes()
	graphOn := b.config.GraphRecall
	if opts.Graph != nil {
		graphOn = *opts.Graph
	}
	graph := graphOn && b.config.HybridRecall
	if err := validateContent(query); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	started := time.Now()
	stage := tr.stage(started)
	vec, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	stage("embed", 0)

	pgVec := pgvector.NewVector(vec)

	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaces)
	if err != nil {
		return nil, err
	}

	kwQuery := ""
	if b.config.HybridRecall {
		kwQuery = keywordQuery(query)
	}
	// Recency and importance reorder the fused list, so the pool must be wider than limit.
	recency := b.reranks()
	candidates := limit
	if kwQuery != "" || recency || temporal {
		candidates = limit * 3
		if candidates > maxCandidates {
			candidates = maxCandidates
		}
	}
	now := time.Now()
	if asOf != nil {
		now = *asOf
	}

	if tr != nil {
		tr.Query, tr.KeywordQuery, tr.Hybrid, tr.Graph, tr.Limit, tr.Candidates = query, kwQuery, kwQuery != "", graph, limit, candidates
		tr.Namespaces, tr.NamespaceIDs, tr.Routed = namespaces, len(nsIDs), routed
		if asOf != nil {
			tr.AsOf = asOf.Format(time.RFC3339)
		}
		if opts.From != nil {
			tr.From = opts.From.Format(time.RFC3339)
		}
		if opts.To != nil {
			tr.To = opts.To.Format(time.RFC3339)
		}
		tr.Temporal = temporal
		tr.MinSimilarity, tr.RRFK = b.config.MinRecallSimilarity, rrfK
		tr.RecencyWeight, tr.RecencyHalfLife, tr.ImportanceWeight = b.config.RecencyWeight, b.config.RecencyHalfLife.String(), b.config.ImportanceWeight
	}

	var results []RecallResult

	// Search facts first (higher quality, consolidated)
	if facts {
		factSQL, factArgs, err := b.queries.RecallFactsWindow(nsIDs, pgVec, candidates, asOf, opts.From, opts.To)
		if err != nil {
			return nil, fmt.Errorf("build fact query: %w", err)
		}
		factVec, err := b.runRecall(ctx, factSQL, factArgs, "fact")
		if err != nil {
			return nil, fmt.Errorf("query facts: %w", err)
		}
		stage("facts_vector", len(factVec))

		var factKw, factGraph, factTemporal []RecallResult
		if kwQuery != "" {
			kwSQL, kwArgs, err := b.queries.RecallFactsKeywordWindow(nsIDs, kwQuery, candidates, asOf, opts.From, opts.To)
			if err != nil {
				return nil, fmt.Errorf("build fact keyword query: %w", err)
			}
			if factKw, err = b.runRecall(ctx, kwSQL, kwArgs, "fact"); err != nil {
				return nil, fmt.Errorf("keyword query facts: %w", err)
			}
			stage("facts_keyword", len(factKw))
		}
		if graph {
			graphSQL, graphArgs, err := b.queries.RecallFactsGraphWindow(nsIDs, query, pgVec, candidates, asOf, opts.From, opts.To)
			if err != nil {
				return nil, fmt.Errorf("build fact graph query: %w", err)
			}
			if factGraph, err = b.runRecall(ctx, graphSQL, graphArgs, "fact"); err != nil {
				return nil, fmt.Errorf("graph query facts: %w", err)
			}
			stage("facts_graph", len(factGraph))
			if tr != nil {
				tr.GraphEntities = b.matchedEntities(ctx, nsIDs, query)
			}
		}
		if temporal {
			temporalSQL, temporalArgs, err := b.queries.RecallFactsTemporal(nsIDs, candidates, asOf, opts.From, opts.To)
			if err != nil {
				return nil, fmt.Errorf("build fact temporal query: %w", err)
			}
			if factTemporal, err = b.runRecall(ctx, temporalSQL, temporalArgs, "fact"); err != nil {
				return nil, fmt.Errorf("temporal query facts: %w", err)
			}
			stage("facts_temporal", len(factTemporal))
		}
		if graph || temporal {
			arms := []rankedArm{{"vector", factVec}, {"keyword", factKw}}
			if graph {
				arms = append(arms, rankedArm{"graph", factGraph})
			}
			if temporal {
				arms = append(arms, rankedArm{"temporal", factTemporal})
			}
			scaleArms := 2
			if graph {
				scaleArms++
			}
			if temporal {
				scaleArms++
			}
			results = fuseArms(arms, scaleArms, candidates, b.config.MinRecallSimilarity)
		} else {
			results = fuse(factVec, factKw, candidates, kwQuery != "", b.config.MinRecallSimilarity)
		}
		if b.config.Reranker != nil {
			results = b.rerank(ctx, query, results)
			stage("facts_reranked", len(results))
		}
		// A historical lookup answers "what did we know then": today's opinion
		// on a fact must not reorder the record of a past moment.
		if asOf == nil {
			b.applyFeedback(ctx, results)
		}
		results = b.applyBoosts(results, limit, now)
		stage("facts_fused", len(results))
	}

	// Search episodes for remaining slots
	if episodeLimit := limit - len(results); episodes && episodeLimit > 0 {
		epCandidates := episodeLimit
		if kwQuery != "" || recency || temporal {
			epCandidates = episodeLimit * 3
			if epCandidates > maxCandidates {
				epCandidates = maxCandidates
			}
		}
		epSQL, epArgs, err := b.queries.RecallEpisodesWindow(nsIDs, pgVec, epCandidates, asOf, opts.From, opts.To)
		if err != nil {
			return nil, fmt.Errorf("build episode query: %w", err)
		}
		epVec, err := b.runRecall(ctx, epSQL, epArgs, "episode")
		if err != nil {
			return nil, fmt.Errorf("query episodes: %w", err)
		}
		stage("episodes_vector", len(epVec))

		var epKw, epTemporal []RecallResult
		if kwQuery != "" {
			kwSQL, kwArgs, err := b.queries.RecallEpisodesKeywordWindow(nsIDs, kwQuery, epCandidates, asOf, opts.From, opts.To)
			if err != nil {
				return nil, fmt.Errorf("build episode keyword query: %w", err)
			}
			if epKw, err = b.runRecall(ctx, kwSQL, kwArgs, "episode"); err != nil {
				return nil, fmt.Errorf("keyword query episodes: %w", err)
			}
			stage("episodes_keyword", len(epKw))
		}
		if temporal {
			temporalSQL, temporalArgs, err := b.queries.RecallEpisodesTemporal(nsIDs, epCandidates, asOf, opts.From, opts.To)
			if err != nil {
				return nil, fmt.Errorf("build episode temporal query: %w", err)
			}
			if epTemporal, err = b.runRecall(ctx, temporalSQL, temporalArgs, "episode"); err != nil {
				return nil, fmt.Errorf("temporal query episodes: %w", err)
			}
			stage("episodes_temporal", len(epTemporal))
		}
		var eps []RecallResult
		if graph || temporal {
			// Episodes carry no entity, so the graph arm never ranks them; they are
			// scored on the same scale as facts so the two stay comparable.
			arms := []rankedArm{{"vector", epVec}, {"keyword", epKw}}
			if temporal {
				arms = append(arms, rankedArm{"temporal", epTemporal})
			}
			scaleArms := 2
			if graph {
				scaleArms++
			}
			if temporal {
				scaleArms++
			}
			eps = fuseArms(arms, scaleArms, epCandidates, b.config.MinRecallSimilarity)
		} else {
			eps = fuse(epVec, epKw, epCandidates, kwQuery != "", b.config.MinRecallSimilarity)
		}
		eps = b.rerank(ctx, query, eps)
		results = append(results, b.applyBoosts(eps, episodeLimit, now)...)
		stage("episodes_fused", len(eps))
	}

	// Failures and goals only take part when asked for by type; they compete with
	// facts and episodes in the final sort.
	if failures, goals := opts.includesWork(); failures || goals {
		work, err := b.recallWork(ctx, nsIDs, query, limit, asOf, failures, goals)
		if err != nil {
			return nil, err
		}
		stage("work_keyword", len(work))
		results = append(results, work...)
	}

	// Sort all results by score descending
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	// A traced recall is an inspection, not a use: like a historical lookup it
	// must not keep facts from decaying.
	if asOf == nil && tr == nil {
		b.touchRecalledFacts(ctx, results)
	}
	if tr != nil {
		for i := range results {
			results[i].Trace = results[i].trace()
		}
		tr.TotalMillis = float64(time.Since(started).Microseconds()) / 1000
	}

	return results, nil
}

// rerank re-scores fused candidates against the query with a cross-encoder,
// in place, best first. Without a configured reranker it is a no-op that keeps
// the fused order and scores exactly. A failing reranker never fails the recall:
// the candidates fall back to the fused order.
func (b *Brain) rerank(ctx context.Context, query string, rs []RecallResult) []RecallResult {
	if b.config.Reranker == nil || len(rs) < 2 {
		return rs
	}
	cap := b.config.RerankCandidates
	if cap <= 0 || cap > len(rs) {
		cap = len(rs)
	}
	candidates := make([]reranker.Candidate, cap)
	for i := 0; i < cap; i++ {
		candidates[i] = reranker.Candidate{Content: rs[i].Content, Fused: rs[i].Score}
	}
	scored, err := b.config.Reranker.Rerank(ctx, query, candidates)
	if err != nil {
		return rs
	}
	// The cross-encoder sees the query and the candidate together, which
	// position-based fusion cannot; its verdict replaces the fused score.
	// Candidates it did not judge keep their fused score and land behind the
	// re-scored ones, in their original relative order.
	byIndex := make(map[int]float32, len(scored))
	for _, s := range scored {
		byIndex[s.Index] = s.Score
	}
	var rescored, rest []RecallResult
	for i := 0; i < cap; i++ {
		if s, ok := byIndex[i]; ok {
			rs[i].Score = s
			rescored = append(rescored, rs[i])
		} else {
			rest = append(rest, rs[i])
		}
	}
	sort.SliceStable(rescored, func(i, j int) bool { return rescored[i].Score > rescored[j].Score })
	return append(rescored, append(rest, rs[cap:]...)...)
}

// recencyFactor is the multiplier applied to a result's score for a given age:
// 1 for something brand new, falling towards 1-weight as the age grows (it is
// halfway there after one half-life). With weight 0 it is always 1, so the
// ranking is untouched. Scores therefore stay within 0..1.
func recencyFactor(age time.Duration, weight float64, halfLife time.Duration) float64 {
	if weight <= 0 || halfLife <= 0 {
		return 1
	}
	if weight > 1 {
		weight = 1
	}
	if age < 0 {
		age = 0
	}
	return (1 - weight) + weight*math.Pow(0.5, float64(age)/float64(halfLife))
}

// importanceFactor is the multiplier for a result's importance: 1 for importance
// 1, falling to 1-weight for importance 0. With weight 0 it is always 1.
func importanceFactor(importance float32, weight float64) float64 {
	if weight <= 0 {
		return 1
	}
	if weight > 1 {
		weight = 1
	}
	imp := math.Min(1, math.Max(0, float64(importance)))
	return (1 - weight) + weight*imp
}

// reranks reports whether recency or importance weighting is switched on.
func (b *Brain) reranks() bool {
	return (b.config.RecencyWeight > 0 && b.config.RecencyHalfLife > 0) || b.config.ImportanceWeight > 0
}

// applyBoosts rescales scores by recencyFactor, importanceFactor and a fact's
// feedback factor, re-sorts and keeps the best limit. With all three off it only
// truncates.
func (b *Brain) applyBoosts(rs []RecallResult, limit int, now time.Time) []RecallResult {
	for i := range rs {
		rs[i].fusedScore, rs[i].recencyF, rs[i].importanceF = rs[i].Score, 1, 1
	}
	if !b.reranks() && !anyFeedback(rs) {
		return truncate(rs, limit)
	}
	w, hl, iw := b.config.RecencyWeight, b.config.RecencyHalfLife, b.config.ImportanceWeight
	for i := range rs {
		imp := importanceFactor(rs[i].Importance, iw)
		f, rec := imp, 1.0
		if !rs[i].refTime.IsZero() {
			rec = recencyFactor(now.Sub(rs[i].refTime), w, hl)
			f *= rec
		}
		rs[i].recencyF, rs[i].importanceF = rec, imp
		f *= feedbackFactor(rs[i].Feedback)
		rs[i].Score *= float32(f)
	}
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].Score > rs[j].Score })
	return truncate(rs, limit)
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

// typedID records which table ID belongs to. Fact ids and episode ids are
// independent sequences that overlap, and only the caller knows which endpoint
// it got the id from; naming it here means a client never has to infer it.
func (r *RecallResult) typedID(kind string) {
	id := r.ID
	switch kind {
	case "fact":
		r.FactID = &id
	case "failure":
		r.FailureID = &id
	case "goal":
		r.GoalID = &id
	default:
		r.EpisodeID = &id
	}
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
			if err := rows.Scan(&r.ID, &r.NamespaceID, &r.Content, &r.Confidence, &createdAt, &r.Importance, &r.Score); err != nil {
				return nil, fmt.Errorf("scan fact: %w", err)
			}
		} else {
			var occurredAt time.Time
			if err := rows.Scan(&r.ID, &r.NamespaceID, &r.Content, &occurredAt, &createdAt, &r.Importance, &r.Score); err != nil {
				return nil, fmt.Errorf("scan episode: %w", err)
			}
			r.OccurredAt = occurredAt.Format(time.RFC3339)
		}
		r.typedID(kind)
		r.CreatedAt = createdAt.Format(time.RFC3339)
		r.refTime = createdAt
		if kind == "episode" {
			r.refTime, _ = time.Parse(time.RFC3339, r.OccurredAt)
		}
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
			vec[i].vecRank = i + 1
		}
		return truncate(relevant(vec, minSimilarity), limit)
	}

	return fuseArms([]rankedArm{{"vector", vec}, {"keyword", kw}}, 2, limit, minSimilarity)
}

// rankedArm is one ranking, best first, that reciprocal rank fusion combines.
type rankedArm struct {
	name    string // "vector", "keyword" or "graph"
	results []RecallResult
}

// fuseArms merges the arms with reciprocal rank fusion. scaleArms is how many
// arms a result could be ranked first in: the best possible score (rank 1 in all
// of them) becomes 1. It is a property of the configuration, not of the query,
// so a score means the same thing whether or not an arm found anything.
func fuseArms(arms []rankedArm, scaleArms, limit int, minSimilarity float32) []RecallResult {
	type entry struct {
		res   RecallResult
		score float64
	}
	var size int
	for _, a := range arms {
		size += len(a.results)
	}
	entries := make(map[int64]*entry, size)
	var order []int64

	for _, arm := range arms {
		for rank, r := range arm.results {
			e, ok := entries[r.ID]
			if !ok {
				e = &entry{res: r}
				entries[r.ID] = e
				order = append(order, r.ID)
			}
			e.score += 1.0 / float64(rrfK+rank+1)
			switch {
			case e.res.Match == "":
				e.res.Match = arm.name
			case e.res.Match != arm.name:
				e.res.Match = "hybrid"
			}
			switch arm.name {
			case "vector":
				e.res.Similarity = r.Score
				e.res.vecRank = rank + 1
			case "keyword":
				e.res.kwRank, e.res.kwScore = rank+1, r.Score
			case "graph":
				e.res.graphRank, e.res.graphScore = rank+1, r.Score
			case "temporal":
				e.res.temporalRank = rank + 1
			}
		}
	}

	best := float64(scaleArms) / float64(rrfK+1)
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
