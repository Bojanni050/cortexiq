// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/Bojanni050/cortexiq/internal/observability"
	"github.com/Bojanni050/cortexiq/internal/reasoner"
	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

// ConsolidationResult describes the outcome of a consolidation run.
type ConsolidationResult struct {
	Namespace         string        `json:"namespace"`
	Duration          time.Duration `json:"duration"`
	EpisodesRead      int           `json:"episodes_read"`
	EpisodesSkipped   int           `json:"episodes_skipped"`
	FactsCreated      int           `json:"facts_created"`
	FactsDeduplicated int           `json:"facts_deduplicated"`
	// DedupNearMisses counts facts that were stored as new while a near-twin —
	// a candidate above dedupNearMissFloor — was already there. Duplicates are
	// born in that band and nothing else reports on it.
	DedupNearMisses            int      `json:"dedup_near_misses"`
	RelationshipsFound         int      `json:"relationships_found"`
	CausalLinksFound           int      `json:"causal_links_found"`
	PatternsFound              int      `json:"patterns_found"`
	ContradictionsFound        int      `json:"contradictions_found"`
	ContradictionsAutoResolved int      `json:"contradictions_auto_resolved"`
	GoalsAnnotated             int      `json:"goals_annotated"`
	GoalsSuggestedComplete     int      `json:"goals_suggested_complete"`
	FailureRepeatsDetected     int      `json:"failure_repeats_detected"`
	FailurePatternsFound       int      `json:"failure_patterns_found"`
	HypothesesAutoConfirmed    int      `json:"hypotheses_auto_confirmed"`
	HypothesesAutoRejected     int      `json:"hypotheses_auto_rejected"`
	HypothesesUpdated          int      `json:"hypotheses_updated"`
	FactsDecayed               int      `json:"facts_decayed"`
	FactsExpired               int      `json:"facts_expired"`
	MentalModelsRefreshed      int      `json:"mental_models_refreshed"`
	MentalModelsFailed         int      `json:"mental_models_failed"`
	LLMCalls                   int      `json:"llm_calls"`
	LLMCallsLimited            bool     `json:"llm_calls_limited"`
	Errors                     []string `json:"errors,omitempty"`
}

// Consolidate runs the full 8-stage consolidation pipeline for a namespace.
func (b *Brain) Consolidate(ctx context.Context, namespaceSlug string) (ConsolidationResult, error) {
	if err := validatePath(namespaceSlug); err != nil {
		return ConsolidationResult{}, err
	}
	nsID, err := b.resolveNamespaceID(ctx, namespaceSlug)
	if err != nil {
		return ConsolidationResult{}, err
	}
	return b.ConsolidateByID(ctx, nsID)
}

