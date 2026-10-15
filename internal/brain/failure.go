// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/jackc/pgx/v5"
)

var ErrFailureNotFound = fmt.Errorf("brain: failure not found")

const failureColumns = `id, namespace_id, goal_id, content, reason, lesson, created_at, deleted_at`

// CreateFailure records what didn't work, why, and what to do instead.
func (b *Brain) CreateFailure(ctx context.Context, nsID int64, content, reason, lesson string, goalID *int64) (*models.Failure, error) {
	if content == "" {
		return nil, ErrEmptyContent
	}
	if reason == "" {
		return nil, fmt.Errorf("brain: failure reason is required")
	}
	if lesson == "" {
		return nil, fmt.Errorf("brain: failure lesson is required")
	}

	var f models.Failure
	err := b.pool.QueryRow(ctx,
		`INSERT INTO failures (namespace_id, goal_id, content, reason, lesson)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+failureColumns,
		nsID, goalID, content, reason, lesson,
	).Scan(&f.ID, &f.NamespaceID, &f.GoalID, &f.Content, &f.Reason, &f.Lesson, &f.CreatedAt, &f.DeletedAt)
	if err != nil {
		return nil, fmt.Errorf("create failure: %w", err)
	}
	return &f, nil
}

// failureWhere builds the filter ListFailures and CountFailures share.
func failureWhere(nsIDs []int64, goalID *int64) (string, []any) {
	where := "namespace_id = ANY($1) AND deleted_at IS NULL"
	args := []any{nsIDs}
	if goalID != nil {
		args = append(args, *goalID)
		where += fmt.Sprintf(" AND goal_id = $%d", len(args))
	}
	return where, args
}

// CountFailures counts the failures ListFailures would return for the same
// filter, ignoring paging.
func (b *Brain) CountFailures(ctx context.Context, namespaceSlugs []string, goalID *int64) (int, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return 0, err
	}
	where, args := failureWhere(nsIDs, goalID)
	return b.countWhere(ctx, "failures", where, args...)
}

// ListFailures returns failures across namespaces, optionally filtered by goal.
func (b *Brain) ListFailures(ctx context.Context, namespaceSlugs []string, goalID *int64, page Pagination) ([]models.Failure, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}

	page = page.Sanitize()

	where, args := failureWhere(nsIDs, goalID)
	query := `SELECT ` + failureColumns + ` FROM failures WHERE ` + where +
		fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, page.Limit, page.Offset)

	rows, err := b.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list failures: %w", err)
	}
	defer rows.Close()

	var result []models.Failure
	for rows.Next() {
		var f models.Failure
		if err := rows.Scan(&f.ID, &f.NamespaceID, &f.GoalID, &f.Content, &f.Reason, &f.Lesson, &f.CreatedAt, &f.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan failure: %w", err)
		}
		result = append(result, f)
	}
	return result, rows.Err()
}

// GetFailure returns a single failure by ID.
func (b *Brain) GetFailure(ctx context.Context, id int64) (*models.Failure, error) {
	var f models.Failure
	err := b.pool.QueryRow(ctx,
		`SELECT `+failureColumns+` FROM failures WHERE id = $1`, id,
	).Scan(&f.ID, &f.NamespaceID, &f.GoalID, &f.Content, &f.Reason, &f.Lesson, &f.CreatedAt, &f.DeletedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrFailureNotFound
		}
		return nil, fmt.Errorf("get failure: %w", err)
	}
	return &f, nil
}

// DeleteFailure soft-deletes a failure by ID.
func (b *Brain) DeleteFailure(ctx context.Context, id int64) error {
	tag, err := b.pool.Exec(ctx,
		"UPDATE failures SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL",
		id,
	)
	if err != nil {
		return fmt.Errorf("delete failure: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrFailureNotFound
	}
	return nil
}
