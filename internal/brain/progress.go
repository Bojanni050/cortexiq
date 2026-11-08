// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"time"
)

// NamespaceProgress says how far consolidation has come in one namespace.
type NamespaceProgress struct {
	NamespaceID int64  `json:"namespace_id"`
	Slug        string `json:"slug"`
	// PendingEpisodes counts episodes stored after the last consolidation
	// checkpoint, i.e. memories that no extractor has looked at yet.
	PendingEpisodes int    `json:"pending_episodes"`
	LastRun         string `json:"last_run,omitempty"`
}

// ConsolidationStatus answers "has my memory been processed yet?" for the
// dashboard: writing a memory stores an episode and the facts only appear once
// consolidation has run over it, which can take minutes and is easy to mistake
// for "it did not work".
type ConsolidationStatus struct {
	PendingEpisodes int                 `json:"pending_episodes"`
	LastRun         string              `json:"last_run,omitempty"`
	Namespaces      []NamespaceProgress `json:"namespaces"`
}

// ConsolidationStatus reports the backlog and the last run per namespace.
func (b *Brain) ConsolidationStatus(ctx context.Context, namespaceSlugs []string) (*ConsolidationStatus, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}

	rows, err := b.pool.Query(ctx,
		`SELECT n.id, n.slug,
		        (SELECT count(*) FROM episodes e
		          WHERE e.namespace_id = n.id AND e.deleted_at IS NULL
		            AND e.id > COALESCE(p.last_episode_id, 0)),
		        p.last_run
		   FROM namespaces n
		   LEFT JOIN consolidation_progress p ON p.namespace_id = n.id
		  WHERE n.id = ANY($1)
		  ORDER BY n.slug`,
		nsIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("consolidation status: %w", err)
	}
	defer rows.Close()

	out := &ConsolidationStatus{Namespaces: []NamespaceProgress{}}
	var newest time.Time
	for rows.Next() {
		var p NamespaceProgress
		var lastRun *time.Time
		if err := rows.Scan(&p.NamespaceID, &p.Slug, &p.PendingEpisodes, &lastRun); err != nil {
			return nil, fmt.Errorf("scan consolidation progress: %w", err)
		}
		if lastRun != nil {
			p.LastRun = lastRun.UTC().Format(time.RFC3339)
			if lastRun.After(newest) {
				newest = *lastRun
			}
		}
		out.PendingEpisodes += p.PendingEpisodes
		out.Namespaces = append(out.Namespaces, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !newest.IsZero() {
		out.LastRun = newest.UTC().Format(time.RFC3339)
	}
	return out, nil
}

// StageStall is one consolidation stage whose checkpoint has not moved while
// work waits behind it. From the outside that is invisible: no run reports an
// error, no counter changes, and every fact after the stuck one silently stops
// being processed — which is exactly what happened when the relationships stage
// kept failing on facts 3028-3032.
type StageStall struct {
	Namespace string `json:"namespace"`
	Stage     string `json:"stage"`
	// Checkpoint is the highest id this stage has consumed.
	Checkpoint int64 `json:"checkpoint"`
	// Pending counts the rows waiting behind that checkpoint.
	Pending int64 `json:"pending"`
	// ChangedAt is when this checkpoint last advanced; empty when it never has.
	ChangedAt string  `json:"changed_at,omitempty"`
	IdleHours float64 `json:"idle_hours"`
	Stalled   bool    `json:"stalled"`
}

// StallAfter is how long a checkpoint may sit still while work waits behind it
// before it counts as stalled. Three scheduled runs is the smallest window that
// cannot be explained by one slow run; the floor keeps a 30-second schedule from
// calling everything stalled.
func (b *Brain) StallAfter(ctx context.Context) time.Duration {
	after := 30 * time.Minute
	cfg, err := b.ScheduleConfig(ctx)
	if err != nil {
		return after
	}
	if d := 3 * cfg.Interval(); d > after {
		after = d
	}
	return after
}

// stageCursor is one stage's checkpoint, used while counting the work behind it.
type stageCursor struct {
	name       string
	checkpoint int64
	// table is the one this stage reads: episodes feed the episode and failure
	// stages, facts feed the rest.
	table string
}

// ConsolidationStageStalls reports the stages that have work waiting but have not
// advanced in a while. A stage with nothing pending is not a stall: an idle
// namespace looks the same as a broken one otherwise.
func (b *Brain) ConsolidationStageStalls(ctx context.Context, namespaceSlugs []string) ([]StageStall, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}
	after := b.StallAfter(ctx)

	rows, err := b.pool.Query(ctx,
		`SELECT n.id, n.slug,
		        COALESCE(p.last_episode_id, 0), COALESCE(p.last_fact_id, 0),
		        COALESCE(p.last_pattern_fact_id, 0), COALESCE(p.last_goal_progress_fact_id, 0),
		        COALESCE(p.last_failure_episode_id, 0), COALESCE(p.last_hypothesis_fact_id, 0),
		        COALESCE(p.last_causal_fact_id, 0),
		        p.stage_changed_at, p.last_run
		   FROM namespaces n
		   LEFT JOIN consolidation_progress p ON p.namespace_id = n.id
		  WHERE n.id = ANY($1)
		  ORDER BY n.slug`,
		nsIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("stage stalls: %w", err)
	}
	defer rows.Close()

	out := []StageStall{}
	for rows.Next() {
		var id int64
		var slug string
		var stages [7]int64
		var changed map[string]string
		var lastRun *time.Time
		if err := rows.Scan(&id, &slug,
			&stages[0], &stages[1], &stages[2], &stages[3],
			&stages[4], &stages[5], &stages[6],
			&changed, &lastRun); err != nil {
			return nil, fmt.Errorf("scan stage stalls: %w", err)
		}

		// Each stage is named after the checkpoint it actually advances. There is no
		// separate "facts" stage: turning episodes into facts moves last_episode_id,
		// and the relationships stage is what consumes new facts and moves last_fact_id.
		specs := []stageCursor{
			{name: "episodes", checkpoint: stages[0], table: "episodes"},
			{name: "relationships", checkpoint: stages[1], table: "facts"},
			{name: "patterns", checkpoint: stages[2], table: "facts"},
			{name: "goals", checkpoint: stages[3], table: "facts"},
			{name: "failures", checkpoint: stages[4], table: "episodes"},
			{name: "hypotheses", checkpoint: stages[5], table: "facts"},
			{name: "causal", checkpoint: stages[6], table: "facts"},
		}

		pending, err := b.pendingBehindCheckpoints(ctx, id, specs)
		if err != nil {
			return nil, err
		}

		for _, s := range specs {
			var since time.Time
			if raw, ok := changed[s.name]; ok {
				if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
					since = t
				}
			}
			if since.IsZero() && lastRun != nil {
				since = *lastRun
			}
			if since.IsZero() {
				continue
			}
			idle := time.Since(since)
			out = append(out, StageStall{
				Namespace:  slug,
				Stage:      s.name,
				Checkpoint: s.checkpoint,
				Pending:    pending[s.name],
				ChangedAt:  since.UTC().Format(time.RFC3339),
				IdleHours:  float64(int(idle.Hours()*10)) / 10,
				Stalled:    pending[s.name] > 0 && idle >= after,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("stage stall rows: %w", err)
	}
	return out, nil
}

// pendingBehindCheckpoints counts the rows waiting behind each stage's
// checkpoint. Episodes feed the episode and failure stages, facts feed the rest,
// which is why the queries differ per stage rather than sharing one.
func (b *Brain) pendingBehindCheckpoints(ctx context.Context, nsID int64, stages []stageCursor) (map[string]int64, error) {
	out := make(map[string]int64, len(stages))
	for _, s := range stages {
		var n int64
		// The table name comes from the fixed list above, never from caller input.
		if err := b.pool.QueryRow(ctx,
			`SELECT count(*) FROM `+s.table+` WHERE namespace_id = $1 AND deleted_at IS NULL AND id > $2`,
			nsID, s.checkpoint,
		).Scan(&n); err != nil {
			return nil, fmt.Errorf("pending behind %s: %w", s.name, err)
		}
		out[s.name] = n
	}
	return out, nil
}
