// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Bojanni050/cortexiq/internal/models"
)

// Event names in the goal log. They are the vocabulary of the dashboard tab and
// the MCP tool, so they stay stable.
const (
	GoalEventCreated           = "created"
	GoalEventProgress          = "progress"
	GoalEventSuggestedComplete = "suggested_complete"
	GoalEventContradicted      = "contradicted"
	GoalEventCompleted         = "completed"
	GoalEventAbandoned         = "abandoned"
)

// goalEvent is one change of a goal waiting to be written.
type goalEvent struct {
	GoalID      int64
	NamespaceID int64
	Event       string
	StatusFrom  *string
	StatusTo    *string
	Note        *string
	Confidence  *float32
	FactID      *int64
	EpisodeID   *int64
	Detail      any
}

// recordGoalEvent appends one entry. It takes an execer so the same call writes
// inside a transaction (complete with its evidence) and outside one (the
// consolidator's inference).
func recordGoalEvent(ctx context.Context, db execer, ev goalEvent) error {
	var detail []byte
	if ev.Detail != nil {
		b, err := json.Marshal(ev.Detail)
		if err != nil {
			return fmt.Errorf("marshal goal event detail: %w", err)
		}
		detail = b
	}
	_, err := db.Exec(ctx,
		`INSERT INTO goal_events
		 (goal_id, namespace_id, event, actor, status_from, status_to,
		  note, confidence, fact_id, episode_id, detail)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		ev.GoalID, ev.NamespaceID, ev.Event, actorFrom(ctx), ev.StatusFrom, ev.StatusTo,
		ev.Note, ev.Confidence, ev.FactID, ev.EpisodeID, detail,
	)
	if err != nil {
		return fmt.Errorf("record goal event: %w", err)
	}
	return nil
}

// goalEventWhere builds the filter the list and the count share. prefix is the
// table alias ("e." for the joined list query, empty for the count).
func goalEventWhere(prefix string, nsIDs []int64, goalID int64) (string, []any) {
	where := prefix + "namespace_id = ANY($1)"
	args := []any{nsIDs}
	if goalID > 0 {
		args = append(args, goalID)
		where += fmt.Sprintf(" AND %sgoal_id = $%d", prefix, len(args))
	}
	return where, args
}

// ListGoalEvents returns the log newest first. An unknown namespace is an empty
// log, not an error, which is what the dashboard expects while a slug is still
// being typed.
func (b *Brain) ListGoalEvents(ctx context.Context, namespaceSlugs []string, goalID int64, page Pagination) ([]models.GoalEvent, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}
	page = page.Sanitize()

	where, args := goalEventWhere("e.", nsIDs, goalID)
	query := `SELECT e.id, e.goal_id, e.namespace_id, e.event, e.actor, e.status_from, e.status_to,
	          e.note, e.confidence, e.fact_id, e.episode_id, e.detail, e.created_at,
	          COALESCE(g.content, '')
	          FROM goal_events e LEFT JOIN goals g ON g.id = e.goal_id
	          WHERE ` + where +
		fmt.Sprintf(" ORDER BY e.created_at DESC, e.id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, page.Limit, page.Offset)

	rows, err := b.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list goal events: %w", err)
	}
	defer rows.Close()

	var result []models.GoalEvent
	for rows.Next() {
		var e models.GoalEvent
		if err := rows.Scan(&e.ID, &e.GoalID, &e.NamespaceID, &e.Event, &e.Actor,
			&e.StatusFrom, &e.StatusTo, &e.Note, &e.Confidence,
			&e.FactID, &e.EpisodeID, &e.Detail, &e.CreatedAt, &e.GoalContent); err != nil {
			return nil, fmt.Errorf("scan goal event: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// CountGoalEvents counts what ListGoalEvents would return for the same filter,
// ignoring paging.
func (b *Brain) CountGoalEvents(ctx context.Context, namespaceSlugs []string, goalID int64) (int, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return 0, err
	}
	where, args := goalEventWhere("", nsIDs, goalID)
	return b.countWhere(ctx, "goal_events", where, args...)
}

// latestGoalEvents returns the newest entry per goal with one query, so a list
// costs one extra round trip instead of one per goal. A goal without history is
// simply absent from the map.
//
// It returns the map instead of filling Goal values in place because a caller
// holding a single goal would pass a one-element slice literal, and writing
// through that slice header reaches a copy — the visible bug was a goal that
// carried no event right after its own completion.
func (b *Brain) latestGoalEvents(ctx context.Context, ids []int64) (map[int64]models.GoalEvent, error) {
	latest := make(map[int64]models.GoalEvent, len(ids))
	if len(ids) == 0 {
		return latest, nil
	}

	rows, err := b.pool.Query(ctx,
		`SELECT DISTINCT ON (goal_id)
		        goal_id, id, namespace_id, event, actor, status_from, status_to,
		        note, confidence, fact_id, episode_id, detail, created_at
		 FROM goal_events
		 WHERE goal_id = ANY($1)
		 ORDER BY goal_id, id DESC`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch latest goal events: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var e models.GoalEvent
		if err := rows.Scan(&e.GoalID, &e.ID, &e.NamespaceID, &e.Event, &e.Actor,
			&e.StatusFrom, &e.StatusTo, &e.Note, &e.Confidence,
			&e.FactID, &e.EpisodeID, &e.Detail, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan latest goal event: %w", err)
		}
		latest[e.GoalID] = e
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("latest goal event rows: %w", err)
	}
	return latest, nil
}

// attachLatestEvents fills Goal.LatestEvent for a batch of goals the caller owns.
func (b *Brain) attachLatestEvents(ctx context.Context, goals []models.Goal) error {
	if len(goals) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(goals))
	for _, g := range goals {
		ids = append(ids, g.ID)
	}
	latest, err := b.latestGoalEvents(ctx, ids)
	if err != nil {
		return err
	}
	for i := range goals {
		setLatestEvent(&goals[i], latest)
	}
	return nil
}

// setLatestEvent copies the newest entry onto one goal, if there is one.
func setLatestEvent(g *models.Goal, latest map[int64]models.GoalEvent) {
	if e, ok := latest[g.ID]; ok {
		ev := e
		g.LatestEvent = &ev
	}
}
