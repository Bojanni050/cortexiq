// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
)

// RebuildReport describes the derived layer of one namespace: what rests on what,
// and what a rebuild would re-read. It is the answer to "can I trust the facts,
// and can I get them again from the episodes?".
type RebuildReport struct {
	Namespace string `json:"namespace"`
	// Episodes are the live episodes: the raw record a rebuild reads from.
	Episodes int `json:"episodes"`
	// Facts are the active facts, and FactsByOrigin splits them by the path that
	// wrote them. The key "unknown" holds facts written before origin was recorded.
	Facts         int            `json:"facts"`
	FactsByOrigin map[string]int `json:"facts_by_origin"`
	// FactsWithoutSource are active facts that rest on nothing a reader can follow:
	// no source episode and no origin reference.
	FactsWithoutSource int `json:"facts_without_source"`
	// FactsOnRetiredSources are active facts all of whose source episodes were
	// forgotten or retracted. Nothing live supports them any more.
	FactsOnRetiredSources int `json:"facts_on_retired_sources"`
	// CheckpointEpisodeID is how far consolidation has read. EpisodesBehind is
	// how many live episodes lie at or before it, which is what a rebuild would
	// read again.
	CheckpointEpisodeID int64 `json:"checkpoint_episode_id"`
	EpisodesBehind      int   `json:"episodes_behind_checkpoint"`

	// Applied is true when the checkpoint was reset. FactsEnded counts the facts
	// on retired sources that were ended as well (only when asked for).
	Applied    bool  `json:"applied"`
	FactsEnded int64 `json:"facts_ended,omitempty"`
}

// Rebuild reports on the derived layer of a namespace and, when apply is set,
// prepares a rebuild: it resets the episode checkpoint so the next consolidation
// reads every live episode again.
//
// A rebuild never deletes a fact. Re-reading goes through the same path as any
// consolidation, so duplicates merge into the facts that exist, rejected values
// stay refused (the tombstone is checked in insertFact), and an episode that is
// already a fact's source is not counted as new evidence. What it adds is the
// facts the current extractor finds that the old one did not.
//
// endRetired also ends the active facts whose every source episode was retired.
// It is separate and off by default: ending a fact is a decision, and the report
// shows what it would touch first.
func (b *Brain) Rebuild(ctx context.Context, namespaceSlug string, apply, endRetired bool) (RebuildReport, error) {
	rep := RebuildReport{Namespace: namespaceSlug, FactsByOrigin: map[string]int{}}
	if apply || endRetired {
		if err := requireWrite(ctx); err != nil {
			return rep, err
		}
	}
	if err := validatePath(namespaceSlug); err != nil {
		return rep, err
	}
	nsID, err := b.resolveNamespaceID(ctx, namespaceSlug)
	if err != nil {
		return rep, err
	}
	cp, err := b.GetOrCreateConsolidationProgress(ctx, nsID)
	if err != nil {
		return rep, err
	}
	rep.CheckpointEpisodeID = cp.LastEpisodeID

	if err := b.pool.QueryRow(ctx,
		`SELECT count(*), count(*) FILTER (WHERE id <= $2)
		   FROM episodes WHERE namespace_id = $1 AND deleted_at IS NULL`,
		nsID, cp.LastEpisodeID,
	).Scan(&rep.Episodes, &rep.EpisodesBehind); err != nil {
		return rep, fmt.Errorf("rebuild: count episodes: %w", err)
	}

	rows, err := b.pool.Query(ctx,
		`SELECT COALESCE(origin, 'unknown'), count(*) FROM facts
		  WHERE namespace_id = $1 AND deleted_at IS NULL AND valid_until IS NULL
		  GROUP BY 1`, nsID)
	if err != nil {
		return rep, fmt.Errorf("rebuild: count facts: %w", err)
	}
	for rows.Next() {
		var origin string
		var n int
		if err := rows.Scan(&origin, &n); err != nil {
			rows.Close()
			return rep, fmt.Errorf("rebuild: scan facts: %w", err)
		}
		rep.FactsByOrigin[origin] = n
		rep.Facts += n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return rep, fmt.Errorf("rebuild: count facts: %w", err)
	}

	if err := b.pool.QueryRow(ctx,
		`SELECT
		   count(*) FILTER (WHERE NOT EXISTS (SELECT 1 FROM fact_sources s WHERE s.fact_id = f.id)
		                      AND f.origin_ref IS NULL),
		   count(*) FILTER (WHERE EXISTS (SELECT 1 FROM fact_sources s WHERE s.fact_id = f.id)
		                      AND NOT EXISTS (SELECT 1 FROM fact_sources s
		                                      JOIN episodes e ON e.id = s.episode_id
		                                      WHERE s.fact_id = f.id AND e.deleted_at IS NULL))
		   FROM facts f
		  WHERE f.namespace_id = $1 AND f.deleted_at IS NULL AND f.valid_until IS NULL`, nsID,
	).Scan(&rep.FactsWithoutSource, &rep.FactsOnRetiredSources); err != nil {
		return rep, fmt.Errorf("rebuild: check sources: %w", err)
	}

	if endRetired && rep.FactsOnRetiredSources > 0 {
		tag, err := b.pool.Exec(ctx,
			`UPDATE facts f
			    SET valid_until = COALESCE(f.valid_until, now()), known_until = now(), updated_at = now()
			  WHERE f.namespace_id = $1 AND f.deleted_at IS NULL AND f.valid_until IS NULL
			    AND EXISTS (SELECT 1 FROM fact_sources s WHERE s.fact_id = f.id)
			    AND NOT EXISTS (SELECT 1 FROM fact_sources s
			                    JOIN episodes e ON e.id = s.episode_id
			                    WHERE s.fact_id = f.id AND e.deleted_at IS NULL)`, nsID)
		if err != nil {
			return rep, fmt.Errorf("rebuild: end facts on retired sources: %w", err)
		}
		rep.FactsEnded = tag.RowsAffected()
	}

	if apply {
		if _, err := b.pool.Exec(ctx,
			`UPDATE consolidation_progress SET last_episode_id = 0, updated_at = now() WHERE namespace_id = $1`, nsID,
		); err != nil {
			return rep, fmt.Errorf("rebuild: reset checkpoint: %w", err)
		}
		rep.Applied = true
	}
	return rep, nil
}
