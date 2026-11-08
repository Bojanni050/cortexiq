// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/jackc/pgx/v5"
)

var (
	ErrGoalNotFound  = fmt.Errorf("brain: goal not found")
	ErrGoalNotActive = fmt.Errorf("brain: goal is not active")
)

const goalColumns = `id, namespace_id, parent_id, content, status, priority, notes,
completed_at, abandoned_at, created_at, updated_at, deleted_at`

func scanGoal(h *models.Goal, row pgx.Row) error {
	return row.Scan(
		&h.ID, &h.NamespaceID, &h.ParentID, &h.Content, &h.Status, &h.Priority, &h.Notes,
		&h.CompletedAt, &h.AbandonedAt, &h.CreatedAt, &h.UpdatedAt, &h.DeletedAt,
	)
}

func scanGoalRows(rows pgx.Rows) ([]models.Goal, error) {
	var result []models.Goal
	for rows.Next() {
		var g models.Goal
		if err := rows.Scan(
			&g.ID, &g.NamespaceID, &g.ParentID, &g.Content, &g.Status, &g.Priority, &g.Notes,
			&g.CompletedAt, &g.AbandonedAt, &g.CreatedAt, &g.UpdatedAt, &g.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan goal: %w", err)
		}
		result = append(result, g)
	}
	return result, rows.Err()
}

// CreateGoal creates a new goal in active status.
func (b *Brain) CreateGoal(ctx context.Context, nsID int64, content string, parentID *int64, priority int) (*models.Goal, error) {
	if content == "" {
		return nil, ErrEmptyContent
	}

	if parentID != nil {
		parent, err := b.GetGoal(ctx, *parentID)
		if err != nil {
			return nil, fmt.Errorf("check parent goal: %w", err)
		}
		if parent.Status != "active" {
			return nil, fmt.Errorf("%w: parent goal %d is %s, must be active", ErrGoalNotActive, *parentID, parent.Status)
		}
	}

	var g models.Goal
	err := b.pool.QueryRow(ctx,
		`INSERT INTO goals (namespace_id, parent_id, content, priority)
		 VALUES ($1, $2, $3, $4)
		 RETURNING `+goalColumns,
		nsID, parentID, content, priority,
	).Scan(
		&g.ID, &g.NamespaceID, &g.ParentID, &g.Content, &g.Status, &g.Priority, &g.Notes,
		&g.CompletedAt, &g.AbandonedAt, &g.CreatedAt, &g.UpdatedAt, &g.DeletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create goal: %w", err)
	}
	if err := recordGoalEvent(ctx, b.pool, goalEvent{
		GoalID: g.ID, NamespaceID: g.NamespaceID, Event: GoalEventCreated,
		StatusTo: strPtr(g.Status),
	}); err != nil {
		return nil, err
	}
	return &g, nil
}

// goalWhere builds the filter ListGoals and CountGoals share, so the total can
// never describe a different set than the page. Without a status, only top-level
// goals are listed.
func goalWhere(nsIDs []int64, status string, parentID *int64) (string, []any) {
	where := "namespace_id = ANY($1) AND deleted_at IS NULL"
	args := []any{nsIDs}

	if status != "" {
		args = append(args, status)
		where += fmt.Sprintf(" AND status = $%d", len(args))
	}

	if parentID != nil {
		args = append(args, *parentID)
		where += fmt.Sprintf(" AND parent_id = $%d", len(args))
	} else if status == "" {
		where += " AND parent_id IS NULL"
	}
	return where, args
}

// CountGoals counts the goals ListGoals would return for the same filters,
// ignoring paging.
func (b *Brain) CountGoals(ctx context.Context, namespaceSlugs []string, status string, parentID *int64) (int, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return 0, err
	}
	where, args := goalWhere(nsIDs, status, parentID)
	return b.countWhere(ctx, "goals", where, args...)
}

// ListGoals returns goals across namespaces, optionally filtered by status and parent.
func (b *Brain) ListGoals(ctx context.Context, namespaceSlugs []string, status string, parentID *int64, page Pagination) ([]models.Goal, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}

	page = page.Sanitize()

	where, args := goalWhere(nsIDs, status, parentID)
	query := `SELECT ` + goalColumns + ` FROM goals WHERE ` + where +
		fmt.Sprintf(" ORDER BY priority DESC, created_at ASC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, page.Limit, page.Offset)

	rows, err := b.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list goals: %w", err)
	}
	defer rows.Close()
	goals, err := scanGoalRows(rows)
	if err != nil {
		return nil, err
	}
	if err := b.attachLatestEvents(ctx, goals); err != nil {
		return nil, err
	}
	return goals, nil
}

