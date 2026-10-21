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

// CreateNamespace creates a new namespace with the given slug, name, and description.
// Parent namespaces are auto-created with slug as name if they don't exist.
func (b *Brain) CreateNamespace(ctx context.Context, slug, name, description string) (int64, error) {
	if err := requireWrite(ctx); err != nil {
		return 0, err
	}
	if err := validatePath(slug); err != nil {
		return 0, err
	}

	segments := splitPath(slug)
	if len(segments) == 0 {
		var id int64
		err := b.pool.QueryRow(ctx,
			"INSERT INTO namespaces (slug, name) VALUES ('/', '/') ON CONFLICT (slug) DO UPDATE SET updated_at = now() RETURNING id",
		).Scan(&id)
		if err != nil {
			return 0, fmt.Errorf("create root namespace: %w", err)
		}
		return id, nil
	}

	currentPath := ""
	for i, seg := range segments {
		currentPath += "/" + seg
		if i < len(segments)-1 {
			var id int64
			_ = b.pool.QueryRow(ctx,
				"INSERT INTO namespaces (slug, name) VALUES ($1, $1) ON CONFLICT (slug) DO UPDATE SET updated_at = now() RETURNING id",
				currentPath,
			).Scan(&id)
		}
	}

	var id int64
	err := b.pool.QueryRow(ctx,
		"INSERT INTO namespaces (slug, name, description) VALUES ($1, $2, $3) ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description, updated_at = now() RETURNING id",
		slug, name, description,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create namespace: %w", err)
	}
	return id, nil
}

var (
	// ErrNamespaceExists means the target path already belongs to a namespace.
	ErrNamespaceExists = errors.New("brain: namespace already exists — pick another path or use the existing one")
	// ErrRenameRoot means the root namespace cannot be renamed.
	ErrRenameRoot = errors.New("brain: the root namespace cannot be renamed")
	// ErrRenameIntoDescendant means the new path lies inside the namespace being renamed.
	ErrRenameIntoDescendant = errors.New("brain: cannot rename a namespace into one of its own descendants")
)

// RenameResult reports what a rename moved.
type RenameResult struct {
	ID          int64  `json:"id"`
	From        string `json:"from"`
	To          string `json:"to"`
	Descendants int64  `json:"descendants"`
}