// ConsolidateByID runs the full 8-stage consolidation pipeline for a namespace by ID.
// Stages run in execution order:
// 1. Episodes -> Facts                    (cluster + synthesize, with inline contradiction detection)
// 2. Facts -> Relationships               (extract entity edges)
// 3. Facts -> Causal Links                (cause/effect relationships)
// 4. Goal Progress Inference              (annotate goals from new facts)
// 5. Failure Pattern Detection            (recurring failures across episodes)
// 6. Facts + Relationships -> Patterns    (extract abstractions)
// 7. Hypothesis Evidence Scanning         (auto-confirm/reject pending hypotheses)
// 8. Confidence Decay                     (pure-SQL, no LLM)
func (b *Brain) ConsolidateByID(ctx context.Context, nsID int64) (ConsolidationResult, error) {
	start := time.Now()

	var namespaceSlug string
	_ = b.pool.QueryRow(ctx, "SELECT slug FROM namespaces WHERE id = $1", nsID).Scan(&namespaceSlug)

	// Per-namespace overrides resolve over the ancestor chain with the global
	// config as the default; a namespace without overrides gets exactly the
	// process configuration.
	nsCfg := b.namespaceRunConfig(ctx, nsID)
	ctx = withLLMBudget(ctx, nsCfg.MaxLLMCallsPerRun)

	result := ConsolidationResult{Namespace: namespaceSlug}

	cp, err := b.GetOrCreateConsolidationProgress(ctx, nsID)
	if err != nil {
		return result, fmt.Errorf("get progress: %w", err)
	}

	// Stage 1: Episodes -> Facts (+ Stage 4: Contradiction detection)
	if ctx.Err() == nil && !llmBudgetExhausted(ctx) {
		factsCreated, factsDeduped, episodesRead, episodesSkipped, nearMisses, llmCalls, contFound, contAuto, errs := b.consolidateEpisodesToFacts(ctx, nsID, cp)
		result.FactsCreated = factsCreated
		result.FactsDeduplicated = factsDeduped
		result.EpisodesRead = episodesRead
		result.EpisodesSkipped = episodesSkipped
		result.DedupNearMisses = nearMisses
		result.LLMCalls += llmCalls
		result.ContradictionsFound = contFound
		result.ContradictionsAutoResolved = contAuto
		result.Errors = append(result.Errors, errs...)
	}

	// Stage 2: Facts -> Relationships
	if ctx.Err() == nil && !llmBudgetExhausted(ctx) {
		relCount, llmCalls, errs := b.consolidateFactsToRelationships(ctx, nsID, cp)
		result.RelationshipsFound = relCount
		result.LLMCalls += llmCalls
		result.Errors = append(result.Errors, errs...)
	}

	// Stage 3.5: Facts -> Causal Links
	if ctx.Err() == nil && !llmBudgetExhausted(ctx) {
		causalCount, llmCalls, errs := b.consolidateFactsToCausalLinks(ctx, nsID, cp)
		result.CausalLinksFound = causalCount
		result.LLMCalls += llmCalls
		result.Errors = append(result.Errors, errs...)
	}

	// Stage 6: Goal Progress Inference
	if ctx.Err() == nil && !llmBudgetExhausted(ctx) {
		annotated, suggestedComplete, llmCalls, errs := b.consolidateGoalProgress(ctx, nsID, cp)
		result.GoalsAnnotated = annotated
		result.GoalsSuggestedComplete = suggestedComplete
		result.LLMCalls += llmCalls
		result.Errors = append(result.Errors, errs...)
	}

	// Stage 7: Failure Pattern Detection
	if ctx.Err() == nil && !llmBudgetExhausted(ctx) {
		repeats, patterns, llmCalls, errs := b.consolidateFailurePatterns(ctx, nsID, cp)
		result.FailureRepeatsDetected = repeats
		result.FailurePatternsFound = patterns
		result.LLMCalls += llmCalls
		result.Errors = append(result.Errors, errs...)
	}

	// Stage 3: Facts + Relationships -> Patterns
	if ctx.Err() == nil && !llmBudgetExhausted(ctx) {
		patCount, llmCalls, errs := b.consolidateToPatterns(ctx, nsID, cp)
		result.PatternsFound = patCount
		result.LLMCalls += llmCalls
		result.Errors = append(result.Errors, errs...)
	}

	// Stage 8: Hypothesis Evidence Scanning
	if ctx.Err() == nil && !llmBudgetExhausted(ctx) {
		autoConfirmed, autoRejected, updated, llmCalls, errs := b.consolidateHypothesisEvidence(ctx, nsID, cp)
		result.HypothesesAutoConfirmed = autoConfirmed
		result.HypothesesAutoRejected = autoRejected
		result.HypothesesUpdated = updated
		result.LLMCalls += llmCalls
		result.Errors = append(result.Errors, errs...)
	}

	// Stage 5: Confidence decay
	if ctx.Err() == nil {
		decayResult, err := b.DecayConfidence(ctx, nsID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("decay confidence: %v", err))
		} else {
			result.FactsDecayed = decayResult.FactsDecayed
			result.FactsExpired = decayResult.FactsExpired
		}
	}

	// Stage 9: refresh mental models whose ground truth changed since their last
	// synthesis. Best effort by design: a model that cannot refresh is recorded
	// on its own row and in the result, but never fails the consolidation.
	if ctx.Err() == nil && !llmBudgetExhausted(ctx) {
		summary, err := b.refreshStaleMentalModels(ctx, nsID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("refresh mental models: %v", err))
		}
		result.MentalModelsRefreshed = summary.Refreshed
		result.MentalModelsFailed = summary.Failed
		result.Errors = append(result.Errors, summary.Errors...)
	}

	// Save progress
	now := time.Now().UTC()
	cp.LastRun = &now
	saveCtx := ctx
	if ctx.Err() != nil {
		saveCtx = context.Background()
	}
	if err := b.SaveConsolidationProgress(saveCtx, *cp); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("save progress: %v", err))
	}

	result.Duration = time.Since(start)
	if llmBudgetExhausted(ctx) {
		result.LLMCallsLimited = true
	}
	for _, e := range result.Errors {
		if strings.Contains(e, "reasoner call budget exhausted") {
			result.LLMCallsLimited = true
			break
		}
	}

	// Run history is best effort: record a failure so it is visible, but never
	// fail consolidation because history could not be written.
	if err := b.SaveConsolidationRun(saveCtx, nsID, result, start); err != nil {
		result.Errors = append(result.Errors, err.Error())
	}

	observability.RecordConsolidation(observability.Observation{
		Namespace:          namespaceSlug,
		EventsRead:         result.EpisodesRead,
		EventsProcessed:    result.EpisodesRead,
		FactsCreated:       result.FactsCreated,
		FactsDeduplicated:  result.FactsDeduplicated,
		RelationshipsFound: result.RelationshipsFound,
		LLMCalls:           result.LLMCalls,
		Duration:           result.Duration,
		Errors:             len(result.Errors),
	})

	return result, nil
}