// GetGoal returns a single goal by ID.
func (b *Brain) GetGoal(ctx context.Context, id int64) (*models.Goal, error) {
	var g models.Goal
	err := b.pool.QueryRow(ctx,
		`SELECT `+goalColumns+` FROM goals WHERE id = $1`, id,
	).Scan(
		&g.ID, &g.NamespaceID, &g.ParentID, &g.Content, &g.Status, &g.Priority, &g.Notes,
		&g.CompletedAt, &g.AbandonedAt, &g.CreatedAt, &g.UpdatedAt, &g.DeletedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrGoalNotFound
		}
		return nil, fmt.Errorf("get goal: %w", err)
	}
	latest, err := b.latestGoalEvents(ctx, []int64{g.ID})
	if err != nil {
		return nil, err
	}
	setLatestEvent(&g, latest)
	return &g, nil
}

// GetGoalProgress returns sub-goal counts for a parent goal.
func (b *Brain) GetGoalProgress(ctx context.Context, id int64) (total, completed int, err error) {
	err = b.pool.QueryRow(ctx,
		`SELECT COUNT(*) FILTER (WHERE status IN ('active', 'completed')),
		        COUNT(*) FILTER (WHERE status = 'completed')
		 FROM goals WHERE parent_id = $1 AND deleted_at IS NULL`,
		id,
	).Scan(&total, &completed)
	if err != nil {
		return 0, 0, fmt.Errorf("get goal progress: %w", err)
	}
	return total, completed, nil
}

// namespaceCovers reports whether slug is ns itself or a descendant of it. A
// goal filed at /projects is closed from a /projects/x session; the reverse would
// let a write reach into a namespace it cannot see.
func namespaceCovers(ns, slug string) bool {
	if ns == "/" {
		return true
	}
	return slug == ns || strings.HasPrefix(slug, ns+"/")
}

// checkGoalCompletable resolves the goal a remember call asked to close and
// refuses it early: before the embedding is paid for and before anything is
// written, so an unknown or already closed goal cannot leave a memory behind that
// claims to have finished it.
func (b *Brain) checkGoalCompletable(ctx context.Context, goalID int64, namespaceSlug string) error {
	var status, slug string
	err := b.pool.QueryRow(ctx,
		`SELECT g.status, n.slug FROM goals g JOIN namespaces n ON n.id = g.namespace_id
		 WHERE g.id = $1 AND g.deleted_at IS NULL`, goalID,
	).Scan(&status, &slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: goal %d", ErrGoalNotFound, goalID)
	}
	if err != nil {
		return fmt.Errorf("look up goal: %w", err)
	}
	if status != "active" {
		return fmt.Errorf("%w: goal %d is %s, must be active", ErrGoalNotActive, goalID, status)
	}
	if !namespaceCovers(slug, namespaceSlug) {
		return fmt.Errorf("goal %d is filed in %q, which does not cover %q", goalID, slug, namespaceSlug)
	}
	return nil
}

// CompleteGoal marks a goal as completed. If all siblings are completed, auto-completes the parent.
func (b *Brain) CompleteGoal(ctx context.Context, id int64, notes string) (*models.Goal, error) {
	return b.CompleteGoalWithEvidence(ctx, id, notes, nil)
}

// CompleteGoalWithEvidence is CompleteGoal plus the memory that proves it. The
// episode that closed the goal is stored with the event, so "why is this done"
// is a link away instead of something to reconstruct from timestamps. A nil
// episode is a completion without evidence (the dashboard, or a caller that only
// knows the goal).
func (b *Brain) CompleteGoalWithEvidence(ctx context.Context, id int64, notes string, episodeID *int64) (*models.Goal, error) {
	current, err := b.GetGoal(ctx, id)
	if err != nil {
		return nil, err
	}

	if current.Status != "active" {
		return nil, fmt.Errorf("%w: goal %d is %s, must be active", ErrGoalNotActive, id, current.Status)
	}

	now := time.Now().UTC()

	var g models.Goal
	err = b.pool.QueryRow(ctx,
		`UPDATE goals SET status = 'completed', completed_at = $2, notes = CASE WHEN $3 = '' THEN notes ELSE $3 END, updated_at = $2
		 WHERE id = $1
		 RETURNING `+goalColumns,
		id, now, notes,
	).Scan(
		&g.ID, &g.NamespaceID, &g.ParentID, &g.Content, &g.Status, &g.Priority, &g.Notes,
		&g.CompletedAt, &g.AbandonedAt, &g.CreatedAt, &g.UpdatedAt, &g.DeletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("complete goal: %w", err)
	}

	if err := recordGoalEvent(ctx, b.pool, goalEvent{
		GoalID: g.ID, NamespaceID: g.NamespaceID, Event: GoalEventCompleted,
		StatusFrom: strPtr(current.Status), StatusTo: strPtr(g.Status),
		Note: strPtr(notes), EpisodeID: episodeID,
	}); err != nil {
		return &g, err
	}

	if g.ParentID != nil {
		if err := b.autoCompleteParent(ctx, *g.ParentID); err != nil {
			return &g, nil
		}
	}

	// The event list is decoration on a completed goal; a failure to read it must
	// not report the completion itself as failed.
	if latest, err := b.latestGoalEvents(ctx, []int64{g.ID}); err == nil {
		setLatestEvent(&g, latest)
	}
	return &g, nil
}

