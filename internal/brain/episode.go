// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/pgvector/pgvector-go"
)

// Remember stores a new episode in the given namespace.
// If occurredAt is nil, the current time is used.
// Returns the episode ID on success.
func (b *Brain) Remember(ctx context.Context, namespaceSlug, content string, occurredAt *time.Time) (int64, error) {
	return b.RememberWithImportance(ctx, namespaceSlug, content, occurredAt, DefaultImportance)
}

// DefaultImportance is the neutral importance of a memory nobody rated.
const DefaultImportance float32 = 0.5

// RememberWithImportance is Remember with an explicit importance in 0..1
// (0.5 is neutral). Importance only matters when recall importance weighting is
// enabled (CORTEXIQ_RECALL_IMPORTANCE_WEIGHT); facts consolidated from the episode
// inherit the highest importance of their source episodes.
func (b *Brain) RememberWithImportance(ctx context.Context, namespaceSlug, content string, occurredAt *time.Time, importance float32) (int64, error) {
	if err := requireWrite(ctx); err != nil {
		return 0, err
	}
	if err := validateContent(content); err != nil {
		return 0, err
	}
	if importance < 0 || importance > 1 || importance != importance {
		return 0, fmt.Errorf("importance must be between 0 and 1, got %v", importance)
	}
	if err := validatePath(namespaceSlug); err != nil {
		return 0, err
	}

	nsID, err := b.resolveNamespaceID(ctx, namespaceSlug)
	if err != nil {
		return 0, err
	}

	occurred := time.Now().UTC()
	if occurredAt != nil {
		occurred = *occurredAt
	}

	vec, err := b.embedder.Embed(ctx, content)
	if err != nil {
		return 0, fmt.Errorf("embed: %w", err)
	}

	var id int64
	err = b.pool.QueryRow(ctx,
		`INSERT INTO episodes (namespace_id, content, embedding, embedding_model, occurred_at, importance)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		nsID, content, pgvector.NewVector(vec), b.embedder.Model(), occurred, importance,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert episode: %w", err)
	}
	return id, nil
}

// ForgetEpisode soft-deletes the episode that best matches the query across the given namespaces.
// If namespaceSlugs is empty, searches all namespaces.
func (b *Brain) ForgetEpisode(ctx context.Context, namespaceSlugs []string, query string) error {
	if err := validateContent(query); err != nil {
		return err
	}
	for _, slug := range namespaceSlugs {
		if err := validatePath(slug); err != nil {
			return err
		}
	}

	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return err
	}

	vec, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}

	var id int64
	if len(nsIDs) == 1 {
		err = b.pool.QueryRow(ctx,
			`SELECT id FROM episodes
			 WHERE namespace_id = $1 AND deleted_at IS NULL AND embedding IS NOT NULL
			 ORDER BY embedding <=> $2 LIMIT 1`,
			nsIDs[0], pgvector.NewVector(vec),
		).Scan(&id)
	} else {
		err = b.pool.QueryRow(ctx,
			`SELECT id FROM episodes
			 WHERE namespace_id = ANY($1) AND deleted_at IS NULL AND embedding IS NOT NULL
			 ORDER BY embedding <=> $2 LIMIT 1`,
			nsIDs, pgvector.NewVector(vec),
		).Scan(&id)
	}
	if err != nil {
		return ErrEpisodeNotFound
	}

	_, err = b.pool.Exec(ctx,
		"UPDATE episodes SET deleted_at = now() WHERE id = $1", id,
	)
	if err != nil {
		return fmt.Errorf("soft delete episode: %w", err)
	}
	return nil
}

// PurgeEpisode hard-deletes an episode by ID.
func (b *Brain) PurgeEpisode(ctx context.Context, episodeID int64) error {
	tag, err := b.pool.Exec(ctx, "DELETE FROM episodes WHERE id = $1", episodeID)
	if err != nil {
		return fmt.Errorf("purge episode: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEpisodeNotFound
	}
	return nil
}

// GetEpisode returns a single episode by ID.
func (b *Brain) GetEpisode(ctx context.Context, episodeID int64) (*models.Episode, error) {
	var e models.Episode
	err := b.pool.QueryRow(ctx,
		`SELECT id, namespace_id, content, embedding, embedding_model, occurred_at, created_at, deleted_at
		 FROM episodes WHERE id = $1`,
		episodeID,
	).Scan(&e.ID, &e.NamespaceID, &e.Content, &e.Embedding, &e.EmbeddingModel, &e.OccurredAt, &e.CreatedAt, &e.DeletedAt)
	if err != nil {
		return nil, ErrEpisodeNotFound
	}
	return &e, nil
}

// ListEpisodes returns the most recent episodes for the given namespaces, newest first.
func (b *Brain) ListEpisodes(ctx context.Context, namespaceSlugs []string, page Pagination) ([]models.Episode, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}
	page = page.Sanitize()
	rows, err := b.pool.Query(ctx,
		`SELECT id, namespace_id, content, embedding_model, occurred_at, created_at
		 FROM episodes WHERE namespace_id = ANY($1) AND deleted_at IS NULL
		 ORDER BY occurred_at DESC LIMIT $2 OFFSET $3`,
		nsIDs, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("list episodes: %w", err)
	}
	defer rows.Close()

	var out []models.Episode
	for rows.Next() {
		var e models.Episode
		if err := rows.Scan(&e.ID, &e.NamespaceID, &e.Content, &e.EmbeddingModel, &e.OccurredAt, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan episode: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
