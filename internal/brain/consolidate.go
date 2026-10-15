// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/Bojanni050/cortexiq/internal/observability"
	"github.com/Bojanni050/cortexiq/internal/reasoner"
	"github.com/pgvector/pgvector-go"
)

// ConsolidationResult describes the outcome of a consolidation run.
type ConsolidationResult struct {
	Namespace                  string        `json:"namespace"`
	Duration                   time.Duration `json:"duration"`
	EpisodesRead               int           `json:"episodes_read"`
	FactsCreated               int           `json:"facts_created"`
	FactsDeduplicated          int           `json:"facts_deduplicated"`
	RelationshipsFound         int           `json:"relationships_found"`
	CausalLinksFound           int           `json:"causal_links_found"`
	PatternsFound              int           `json:"patterns_found"`
	ContradictionsFound        int           `json:"contradictions_found"`
	ContradictionsAutoResolved int           `json:"contradictions_auto_resolved"`
	GoalsAnnotated             int           `json:"goals_annotated"`
	GoalsSuggestedComplete     int           `json:"goals_suggested_complete"`
	FailureRepeatsDetected     int           `json:"failure_repeats_detected"`
	FailurePatternsFound       int           `json:"failure_patterns_found"`
	HypothesesAutoConfirmed    int           `json:"hypotheses_auto_confirmed"`
	HypothesesAutoRejected     int           `json:"hypotheses_auto_rejected"`
	HypothesesUpdated          int           `json:"hypotheses_updated"`
	FactsDecayed               int           `json:"facts_decayed"`
	FactsExpired               int           `json:"facts_expired"`
	LLMCalls                   int           `json:"llm_calls"`
	LLMCallsLimited            bool          `json:"llm_calls_limited"`
	Errors                     []string      `json:"errors,omitempty"`
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
	ctx = withLLMBudget(ctx, b.config.MaxLLMCallsPerRun)

	var namespaceSlug string
	_ = b.pool.QueryRow(ctx, "SELECT slug FROM namespaces WHERE id = $1", nsID).Scan(&namespaceSlug)

	result := ConsolidationResult{Namespace: namespaceSlug}

	cp, err := b.GetOrCreateConsolidationProgress(ctx, nsID)
	if err != nil {
		return result, fmt.Errorf("get progress: %w", err)
	}

	// Stage 1: Episodes -> Facts (+ Stage 4: Contradiction detection)
	if ctx.Err() == nil && !llmBudgetExhausted(ctx) {
		factsCreated, factsDeduped, episodesRead, llmCalls, contFound, contAuto, errs := b.consolidateEpisodesToFacts(ctx, nsID, cp)
		result.FactsCreated = factsCreated
		result.FactsDeduplicated = factsDeduped
		result.EpisodesRead = episodesRead
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

func (b *Brain) consolidateEpisodesToFacts(ctx context.Context, nsID int64, cp *models.ConsolidationProgress) (created, deduped, read, llmCalls, contradictionsFound, contradictionsAutoResolved int, errs []string) {
	sql, args, err := b.queries.FetchEpisodes(nsID, cp.LastEpisodeID, b.config.BatchSize)
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

	// Cluster by vector similarity
	clusters := b.clusterEpisodes(episodes)

	// done holds episodes whose cluster was fully handled. The checkpoint only
	// advances over a contiguous run of done episodes, so a failed cluster is
	// retried next run without blocking (or re-processing) everything before it.
	done := make(map[int64]bool)
	markDone := func(cluster []models.Episode) {
		for _, e := range cluster {
			done[e.ID] = true
		}
	}
	scanErrs := len(errs)

	subject := b.namespaceSubject(ctx, nsID)

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
			r, err := b.storeFact(ctx, nsID, sf, episodeIDs)
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
			contradictionsFound += r.contradictions
			contradictionsAutoResolved += r.autoResolved
		}
		if clusterOK {
			markDone(cluster)
		}
	}

	// Rows that failed to scan are missing from `episodes`; advancing past them
	// would lose them, so only move the checkpoint when the fetch was clean.
	if scanErrs == 0 {
		ids := make([]int64, len(episodes))
		for i, e := range episodes {
			ids[i] = e.ID
		}
		cp.LastEpisodeID = contiguousCheckpoint(ids, done, cp.LastEpisodeID)
	}
	return
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

func (b *Brain) clusterEpisodes(episodes []models.Episode) [][]models.Episode {
	if len(episodes) == 0 {
		return nil
	}

	clustered := make(map[int64]bool)
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

		seedVec := seed.Embedding.Slice()
		for _, candidate := range episodes {
			if clustered[candidate.ID] {
				continue
			}
			candVec := candidate.Embedding.Slice()
			if candVec == nil {
				continue
			}
			sim := cosineSimilarity(seedVec, candVec)
			if sim >= float32(b.config.SimilarityThreshold) {
				cluster = append(cluster, candidate)
				clustered[candidate.ID] = true
			}
		}

		clusters = append(clusters, cluster)
	}

	return clusters
}

// storeResult describes what storeFact did with one synthesized fact.
type storeResult struct {
	created, deduped bool
	contradictions   int
	autoResolved     int
	llmCalls         int
}

// storeFact embeds one synthesized fact and either folds it into an existing
// duplicate (reinforcing that one) or inserts it, links its source episodes and
// checks it for contradictions.
func (b *Brain) storeFact(ctx context.Context, nsID int64, sf *reasoner.StructuredFact, episodeIDs []int64) (storeResult, error) {
	var r storeResult

	vec, err := b.embedder.Embed(ctx, sf.Summary)
	if err != nil {
		return r, fmt.Errorf("embed fact: %w", err)
	}

	dupID, dup, reviewCalls, err := b.findDuplicate(ctx, nsID, vec, sf.Summary)
	r.llmCalls += reviewCalls
	if err != nil {
		return r, fmt.Errorf("check duplicate: %w", err)
	}
	if dup {
		b.reinforceFact(ctx, dupID, episodeIDs)
		r.deduped = true
		return r, nil
	}

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

				// Check for existing relationship from this fact
				exists, _ := b.relationshipExists(ctx, nsID, rel.FromEntity, rel.RelationType, rel.ToEntity, fact.ID)
				if exists {
					continue
				}

				_, err := b.pool.Exec(ctx,
					`INSERT INTO relationships (namespace_id, from_entity, relation_type, to_entity, confidence, source_fact_id)
					 VALUES ($1, $2, $3, $4, $5, $6)`,
					nsID, rel.FromEntity, rel.RelationType, rel.ToEntity, rel.Confidence, fact.ID,
				)
				if err != nil {
					errs = append(errs, fmt.Sprintf("insert relationship: %v", err))
					batchOK = false
					continue
				}
				count++
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

func (b *Brain) relationshipExists(ctx context.Context, nsID int64, from, relType, to string, sourceFactID int64) (bool, error) {
	var id int64
	err := b.pool.QueryRow(ctx,
		`SELECT id FROM relationships
		 WHERE namespace_id = $1 AND from_entity = $2 AND relation_type = $3 AND to_entity = $4
		 AND source_fact_id = $5 AND deleted_at IS NULL LIMIT 1`,
		nsID, from, relType, to, sourceFactID,
	).Scan(&id)
	if err != nil {
		return false, nil
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