// --- Stage 1: Episodes -> Facts ---

func (b *Brain) consolidateEpisodesToFacts(ctx context.Context, nsID int64, cp *models.ConsolidationProgress) (created, deduped, read, skipped, nearMisses, llmCalls, contradictionsFound, contradictionsAutoResolved int, errs []string) {
	nsCfg := b.namespaceRunConfig(ctx, nsID)
	sql, args, err := b.queries.FetchEpisodes(nsID, cp.LastEpisodeID, nsCfg.BatchSize)
	if err != nil {
		errs = append(errs, fmt.Sprintf("build fetch episodes: %v", err))
		return
	}

	rows, err := b.pool.Query(ctx, sql, args...)
	if err != nil {
		errs = append(errs, fmt.Sprintf("fetch episodes: %v", err))
		return
	}
	defer rows.Close()

	var episodes []models.Episode
	for rows.Next() {
		var e models.Episode
		if err := rows.Scan(&e.ID, &e.NamespaceID, &e.Content, &e.Embedding, &e.EmbeddingModel, &e.OccurredAt, &e.CreatedAt); err != nil {
			errs = append(errs, fmt.Sprintf("scan episode: %v", err))
			continue
		}
		episodes = append(episodes, e)
	}
	if err := rows.Err(); err != nil {
		errs = append(errs, fmt.Sprintf("episode rows: %v", err))
		return
	}

	read = len(episodes)
	if read == 0 {
		return
	}

	// done holds episodes whose cluster was fully handled. The checkpoint only
	// advances over a contiguous run of done episodes, so a failed cluster is
	// retried next run without blocking (or re-processing) everything before it.
	done := make(map[int64]bool)
	markDone := func(cluster []models.Episode) {
		for _, e := range cluster {
			done[e.ID] = true
		}
	}

	// An episode that states nothing — one token, a greeting, a clipped
	// acknowledgement — must not reach the extractor: asked for facts, it reads
	// a claim into the fragment. A bare "2" became "CortexIQ includes the content
	// '2' … as a signal or reminder". Skipped episodes count as done, otherwise
	// the checkpoint stops on them and nothing after them is ever consolidated.
	extractable, fragments := partitionFragments(episodes, nsCfg.MinEpisodeLength)
	markDone(fragments)
	skipped = len(fragments)

	// Cluster by vector similarity
	clusters := clusterEpisodes(extractable, nsCfg)

	scanErrs := len(errs)

	var subject string
	if len(clusters) > 0 {
		subject = b.namespaceSubject(ctx, nsID)
	}

	for _, cluster := range clusters {
		if ctx.Err() != nil {
			break
		}

		var texts []string
		var episodeIDs []int64
		for _, e := range cluster {
			texts = append(texts, e.Content)
			episodeIDs = append(episodeIDs, e.ID)
		}

		facts, err := b.reasoner.ReasonStructured(ctx, texts, subject)
		llmCalls++
		if err != nil {
			errs = append(errs, fmt.Sprintf("reason structured: %v", err))
			continue
		}

		// A cluster is done only when every one of its facts was handled; with no
		// facts (nothing durable in the episodes) it is done right away.
		clusterOK := true
		for _, sf := range facts {
			r, err := b.storeFactWithConfig(ctx, nsID, sf, episodeIDs, nsCfg)
			llmCalls += r.llmCalls
			if err != nil {
				errs = append(errs, err.Error())
				clusterOK = false
				continue
			}
			if r.created {
				created++
			}
			if r.deduped {
				deduped++
			}
			if r.nearMiss {
				nearMisses++
			}
			contradictionsFound += r.contradictions
			contradictionsAutoResolved += r.autoResolved
		}
		if clusterOK {
			markDone(cluster)
		}
	}

	// Rows that failed to scan are missing from `episodes`; advancing past them
	// would lose them, so only move the checkpoint when the fetch was clean.
	// The ids come from the whole fetched batch, fragments included: a skipped
	// fragment is done and must count when the checkpoint walks forward.
	if scanErrs == 0 {
		ids := make([]int64, len(episodes))
		for i, e := range episodes {
			ids[i] = e.ID
		}
		cp.LastEpisodeID = contiguousCheckpoint(ids, done, cp.LastEpisodeID)
	}
	return
}

