package brain

import (
	"context"
	"fmt"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/jackc/pgx/v5"
)

// runHistoryRetention bounds how long consolidation runs are kept per namespace.
const runHistoryRetention = 30 * 24 * time.Hour

const runColumns = `id, namespace_id, namespace_slug, started_at, duration_ms,
	episodes_read, episodes_skipped, facts_created, facts_deduplicated, relationships_found,
	causal_links_found, patterns_found, contradictions_found,
	goals_annotated, goals_suggested_complete,
	failure_repeats_detected, failure_patterns_found,
	hypotheses_auto_confirmed, hypotheses_auto_rejected, hypotheses_updated,
	facts_decayed, facts_expired, dedup_near_misses, llm_calls, llm_calls_limited, errors, error_sample, created_at`

// SaveConsolidationRun persists the outcome of one run and trims old history for
// the namespace. It is best effort: callers must not fail consolidation on error.
func (b *Brain) SaveConsolidationRun(ctx context.Context, nsID int64, res ConsolidationResult, startedAt time.Time) error {
	sample := ""
	if len(res.Errors) > 0 {
		sample = res.Errors[0]
		if len(sample) > 500 {
			sample = sample[:500]
		}
	}
	_, err := b.pool.Exec(ctx,
		`INSERT INTO consolidation_runs (
			namespace_id, namespace_slug, started_at, duration_ms,
			episodes_read, episodes_skipped, facts_created, facts_deduplicated, relationships_found,
			causal_links_found, patterns_found, contradictions_found,
			goals_annotated, goals_suggested_complete,
			failure_repeats_detected, failure_patterns_found,
			hypotheses_auto_confirmed, hypotheses_auto_rejected, hypotheses_updated,
			facts_decayed, facts_expired, dedup_near_misses, llm_calls, llm_calls_limited, errors, error_sample)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)`,
		nsID, res.Namespace, startedAt, res.Duration.Milliseconds(),
		res.EpisodesRead, res.EpisodesSkipped, res.FactsCreated, res.FactsDeduplicated, res.RelationshipsFound,
		res.CausalLinksFound, res.PatternsFound, res.ContradictionsFound,
		res.GoalsAnnotated, res.GoalsSuggestedComplete,
		res.FailureRepeatsDetected, res.FailurePatternsFound,
		res.HypothesesAutoConfirmed, res.HypothesesAutoRejected, res.HypothesesUpdated,
		res.FactsDecayed, res.FactsExpired, res.DedupNearMisses, res.LLMCalls, res.LLMCallsLimited, len(res.Errors), sample,
	)
	if err != nil {
		return fmt.Errorf("save consolidation run: %w", err)
	}
	// Trim is best effort; a failure here must not lose the run we just wrote.
	_, _ = b.pool.Exec(ctx,
		`DELETE FROM consolidation_runs WHERE namespace_id = $1 AND started_at < now() - $2::interval`,
		nsID, runHistoryRetention.String(),
	)
	return nil
}

// CountConsolidationRuns counts the runs ListConsolidationRuns would return for
// the same filter, ignoring paging.
func (b *Brain) CountConsolidationRuns(ctx context.Context, namespaceSlugs []string) (int, error) {
	if len(namespaceSlugs) == 0 {
		return b.countWhere(ctx, "consolidation_runs", "")
	}
	ids, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return 0, err
	}
	return b.countWhere(ctx, "consolidation_runs", "namespace_id = ANY($1)", ids)
}

// ListConsolidationRuns returns recent runs, newest first. An empty namespace
// list returns runs across all namespaces.
func (b *Brain) ListConsolidationRuns(ctx context.Context, namespaceSlugs []string, page Pagination) ([]models.ConsolidationRun, error) {
	page = page.Sanitize()

	var err error
	var rows pgx.Rows
	if len(namespaceSlugs) == 0 {
		rows, err = b.pool.Query(ctx,
			`SELECT `+runColumns+` FROM consolidation_runs ORDER BY started_at DESC LIMIT $1 OFFSET $2`,
			page.Limit, page.Offset)
	} else {
		ids, idErr := b.resolveNamespaceIDs(ctx, namespaceSlugs)
		if idErr != nil {
			return nil, idErr
		}
		rows, err = b.pool.Query(ctx,
			`SELECT `+runColumns+` FROM consolidation_runs WHERE namespace_id = ANY($1) ORDER BY started_at DESC LIMIT $2 OFFSET $3`,
			ids, page.Limit, page.Offset)
	}
	if err != nil {
		return nil, fmt.Errorf("list consolidation runs: %w", err)
	}
	defer rows.Close()

	var out []models.ConsolidationRun
	for rows.Next() {
		var run models.ConsolidationRun
		if err := rows.Scan(&run.ID, &run.NamespaceID, &run.NamespaceSlug, &run.StartedAt, &run.DurationMS,
			&run.EpisodesRead, &run.EpisodesSkipped, &run.FactsCreated, &run.FactsDeduplicated, &run.RelationshipsFound,
			&run.CausalLinksFound, &run.PatternsFound, &run.ContradictionsFound,
			&run.GoalsAnnotated, &run.GoalsSuggestedComplete,
			&run.FailureRepeatsDetected, &run.FailurePatternsFound,
			&run.HypothesesAutoConfirmed, &run.HypothesesAutoRejected, &run.HypothesesUpdated,
			&run.FactsDecayed, &run.FactsExpired, &run.DedupNearMisses, &run.LLMCalls, &run.LLMCallsLimited, &run.Errors, &run.ErrorSample, &run.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan consolidation run: %w", err)
		}
		out = append(out, run)
	}
	return out, rows.Err()
}
