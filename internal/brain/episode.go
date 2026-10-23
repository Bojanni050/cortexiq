// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/pgvector/pgvector-go"
)

// Remember stores a new episode in the given namespace.
// If occurredAt is nil, the current time is used.
// Returns the episode ID on success.
func (b *Brain) Remember(ctx context.Context, namespaceSlug, content string, occurredAt *time.Time) (int64, error) {
	return b.RememberWith(ctx, namespaceSlug, content, RememberOptions{
		OccurredAt: occurredAt,
		Importance: DefaultImportance,
	})
}

// DefaultImportance is the neutral importance of a memory nobody rated.
const DefaultImportance float32 = 0.5

// RememberOptions carries the optional attributes of a memory. Importance has
// no "unset" value (0 is a legitimate rating), so callers that do not rate a
// memory pass DefaultImportance explicitly.
type RememberOptions struct {
	OccurredAt *time.Time
	Importance float32
	// Tags describe what a memory is about ("beslissing", "bugfix",
	// "klant-x"). They are orthogonal to the namespace and normalized by
	// NormalizeTags. Facts consolidated from this episode inherit them.
	Tags []string
}

// RememberWithImportance is Remember with an explicit importance in 0..1
// (0.5 is neutral). Importance only matters when recall importance weighting is
// enabled (CORTEXIQ_RECALL_IMPORTANCE_WEIGHT); facts consolidated from the episode
// inherit the highest importance of their source episodes.
func (b *Brain) RememberWithImportance(ctx context.Context, namespaceSlug, content string, occurredAt *time.Time, importance float32) (int64, error) {
	return b.RememberWith(ctx, namespaceSlug, content, RememberOptions{
		OccurredAt: occurredAt,
		Importance: importance,
	})
}

// RememberWith stores a new episode with all optional attributes set at once.
func (b *Brain) RememberWith(ctx context.Context, namespaceSlug, content string, opts RememberOptions) (int64, error) {
	if err := requireWrite(ctx); err != nil {
		return 0, err
	}
	if err := validateContent(content); err != nil {
		return 0, err
	}
	importance := opts.Importance
	if importance < 0 || importance > 1 || importance != importance {
		return 0, fmt.Errorf("importance must be between 0 and 1, got %v", importance)
	}
	tags, err := NormalizeTags(opts.Tags)
	if err != nil {
		return 0, err
	}
	// The social filter is the one gate in front of every write path (MCP,
	// HTTP, CLI), so the store never fills with "dank je wel" in the first
	// place; see socialCloser. It is per-namespace overridable: a chat-like
	// namespace may want to keep short exchanges. The lookup needs the
	// namespace to exist, so it runs after the ID is resolved below.
	socialFilter := b.config.SocialFilter

	if err := validatePath(namespaceSlug); err != nil {
		return 0, err
	}

	nsID, err := b.resolveNamespaceID(ctx, namespaceSlug)
	if err != nil {
		return 0, err
	}

	if eff, err := b.EffectiveNamespaceConfig(ctx, namespaceSlug); err == nil && eff != nil {
		socialFilter = eff.SocialFilter()
	}
	if socialFilter && socialCloser(content) {
		return 0, ErrSocialCloser
	}

	occurred := time.Now().UTC()
	if opts.OccurredAt != nil {
		occurred = *opts.OccurredAt
	}

	vec, err := b.embedder.Embed(ctx, content)
	if err != nil {
		return 0, fmt.Errorf("embed: %w", err)
	}

	var id int64
	err = b.pool.QueryRow(ctx,
		`INSERT INTO episodes (namespace_id, content, embedding, embedding_model, occurred_at, importance, tags)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		nsID, content, pgvector.NewVector(vec), b.embedder.Model(), occurred, importance, tagsOrEmpty(tags),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert episode: %w", err)
	}
	// Let the consolidator pick this up right away, unless this is an eval
	// write: /eval/ episodes exist to measure recall and must not turn into
	// facts while the evaluation is still running.
	if !strings.HasPrefix(namespaceSlug, "/eval/") {
		b.noteEpisode(nsID)
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

// CountEpisodes counts the episodes ListEpisodes would return, ignoring paging.
// tags narrows the count the same way it narrows the list (AND).
func (b *Brain) CountEpisodes(ctx context.Context, namespaceSlugs []string, tags []string) (int, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return 0, err
	}
	where, args := tagWhere("namespace_id = ANY($1) AND deleted_at IS NULL", []any{nsIDs}, tags)
	return b.countWhere(ctx, "episodes", where, args...)
}

// GetEpisode returns a single episode by ID.
func (b *Brain) GetEpisode(ctx context.Context, episodeID int64) (*models.Episode, error) {
	var e models.Episode
	err := b.pool.QueryRow(ctx,
		`SELECT id, namespace_id, content, embedding, embedding_model, occurred_at, created_at, deleted_at, tags
		 FROM episodes WHERE id = $1`,
		episodeID,
	).Scan(&e.ID, &e.NamespaceID, &e.Content, &e.Embedding, &e.EmbeddingModel, &e.OccurredAt, &e.CreatedAt, &e.DeletedAt, &e.Tags)
	if err != nil {
		return nil, ErrEpisodeNotFound
	}
	return &e, nil
}

// ListEpisodes returns the most recent episodes for the given namespaces, newest
// first. tags narrows the list to episodes carrying every listed tag (AND).
func (b *Brain) ListEpisodes(ctx context.Context, namespaceSlugs []string, tags []string, page Pagination) ([]models.Episode, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}
	page = page.Sanitize()
	where, args := tagWhere("namespace_id = ANY($1) AND deleted_at IS NULL", []any{nsIDs}, tags)
	query := `SELECT id, namespace_id, content, embedding_model, occurred_at, created_at, tags
		 FROM episodes WHERE ` + where +
		fmt.Sprintf(" ORDER BY occurred_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, page.Limit, page.Offset)
	rows, err := b.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list episodes: %w", err)
	}
	defer rows.Close()

	var out []models.Episode
	for rows.Next() {
		var e models.Episode
		if err := rows.Scan(&e.ID, &e.NamespaceID, &e.Content, &e.EmbeddingModel, &e.OccurredAt, &e.CreatedAt, &e.Tags); err != nil {
			return nil, fmt.Errorf("scan episode: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
