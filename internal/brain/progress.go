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