// partitionFragments splits a fetched batch into the episodes worth extracting
// from and the ones that state nothing. minChars of 0 disables the filter. The
// length is measured in runes, so an accented sentence is not judged by its
// bytes. Callers must treat the fragments as done, or the consolidation
// checkpoint stops on them forever.
func partitionFragments(episodes []models.Episode, minChars int) (extractable, fragments []models.Episode) {
	if minChars <= 0 {
		return episodes, nil
	}
	for _, e := range episodes {
		if utf8.RuneCountInString(strings.TrimSpace(e.Content)) < minChars {
			fragments = append(fragments, e)
			continue
		}
		extractable = append(extractable, e)
	}
	return extractable, fragments
}

// contiguousCheckpoint returns the highest ID such that every ID up to and
// including it is done. ids must be ordered ascending (as fetched).
// If the first ID is not done, current is returned unchanged.
func contiguousCheckpoint(ids []int64, done map[int64]bool, current int64) int64 {
	next := current
	for _, id := range ids {
		if !done[id] {
			break
		}
		if id > next {
			next = id
		}
	}
	return next
}

func clusterEpisodes(episodes []models.Episode, cfg Config) [][]models.Episode {
	if len(episodes) == 0 {
		return nil
	}

	clustered := make(map[int64]bool)
	cache := make(trigramCache, len(episodes))
	var clusters [][]models.Episode

	for _, seed := range episodes {
		if clustered[seed.ID] {
			continue
		}

		cluster := []models.Episode{seed}
		clustered[seed.ID] = true

		if seed.Embedding.Slice() == nil {
			clusters = append(clusters, cluster)
			continue
		}

		for _, member := range clusterMembers(seed, episodes, clustered, cache, cfg) {
			cluster = append(cluster, member)
			clustered[member.ID] = true
		}

		clusters = append(clusters, cluster)
	}

	return clusters
}

// clusterMembers weighs the episodes still free against one seed and returns
// the ones that join its cluster.
//
// The semantic arm is the original rule and is never truncated: every episode
// with cosine at or above the similarity threshold joins, in batch order. With
// CandidateCap 0 that is the whole decision, so clustering behaves exactly as
// before.
//
// With a cap a second, lexical arm adds episodes the threshold misses: it weighs
// every other free episode by shared character trigrams, drops the ones below
// LexicalFloor, and adds the best cap of them. The cap bounds only these extras.
// Capping the semantic arm too would drop twins the threshold alone finds (measured
// with cortexiq eval cluster: cap 2 reached 15 of 20 twins, threshold-only 16).
// Without a floor the lexical arm pads every cluster to the cap with the least
// unrelated episodes, which is why the floor exists.
//
// The extractor then reads both texts in one call, which is the only place a
// twin can be merged instead of becoming a duplicate. Every fact the call
// produces is linked to every episode in the cluster, so extras widen
// provenance: keep the cap small.
func clusterMembers(seed models.Episode, episodes []models.Episode, clustered map[int64]bool, cache trigramCache, cfg Config) []models.Episode {
	seedVec := seed.Embedding.Slice()
	threshold := float32(cfg.SimilarityThreshold)

	var members []models.Episode
	inCluster := make(map[int64]bool)
	for _, candidate := range episodes {
		if clustered[candidate.ID] {
			continue
		}
		candVec := candidate.Embedding.Slice()
		if candVec == nil {
			continue
		}
		if cosineSimilarity(seedVec, candVec) >= threshold {
			members = append(members, candidate)
			inCluster[candidate.ID] = true
		}
	}

	cap := cfg.CandidateCap
	if cap <= 0 {
		return members
	}

	var lexical []clusterCandidate
	for _, candidate := range episodes {
		if clustered[candidate.ID] || inCluster[candidate.ID] {
			continue
		}
		score := trigramJaccard(cache.of(seed), cache.of(candidate))
		lexical = append(lexical, clusterCandidate{episode: candidate, score: score})
	}
	for _, c := range rankCandidates(lexical, float32(cfg.LexicalFloor), cap) {
		members = append(members, c.episode)
	}
	return members
}