func (b *Brain) autoCompleteParent(ctx context.Context, parentID int64) error {
	var total, completed int
	err := b.pool.QueryRow(ctx,
		`SELECT COUNT(*) FILTER (WHERE status IN ('active', 'completed')),
		        COUNT(*) FILTER (WHERE status = 'completed')
		 FROM goals WHERE parent_id = $1 AND deleted_at IS NULL`,
		parentID,
	).Scan(&total, &completed)
	if err != nil {
		return err
	}

	if total > 0 && total == completed {
		now := time.Now().UTC()
		tag, err := b.pool.Exec(ctx,
			`UPDATE goals SET status = 'completed', completed_at = $2, updated_at = $2 WHERE id = $1 AND status = 'active'`,
			parentID, now,
		)
		if err != nil {
			return err
		}
		// Only log when the parent actually moved: the guard above can hold while
		// a concurrent run already completed it.
		if tag.RowsAffected() > 0 {
			var nsID int64
			if err := b.pool.QueryRow(ctx, `SELECT namespace_id FROM goals WHERE id = $1`, parentID).Scan(&nsID); err == nil {
				_ = recordGoalEvent(ctx, b.pool, goalEvent{
					GoalID: parentID, NamespaceID: nsID, Event: GoalEventCompleted,
					StatusFrom: strPtr("active"), StatusTo: strPtr("completed"),
					Detail: map[string]any{"reason": "all_subgoals_completed"},
				})
			}
		}

		var grandparentID *int64
		err = b.pool.QueryRow(ctx,
			"SELECT parent_id FROM goals WHERE id = $1", parentID,
		).Scan(&grandparentID)
		if err != nil {
			return err
		}
		if grandparentID != nil {
			return b.autoCompleteParent(ctx, *grandparentID)
		}
	}

	return nil
}

// AbandonGoal marks a goal as abandoned.
func (b *Brain) AbandonGoal(ctx context.Context, id int64, notes string) (*models.Goal, error) {
	current, err := b.GetGoal(ctx, id)
	if err != nil {
		return nil, err
	}

	if current.Status != "active" {
		return nil, fmt.Errorf("%w: goal %d is %s, must be active", ErrGoalNotActive, id, current.Status)
	}

	now := time.Now().UTC()

	var g models.Goal
	err = b.pool.QueryRow(ctx,
		`UPDATE goals SET status = 'abandoned', abandoned_at = $2, notes = CASE WHEN $3 = '' THEN notes ELSE $3 END, updated_at = $2
		 WHERE id = $1
		 RETURNING `+goalColumns,
		id, now, notes,
	).Scan(
		&g.ID, &g.NamespaceID, &g.ParentID, &g.Content, &g.Status, &g.Priority, &g.Notes,
		&g.CompletedAt, &g.AbandonedAt, &g.CreatedAt, &g.UpdatedAt, &g.DeletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("abandon goal: %w", err)
	}
	if err := recordGoalEvent(ctx, b.pool, goalEvent{
		GoalID: g.ID, NamespaceID: g.NamespaceID, Event: GoalEventAbandoned,
		StatusFrom: strPtr(current.Status), StatusTo: strPtr(g.Status), Note: strPtr(notes),
	}); err != nil {
		return &g, err
	}
	if latest, err := b.latestGoalEvents(ctx, []int64{g.ID}); err == nil {
		setLatestEvent(&g, latest)
	}
	return &g, nil
}

// UpdateGoal updates content, priority, and notes of an active goal.
func (b *Brain) UpdateGoal(ctx context.Context, id int64, content string, priority int, notes string) (*models.Goal, error) {
	current, err := b.GetGoal(ctx, id)
	if err != nil {
		return nil, err
	}

	if current.Status != "active" {
		return nil, fmt.Errorf("%w: goal %d is %s, must be active", ErrGoalNotActive, id, current.Status)
	}

	if content == "" {
		content = current.Content
	}
	if priority == 0 {
		priority = current.Priority
	}
	if notes == "" {
		notes = current.Notes
	}

	now := time.Now().UTC()

	var g models.Goal
	err = b.pool.QueryRow(ctx,
		`UPDATE goals SET content = $2, priority = $3, notes = $4, updated_at = $5
		 WHERE id = $1
		 RETURNING `+goalColumns,
		id, content, priority, notes, now,
	).Scan(
		&g.ID, &g.NamespaceID, &g.ParentID, &g.Content, &g.Status, &g.Priority, &g.Notes,
		&g.CompletedAt, &g.AbandonedAt, &g.CreatedAt, &g.UpdatedAt, &g.DeletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("update goal: %w", err)
	}
	return &g, nil
}

// DeleteGoal soft-deletes a goal by ID. Children cascade via FK.
func (b *Brain) DeleteGoal(ctx context.Context, id int64) error {
	tag, err := b.pool.Exec(ctx,
		"UPDATE goals SET deleted_at = now(), updated_at = now() WHERE id = $1 AND deleted_at IS NULL",
		id,
	)
	if err != nil {
		return fmt.Errorf("delete goal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrGoalNotFound
	}
	return nil
}
