// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"

	"github.com/Bojanni050/cortexiq/internal/models"
)

func (b *Brain) consolidateGoalProgress(ctx context.Context, nsID int64, cp *models.ConsolidationProgress) (annotated, suggestedComplete, llmCalls int, errs []string) {
	rows, err := b.pool.Query(ctx,
		`SELECT id, namespace_id, parent_id, content, status, priority, notes,
		 completed_at, abandoned_at, created_at, updated_at, deleted_at
		 FROM goals WHERE namespace_id = $1 AND status = 'active' AND deleted_at IS NULL`,
		nsID,
	)
	if err != nil {
		errs = append(errs, fmt.Sprintf("fetch active goals: %v", err))
		return
	}
	defer rows.Close()

	goals, err := scanGoalRows(rows)
	if err != nil {
		errs = append(errs, fmt.Sprintf("scan goals: %v", err))
		return
	}

	if len(goals) == 0 {
		return
	}

	factSQL, factArgs, err := b.queries.FetchFacts(nsID, cp.LastGoalProgressFactID, 30)
	if err != nil {
		errs = append(errs, fmt.Sprintf("build fetch facts for goals: %v", err))
		return
	}

	factRows, err := b.pool.Query(ctx, factSQL, factArgs...)
	if err != nil {
		errs = append(errs, fmt.Sprintf("fetch facts for goals: %v", err))
		return
	}
	defer factRows.Close()

	var facts []models.Fact
	for factRows.Next() {
		var f models.Fact
		if err := factRows.Scan(&f.ID, &f.NamespaceID, &f.Content, &f.Embedding, &f.EmbeddingModel, &f.Confidence, &f.Entity, &f.Property, &f.Value, &f.ValidFrom, &f.ValidUntil, &f.CreatedAt, &f.UpdatedAt); err != nil {
			errs = append(errs, fmt.Sprintf("scan fact for goals: %v", err))
			continue
		}
		facts = append(facts, f)
	}
	if err := factRows.Err(); err != nil {
		errs = append(errs, fmt.Sprintf("fact rows for goals: %v", err))
		return
	}

	if len(facts) == 0 {
		return
	}

	llmCalls++
	assessments, err := b.reasoner.ReasonGoalProgress(ctx, goals, facts)
	if err != nil {
		errs = append(errs, fmt.Sprintf("reason goal progress: %v", err))
		return
	}

	var maxFactID int64
	for _, f := range facts {
		if f.ID > maxFactID {
			maxFactID = f.ID
		}
	}

	// The newest entry per goal decides whether this run has anything new to say.
	// The same conclusion drawn from the same facts is not a new observation, and
	// writing it again would add one row per run to a log nobody could read.
	goalIDs := make([]int64, 0, len(goals))
	for _, g := range goals {
		goalIDs = append(goalIDs, g.ID)
	}
	previous, err := b.latestGoalEvents(ctx, goalIDs)
	if err != nil {
		errs = append(errs, fmt.Sprintf("latest goal events: %v", err))
		previous = map[int64]models.GoalEvent{}
	}
	known := make(map[int64]bool, len(goals))
	for i := range goals {
		known[goals[i].ID] = true
	}

	for _, a := range assessments {
		var event string
		switch a.Assessment {
		case "progress":
			event = GoalEventProgress
		case "suggested_complete":
			event = GoalEventSuggestedComplete
		case "contradicted":
			event = GoalEventContradicted
		default:
			// "irrelevant" is the model saying it has nothing to add; so does an
			// unknown goal id, which would only fail the foreign key.
			continue
		}
		if !known[a.GoalID] {
			continue
		}
		if prev, ok := previous[a.GoalID]; ok && prev.Event == event &&
			prev.FactID != nil && *prev.FactID == maxFactID {
			continue
		}

		confidence, factID := a.Confidence, maxFactID
		if err := recordGoalEvent(ctx, b.pool, goalEvent{
			GoalID: a.GoalID, NamespaceID: nsID, Event: event,
			Note: strPtr(a.Note), Confidence: &confidence, FactID: &factID,
		}); err != nil {
			errs = append(errs, fmt.Sprintf("record goal event %d: %v", a.GoalID, err))
			continue
		}
		// Counted only when something was written: the run summary must not claim
		// an annotation that the log does not contain.
		annotated++
		if event == GoalEventSuggestedComplete {
			suggestedComplete++
		}
	}
	if maxFactID > cp.LastGoalProgressFactID {
		cp.LastGoalProgressFactID = maxFactID
	}

	return
}