// RenameNamespace moves a namespace and all of its descendants to a new path.
//
// Only the slug (the path) changes. A display name or description that was set
// deliberately stays, but a name that still equals the old slug follows the path,
// because those rows were auto-named from their slug. Missing parents of the new
// path are created — the same mkdir -p behavior as CreateNamespace.
//
// No data moves: episodes, facts, goals and the rest point at the namespace ID,
// not at the path. The old path stops working for callers that hardcode it.
func (b *Brain) RenameNamespace(ctx context.Context, from, to string) (*RenameResult, error) {
	if err := requireWrite(ctx); err != nil {
		return nil, err
	}
	if err := validateRename(from, to); err != nil {
		return nil, err
	}

	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("rename namespace: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id int64
	err = tx.QueryRow(ctx, "SELECT id FROM namespaces WHERE slug = $1 FOR UPDATE", from).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNamespaceNotFound
		}
		return nil, fmt.Errorf("rename namespace: lookup %s: %w", from, err)
	}

	var taken bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM namespaces WHERE slug = $1)", to).Scan(&taken); err != nil {
		return nil, fmt.Errorf("rename namespace: check target: %w", err)
	}
	if taken {
		return nil, fmt.Errorf("%w: %s", ErrNamespaceExists, to)
	}

	for _, parent := range parentPaths(to) {
		if _, err := tx.Exec(ctx,
			"INSERT INTO namespaces (slug, name) VALUES ($1, $1) ON CONFLICT (slug) DO NOTHING",
			parent,
		); err != nil {
			return nil, fmt.Errorf("rename namespace: create parent %s: %w", parent, err)
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE namespaces
		    SET slug = $2,
		        name = CASE WHEN name = $1 THEN $2 ELSE name END,
		        updated_at = now()
		  WHERE id = $3`,
		from, to, id,
	); err != nil {
		return nil, fmt.Errorf("rename namespace: move %s: %w", from, err)
	}

	tag, err := tx.Exec(ctx,
		`UPDATE namespaces
		    SET slug = $2 || substring(slug FROM length($1) + 1),
		        updated_at = now()
		  WHERE slug LIKE $3 ESCAPE '\'`,
		from, to, likePatternForDescendants(from),
	)
	if err != nil {
		return nil, fmt.Errorf("rename namespace: move descendants of %s: %w", from, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("rename namespace: commit: %w", err)
	}
	return &RenameResult{ID: id, From: from, To: to, Descendants: tag.RowsAffected()}, nil
}

// validateRename rejects a rename before any SQL runs.
func validateRename(from, to string) error {
	if err := validatePath(from); err != nil {
		return fmt.Errorf("from %q: %w", from, err)
	}
	if err := validatePath(to); err != nil {
		return fmt.Errorf("to %q: %w", to, err)
	}
	if from == "/" || to == "/" {
		return ErrRenameRoot
	}
	if from == to {
		return fmt.Errorf("brain: from and to are the same path: %s", from)
	}
	if strings.HasPrefix(to, from+"/") {
		return fmt.Errorf("%w: %s lies inside %s", ErrRenameIntoDescendant, to, from)
	}
	return nil
}

// parentPaths returns every ancestor of path, shallowest first, excluding the
// path itself and the root.
func parentPaths(path string) []string {
	segments := splitPath(path)
	var out []string
	current := ""
	for i := 0; i < len(segments)-1; i++ {
		current += "/" + segments[i]
		out = append(out, current)
	}
	return out
}

// GetNamespace returns a namespace by slug.
func (b *Brain) GetNamespace(ctx context.Context, slug string) (*models.Namespace, error) {
	var ns models.Namespace
	err := b.pool.QueryRow(ctx,
		"SELECT id, slug, name, description, created_at, updated_at FROM namespaces WHERE slug = $1",
		slug,
	).Scan(&ns.ID, &ns.Slug, &ns.Name, &ns.Description, &ns.CreatedAt, &ns.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNamespaceNotFound
		}
		return nil, fmt.Errorf("get namespace: %w", err)
	}
	return &ns, nil
}

// CountNamespaces counts the namespaces ListNamespaces would return for the same
// filter, ignoring paging.
func (b *Brain) CountNamespaces(ctx context.Context, slugs []string) (int, error) {
	if len(slugs) == 0 {
		return b.countWhere(ctx, "namespaces", "")
	}
	ids, err := b.resolveNamespaceIDs(ctx, slugs)
	if err != nil {
		return 0, err
	}
	return b.countWhere(ctx, "namespaces", "id = ANY($1)", ids)
}

// ListNamespaces returns namespaces, optionally filtered by slug paths.
// If slugs is empty, returns all namespaces.
// Each path matches itself and all descendants.
func (b *Brain) ListNamespaces(ctx context.Context, slugs []string, page Pagination) ([]models.Namespace, error) {
	page = page.Sanitize()

	if len(slugs) == 0 {
		rows, err := b.pool.Query(ctx,
			"SELECT id, slug, name, description, created_at, updated_at FROM namespaces ORDER BY slug LIMIT $1 OFFSET $2",
			page.Limit, page.Offset,
		)
		if err != nil {
			return nil, fmt.Errorf("list namespaces: %w", err)
		}
		defer rows.Close()

		var result []models.Namespace
		for rows.Next() {
			var ns models.Namespace
			if err := rows.Scan(&ns.ID, &ns.Slug, &ns.Name, &ns.Description, &ns.CreatedAt, &ns.UpdatedAt); err != nil {
				return nil, fmt.Errorf("scan namespace: %w", err)
			}
			result = append(result, ns)
		}
		return result, rows.Err()
	}

	ids, err := b.resolveNamespaceIDs(ctx, slugs)
	if err != nil {
		return nil, err
	}

	rows, err := b.pool.Query(ctx,
		"SELECT id, slug, name, description, created_at, updated_at FROM namespaces WHERE id = ANY($1) ORDER BY slug LIMIT $2 OFFSET $3",
		ids, page.Limit, page.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}
	defer rows.Close()

	var result []models.Namespace
	for rows.Next() {
		var ns models.Namespace
		if err := rows.Scan(&ns.ID, &ns.Slug, &ns.Name, &ns.Description, &ns.CreatedAt, &ns.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan namespace: %w", err)
		}
		result = append(result, ns)
	}
	return result, rows.Err()
}