// storeResult describes what storeFact did with one synthesized fact.
type storeResult struct {
	created, deduped bool
	// nearMiss says the fact was stored as new while a near-twin already existed.
	nearMiss       bool
	contradictions int
	autoResolved   int
	llmCalls       int
}

// storeFact embeds one synthesized fact and either folds it into an existing
// duplicate (reinforcing that one) or inserts it, links its source episodes and
// checks it for contradictions.
func (b *Brain) storeFact(ctx context.Context, nsID int64, sf *reasoner.StructuredFact, episodeIDs []int64) (storeResult, error) {
	return b.storeFactWithConfig(ctx, nsID, sf, episodeIDs, b.config)
}

// storeFactWithConfig is storeFact with an explicit configuration, so the
// consolidation run can pass its per-namespace resolved config.
func (b *Brain) storeFactWithConfig(ctx context.Context, nsID int64, sf *reasoner.StructuredFact, episodeIDs []int64, cfg Config) (storeResult, error) {
	var r storeResult

	vec, err := b.embedder.Embed(ctx, sf.Summary)
	if err != nil {
		return r, fmt.Errorf("embed fact: %w", err)
	}

	out, err := b.findDuplicateWithConfig(ctx, nsID, vec, sf.Summary, cfg)
	r.llmCalls += out.LLMCalls
	if err != nil {
		return r, fmt.Errorf("check duplicate: %w", err)
	}
	if out.Merged {
		b.reinforceFact(ctx, out.DupID, episodeIDs)
		r.deduped = true
		return r, nil
	}
	// The fact is stored as new. If something close was already there, this is
	// where a duplicate was born, and the run reports it.
	r.nearMiss = out.BestScore >= dedupNearMissFloor

	confidence := calculateConfidence(len(episodeIDs), sf.Entity != "" && sf.Property != "")
	now := time.Now().UTC()

	var factID int64
	err = b.pool.QueryRow(ctx,
		`INSERT INTO facts (namespace_id, content, embedding, embedding_model, confidence, entity, property, value, valid_from, importance)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
		nsID, sf.Summary, pgvector.NewVector(vec), b.embedder.Model(), confidence,
		strPtrOrNull(sf.Entity), strPtrOrNull(sf.Property), strPtrOrNull(sf.Value), now,
		b.sourceImportance(ctx, episodeIDs),
	).Scan(&factID)
	if err != nil {
		return r, fmt.Errorf("insert fact: %w", err)
	}
	r.created = true

	for _, eid := range episodeIDs {
		_, _ = b.pool.Exec(ctx,
			"INSERT INTO fact_sources (fact_id, episode_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
			factID, eid,
		)
	}

	// Stage 4: Contradiction detection
	newFact := &models.Fact{
		ID:          factID,
		NamespaceID: nsID,
		Content:     sf.Summary,
		Confidence:  confidence,
		Entity:      strPtrOrNull(sf.Entity),
		Property:    strPtrOrNull(sf.Property),
		Value:       strPtrOrNull(sf.Value),
	}
	r.contradictions, r.autoResolved, _ = b.DetectContradictions(ctx, nsID, newFact)
	return r, nil
}

// reinforceFact records that an existing fact was observed again: it resets the
// decay clock, nudges confidence towards 1 and links the new episodes as sources.
// Best effort — a failure here must not fail consolidation.
func (b *Brain) reinforceFact(ctx context.Context, factID int64, episodeIDs []int64) {
	_, _ = b.pool.Exec(ctx,
		`UPDATE facts SET confidence = LEAST(1.0, confidence + (1 - confidence) * $2),
		        importance = GREATEST(importance, $3), updated_at = now()
		 WHERE id = $1`,
		factID, reinforceStep, b.sourceImportance(ctx, episodeIDs),
	)
	for _, eid := range episodeIDs {
		_, _ = b.pool.Exec(ctx,
			"INSERT INTO fact_sources (fact_id, episode_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
			factID, eid,
		)
	}
}

// sourceImportance is the highest importance among the given source episodes: a
// fact is as important as the most important thing it was derived from. Falls
// back to the neutral default when there are no sources or the lookup fails —
// importance is a ranking hint and must never fail consolidation.
func (b *Brain) sourceImportance(ctx context.Context, episodeIDs []int64) float32 {
	if len(episodeIDs) == 0 {
		return DefaultImportance
	}
	var imp *float32
	if err := b.pool.QueryRow(ctx,
		"SELECT MAX(importance) FROM episodes WHERE id = ANY($1)", episodeIDs,
	).Scan(&imp); err != nil || imp == nil {
		return DefaultImportance
	}
	return *imp
}

// reinforceStep is the fraction of the remaining confidence gap closed each time
// an existing fact is observed again.
const reinforceStep = 0.2

func calculateConfidence(observationCount int, hasStructuredFields bool) float32 {
	if observationCount == 0 {
		return 0.0
	}
	base := float32(observationCount) / float32(observationCount+2)
	if hasStructuredFields {
		base = base + (1-base)*0.3
	}
	if base > 1.0 {
		base = 1.0
	}
	return base
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}

func strPtrOrNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// --- Stage 2: Facts -> Relationships ---

func (b *Brain) consolidateFactsToRelationships(ctx context.Context, nsID int64, cp *models.ConsolidationProgress) (count, llmCalls int, errs []string) {
	sql, args, err := b.queries.FetchFacts(nsID, cp.LastFactID, 50)
	if err != nil {
		errs = append(errs, fmt.Sprintf("build fetch facts: %v", err))
		return
	}

	rows, err := b.pool.Query(ctx, sql, args...)
	if err != nil {
		errs = append(errs, fmt.Sprintf("fetch facts: %v", err))
		return
	}
	defer rows.Close()

	var facts []models.Fact
	for rows.Next() {
		var f models.Fact
		if err := rows.Scan(&f.ID, &f.NamespaceID, &f.Content, &f.Embedding, &f.EmbeddingModel, &f.Confidence, &f.Entity, &f.Property, &f.Value, &f.ValidFrom, &f.ValidUntil, &f.CreatedAt, &f.UpdatedAt); err != nil {
			errs = append(errs, fmt.Sprintf("scan fact: %v", err))
			continue
		}
		facts = append(facts, f)
	}
	if err := rows.Err(); err != nil {
		errs = append(errs, fmt.Sprintf("fact rows: %v", err))
		return
	}

	if len(facts) == 0 {
		return
	}

	batchSize := b.config.RelationshipBatchSize
	if batchSize <= 0 {
		batchSize = defaultRelationshipBatchSize
	}

	// One LLM call per batch of facts instead of per fact. Facts are done per
	// batch; the checkpoint only moves over a contiguous run of done facts so a
	// failed batch is retried next run without blocking the ones before it.
	done := make(map[int64]bool)
	for start := 0; start < len(facts); start += batchSize {
		if ctx.Err() != nil {
			break
		}
		end := start + batchSize
		if end > len(facts) {
			end = len(facts)
		}
		batch := facts[start:end]

		byFact, err := b.reasoner.ReasonRelationshipsBatch(ctx, batch)
		llmCalls++
		if err != nil {
			errs = append(errs, fmt.Sprintf("reason relationships facts %d-%d: %v", batch[0].ID, batch[len(batch)-1].ID, err))
			continue
		}

		batchOK := true
		for _, fact := range batch {
			for _, rel := range byFact[fact.ID] {
				if rel.FromEntity == "" || rel.RelationType == "" || rel.ToEntity == "" {
					continue
				}

				created, err := b.storeRelationship(ctx, nsID, fact.ID, rel)
				if err != nil {
					errs = append(errs, fmt.Sprintf("store relationship: %v", err))
					batchOK = false
					continue
				}
				if created {
					count++
				}
			}
		}
		if batchOK {
			for _, f := range batch {
				done[f.ID] = true
			}
		}
	}

	ids := make([]int64, len(facts))
	for i, f := range facts {
		ids[i] = f.ID
	}
	cp.LastFactID = contiguousCheckpoint(ids, done, cp.LastFactID)
	return
}

// storeRelationship inserts an extracted relationship unless the same statement is
// already stored. "The same" ignores which fact it was extracted from, the case of
// the entities and, via canonical_type, the exact wording of the relation
// ("includes" vs "contains"): the same edge stated by several facts used to be
// stored once per fact (e.g. "Bo uses Kilo Code" six times). A repeat only raises
// the stored confidence; source_fact_id keeps pointing at the first fact. Returns
// whether a new row was created.
func (b *Brain) storeRelationship(ctx context.Context, nsID, factID int64, rel *reasoner.StructuredRelationship) (bool, error) {
	// Rows without a canonical_type are compared by their lowercased relation_type.
	key := rel.CanonicalType
	if key == "" {
		key = strings.ToLower(strings.TrimSpace(rel.RelationType))
	}

	var id int64
	var conf float32
	err := b.pool.QueryRow(ctx,
		`SELECT id, confidence FROM relationships
		 WHERE namespace_id = $1 AND lower(from_entity) = lower($2) AND lower(to_entity) = lower($3)
		   AND COALESCE(canonical_type, lower(btrim(relation_type))) = $4
		   AND deleted_at IS NULL
		 ORDER BY id LIMIT 1`,
		nsID, rel.FromEntity, rel.ToEntity, key,
	).Scan(&id, &conf)
	switch {
	case err == nil:
		if rel.Confidence > conf {
			if _, err := b.pool.Exec(ctx, "UPDATE relationships SET confidence = $2 WHERE id = $1", id, rel.Confidence); err != nil {
				return false, fmt.Errorf("raise relationship confidence: %w", err)
			}
		}
		return false, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return false, fmt.Errorf("look up relationship: %w", err)
	}

	if _, err := b.pool.Exec(ctx,
		`INSERT INTO relationships (namespace_id, from_entity, relation_type, canonical_type, to_entity, confidence, source_fact_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		nsID, rel.FromEntity, rel.RelationType, strPtrOrNull(rel.CanonicalType), rel.ToEntity, rel.Confidence, factID,
	); err != nil {
		return false, fmt.Errorf("insert relationship: %w", err)
	}
	return true, nil
}

// --- Stage 3.5: Facts -> Causal Links ---

func (b *Brain) consolidateFactsToCausalLinks(ctx context.Context, nsID int64, cp *models.ConsolidationProgress) (count, llmCalls int, errs []string) {
	sql, args, err := b.queries.FetchFacts(nsID, cp.LastCausalFactID, 30)
	if err != nil {
		errs = append(errs, fmt.Sprintf("build fetch facts for causal: %v", err))
		return
	}

	rows, err := b.pool.Query(ctx, sql, args...)
	if err != nil {
		errs = append(errs, fmt.Sprintf("fetch facts for causal: %v", err))
		return
	}
	defer rows.Close()

	var facts []models.Fact
	for rows.Next() {
		var f models.Fact
		if err := rows.Scan(&f.ID, &f.NamespaceID, &f.Content, &f.Embedding, &f.EmbeddingModel, &f.Confidence, &f.Entity, &f.Property, &f.Value, &f.ValidFrom, &f.ValidUntil, &f.CreatedAt, &f.UpdatedAt); err != nil {
			errs = append(errs, fmt.Sprintf("scan fact for causal: %v", err))
			continue
		}
		facts = append(facts, f)
	}
	if err := rows.Err(); err != nil {
		errs = append(errs, fmt.Sprintf("fact rows for causal: %v", err))
		return
	}

	if len(facts) < 2 {
		return
	}

	llmCalls++
	found, detectErrs := b.DetectCausalLinks(ctx, nsID, facts)
	count = found
	errs = append(errs, detectErrs...)

	// Own checkpoint: sharing LastFactID with the relationships stage meant that
	// stage had already advanced it, so this stage saw (almost) no new facts.
	if len(detectErrs) == 0 {
		for _, f := range facts {
			if f.ID > cp.LastCausalFactID {
				cp.LastCausalFactID = f.ID
			}
		}
	}
	return
}

// --- Stage 3: Facts + Relationships -> Patterns ---

func (b *Brain) consolidateToPatterns(ctx context.Context, nsID int64, cp *models.ConsolidationProgress) (count, llmCalls int, errs []string) {
	// Fetch new facts since last pattern extraction
	factSQL, factArgs, err := b.queries.FetchFacts(nsID, cp.LastPatternFactID, 30)
	if err != nil {
		errs = append(errs, fmt.Sprintf("build fetch facts for patterns: %v", err))
		return
	}

	factRows, err := b.pool.Query(ctx, factSQL, factArgs...)
	if err != nil {
		errs = append(errs, fmt.Sprintf("fetch facts for patterns: %v", err))
		return
	}
	defer factRows.Close()

	var facts []models.Fact
	for factRows.Next() {
		var f models.Fact
		if err := factRows.Scan(&f.ID, &f.NamespaceID, &f.Content, &f.Embedding, &f.EmbeddingModel, &f.Confidence, &f.Entity, &f.Property, &f.Value, &f.ValidFrom, &f.ValidUntil, &f.CreatedAt, &f.UpdatedAt); err != nil {
			errs = append(errs, fmt.Sprintf("scan fact for pattern: %v", err))
			continue
		}
		facts = append(facts, f)
	}
	if err := factRows.Err(); err != nil {
		errs = append(errs, fmt.Sprintf("fact rows for patterns: %v", err))
		return
	}

	if len(facts) == 0 {
		return
	}

	// Fetch new relationships since last pattern extraction
	relSQL, relArgs, err := b.queries.FetchRelationships(nsID, cp.LastPatternRelID, 50)
	if err != nil {
		errs = append(errs, fmt.Sprintf("build fetch rels for patterns: %v", err))
		return
	}

	relRows, err := b.pool.Query(ctx, relSQL, relArgs...)
	if err != nil {
		errs = append(errs, fmt.Sprintf("fetch rels for patterns: %v", err))
		return
	}
	defer relRows.Close()

	var rels []models.Relationship
	for relRows.Next() {
		var r models.Relationship
		if err := relRows.Scan(&r.ID, &r.NamespaceID, &r.FromEntity, &r.RelationType, &r.ToEntity, &r.Confidence, &r.SourceFactID, &r.CreatedAt); err != nil {
			errs = append(errs, fmt.Sprintf("scan rel for pattern: %v", err))
			continue
		}
		rels = append(rels, r)
	}
	if err := relRows.Err(); err != nil {
		errs = append(errs, fmt.Sprintf("rel rows for patterns: %v", err))
	}

	if len(rels) == 0 && len(facts) < 3 {
		// Not enough data for pattern extraction
		b.updatePatternCheckpoint(ctx, cp, facts, rels, len(errs) == 0)
		return
	}

	// Call reasoner for pattern extraction
	patterns, err := b.reasoner.ReasonPatterns(ctx, facts, rels)
	llmCalls++
	if err != nil {
		errs = append(errs, fmt.Sprintf("reason patterns: %v", err))
		// Don't update checkpoint on error
		return
	}

	for _, p := range patterns {
		if p.Content == "" {
			continue
		}

		// Confidence = min(source confidences) * coherence_score
		confidence := p.CoherenceScore
		if len(p.SourceFactIDs) > 0 || len(p.SourceRelIDs) > 0 {
			minConf := float32(1.0)
			for _, fid := range p.SourceFactIDs {
				for _, f := range facts {
					if f.ID == fid && f.Confidence < minConf {
						minConf = f.Confidence
					}
				}
			}
			for _, rid := range p.SourceRelIDs {
				for _, r := range rels {
					if r.ID == rid && r.Confidence < minConf {
						minConf = r.Confidence
					}
				}
			}
			confidence = minConf * p.CoherenceScore
		}

		// If no source IDs provided, use all facts/rels as sources
		sourceFactIDs := p.SourceFactIDs
		sourceRelIDs := p.SourceRelIDs
		if len(sourceFactIDs) == 0 && len(sourceRelIDs) == 0 {
			sourceFactIDs = make([]int64, len(facts))
			for i, f := range facts {
				sourceFactIDs[i] = f.ID
			}
			sourceRelIDs = make([]int64, len(rels))
			for i, r := range rels {
				sourceRelIDs[i] = r.ID
			}
		}

		// A NOT NULL array column takes an empty array, not NULL: a pattern that
		// came with facts but no relations (or the other way round) would
		// otherwise reach the database as a nil slice, which pgx sends as NULL.
		sourceFactIDs = nonNilIDs(sourceFactIDs)
		sourceRelIDs = nonNilIDs(sourceRelIDs)

		_, err := b.pool.Exec(ctx,
			`INSERT INTO patterns (namespace_id, content, confidence, source_fact_ids, source_rel_ids, coherence_score)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			nsID, p.Content, confidence, sourceFactIDs, sourceRelIDs, p.CoherenceScore,
		)
		if err != nil {
			errs = append(errs, fmt.Sprintf("insert pattern: %v", err))
			continue
		}
		count++
	}

	// Only update checkpoint if no errors occurred (bullet-proof: prevents losing patterns)
	b.updatePatternCheckpoint(ctx, cp, facts, rels, len(errs) == 0)
	return
}

func (b *Brain) updatePatternCheckpoint(ctx context.Context, cp *models.ConsolidationProgress, facts []models.Fact, rels []models.Relationship, success bool) {
	// Only advance checkpoint if no errors occurred
	if !success {
		return
	}
	for _, f := range facts {
		if f.ID > cp.LastPatternFactID {
			cp.LastPatternFactID = f.ID
		}
	}
	for _, r := range rels {
		if r.ID > cp.LastPatternRelID {
			cp.LastPatternRelID = r.ID
		}
	}
}