// GetOrCreateConsolidationProgress returns progress for a namespace, creating a row if needed.
func (b *Brain) GetOrCreateConsolidationProgress(ctx context.Context, namespaceID int64) (*models.ConsolidationProgress, error) {
	var cp models.ConsolidationProgress
	err := b.pool.QueryRow(ctx,
		`INSERT INTO consolidation_progress (namespace_id) VALUES ($1)
		 ON CONFLICT (namespace_id) DO UPDATE SET updated_at = consolidation_progress.updated_at
		 RETURNING namespace_id, last_episode_id, last_fact_id, last_relationship_id, last_pattern_fact_id, last_pattern_rel_id, last_goal_progress_fact_id, last_failure_id, last_failure_episode_id, last_hypothesis_fact_id, last_causal_fact_id, last_decay_run, last_run, updated_at`,
		namespaceID,
	).Scan(&cp.NamespaceID, &cp.LastEpisodeID, &cp.LastFactID, &cp.LastRelationshipID, &cp.LastPatternFactID, &cp.LastPatternRelID, &cp.LastGoalProgressFactID, &cp.LastFailureID, &cp.LastFailureEpisodeID, &cp.LastHypothesisFactID, &cp.LastCausalFactID, &cp.LastDecayRun, &cp.LastRun, &cp.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get consolidation progress: %w", err)
	}
	return &cp, nil
}

// SaveConsolidationProgress updates the checkpoint for a namespace.
func (b *Brain) SaveConsolidationProgress(ctx context.Context, cp models.ConsolidationProgress) error {
	now := time.Now().UTC()
	_, err := b.pool.Exec(ctx,
		`UPDATE consolidation_progress SET
			last_episode_id = $2, last_fact_id = $3, last_relationship_id = $4,
			last_pattern_fact_id = $5, last_pattern_rel_id = $6,
			last_goal_progress_fact_id = $7, last_failure_id = $8, last_failure_episode_id = $9, last_hypothesis_fact_id = $10,
			last_causal_fact_id = $11, last_decay_run = $12, last_run = $13, updated_at = $14
		 WHERE namespace_id = $1`,
		cp.NamespaceID, cp.LastEpisodeID, cp.LastFactID, cp.LastRelationshipID,
		cp.LastPatternFactID, cp.LastPatternRelID,
		cp.LastGoalProgressFactID, cp.LastFailureID, cp.LastFailureEpisodeID, cp.LastHypothesisFactID,
		cp.LastCausalFactID, cp.LastDecayRun, now, now,
	)
	if err != nil {
		return fmt.Errorf("save consolidation progress: %w", err)
	}
	return nil
}

// DeleteNamespace removes a namespace and all its data (episodes, facts, etc.
// cascade). It refuses to delete the root namespace "/".
func (b *Brain) DeleteNamespace(ctx context.Context, slug string) error {
	if err := requireWrite(ctx); err != nil {
		return err
	}
	if err := validatePath(slug); err != nil {
		return err
	}
	if slug == "/" {
		return fmt.Errorf("brain: cannot delete the root namespace")
	}
	_, err := b.pool.Exec(ctx, "DELETE FROM namespaces WHERE slug = $1", slug)
	if err != nil {
		return fmt.Errorf("delete namespace: %w", err)
	}
	return nil
}
