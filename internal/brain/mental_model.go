// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

var ErrMentalModelNotFound = fmt.Errorf("brain: mental model not found")

const mentalModelColumns = `id, namespace_id, name, source_query, content, embedding,
	embedding_model, tags, max_tokens, last_refreshed_at, created_at, updated_at, deleted_at`

// MentalModelSearchResult is a mental model hit from a similarity search.
type MentalModelSearchResult struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	Content         string     `json:"content"`
	Tags            []string   `json:"tags"`
	Relevance       float32    `json:"relevance"`
	LastRefreshedAt *time.Time `json:"last_refreshed_at,omitempty"`
	IsStale         bool       `json:"is_stale"`
}

// CreateMentalModelOptions configures a new mental model.
type CreateMentalModelOptions struct {
	Name        string
	SourceQuery string
	Tags        []string
	MaxTokens   int
}

func scanMentalModel(m *models.MentalModel, row pgx.Row) error {
	return row.Scan(
		&m.ID, &m.NamespaceID, &m.Name, &m.SourceQuery, &m.Content, &m.Embedding,
		&m.EmbeddingModel, &m.Tags, &m.MaxTokens, &m.LastRefreshedAt,
		&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt,
	)
}

// CreateMentalModel creates a mental model and generates its content by running
// a reflect call on the source query. The mental model is embedded on its
// source query for semantic search.
func (b *Brain) CreateMentalModel(ctx context.Context, nsID int64, opts CreateMentalModelOptions) (*models.MentalModel, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return nil, fmt.Errorf("brain: mental model name cannot be empty")
	}
	query := strings.TrimSpace(opts.SourceQuery)
	if query == "" {
		return nil, fmt.Errorf("brain: mental model source query cannot be empty")
	}
	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}

	nsSlug, err := b.namespaceSlug(ctx, nsID)
	if err != nil {
		return nil, err
	}

	reflectResult, err := b.Reflect(ctx, query, ReflectOptions{
		Namespaces: []string{nsSlug},
		Limit:      20,
		MaxTokens:  maxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("generate mental model content: %w", err)
	}

	vec, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed mental model query: %w", err)
	}

	now := time.Now()
	var m models.MentalModel
	err = b.pool.QueryRow(ctx, `
		INSERT INTO mental_models (namespace_id, name, source_query, content, embedding, embedding_model, tags, max_tokens, last_refreshed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+mentalModelColumns, nsID, name, query, reflectResult.Answer,
		pgvector.NewVector(vec), b.embedder.Model(), opts.Tags, maxTokens, now).Scan(
		&m.ID, &m.NamespaceID, &m.Name, &m.SourceQuery, &m.Content, &m.Embedding,
		&m.EmbeddingModel, &m.Tags, &m.MaxTokens, &m.LastRefreshedAt,
		&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert mental model: %w", err)
	}
	return &m, nil
}

// GetMentalModel returns a mental model by ID.
func (b *Brain) GetMentalModel(ctx context.Context, id int64) (*models.MentalModel, error) {
	var m models.MentalModel
	err := scanMentalModel(&m, b.pool.QueryRow(ctx,
		`SELECT `+mentalModelColumns+` FROM mental_models WHERE id = $1 AND deleted_at IS NULL`, id))
	if err == pgx.ErrNoRows {
		return nil, ErrMentalModelNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get mental model: %w", err)
	}
	return &m, nil
}

// ListMentalModels returns mental models in the given namespaces, optionally
// filtered by tags (any match).
func (b *Brain) ListMentalModels(ctx context.Context, namespaceSlugs []string, tags []string, page Pagination) ([]models.MentalModel, error) {
	nsIDs, err := b.ResolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}
	if len(nsIDs) == 0 {
		return nil, nil
	}
	page = page.Sanitize()

	where := "deleted_at IS NULL AND namespace_id = ANY($1)"
	args := []any{nsIDs}
	if len(tags) > 0 {
		where += " AND tags && $2"
		args = append(args, tags)
	}

	rows, err := b.pool.Query(ctx,
		`SELECT `+mentalModelColumns+` FROM mental_models WHERE `+where+` ORDER BY created_at DESC LIMIT $`+fmt.Sprint(len(args)+1)+` OFFSET $`+fmt.Sprint(len(args)+2),
		append(args, page.Limit, page.Offset)...)
	if err != nil {
		return nil, fmt.Errorf("list mental models: %w", err)
	}
	defer rows.Close()

	var result []models.MentalModel
	for rows.Next() {
		var m models.MentalModel
		if err := rows.Scan(
			&m.ID, &m.NamespaceID, &m.Name, &m.SourceQuery, &m.Content, &m.Embedding,
			&m.EmbeddingModel, &m.Tags, &m.MaxTokens, &m.LastRefreshedAt,
			&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan mental model: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// CountMentalModels counts mental models matching the given filters.
func (b *Brain) CountMentalModels(ctx context.Context, namespaceSlugs []string, tags []string) (int, error) {
	nsIDs, err := b.ResolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return 0, err
	}
	if len(nsIDs) == 0 {
		return 0, nil
	}
	where := "deleted_at IS NULL AND namespace_id = ANY($1)"
	args := []any{nsIDs}
	if len(tags) > 0 {
		where += " AND tags && $2"
		args = append(args, tags)
	}
	return b.countWhere(ctx, "mental_models", where, args...)
}

// SearchMentalModels finds mental models by semantic similarity to the query.
// Results are ordered by relevance. Tags filter (any match) when provided.
func (b *Brain) SearchMentalModels(ctx context.Context, namespaceSlugs []string, query string, limit int, tags []string) ([]MentalModelSearchResult, error) {
	nsIDs, err := b.ResolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}
	if len(nsIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}

	vec, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed mental model query: %w", err)
	}

	where := "deleted_at IS NULL AND namespace_id = ANY($1) AND embedding IS NOT NULL"
	args := []any{nsIDs, pgvector.NewVector(vec)}
	argIdx := 3
	if len(tags) > 0 {
		where += fmt.Sprintf(" AND tags && $%d", argIdx)
		args = append(args, tags)
		argIdx++
	}
	args = append(args, limit)

	rows, err := b.pool.Query(ctx, `
		SELECT id, name, content, tags, last_refreshed_at,
		       1 - (embedding <=> $2) AS relevance
		FROM mental_models
		WHERE `+where+`
		ORDER BY embedding <=> $2
		LIMIT $`+fmt.Sprint(argIdx), args...)
	if err != nil {
		return nil, fmt.Errorf("search mental models: %w", err)
	}
	defer rows.Close()

	var results []MentalModelSearchResult
	for rows.Next() {
		var r MentalModelSearchResult
		if err := rows.Scan(&r.ID, &r.Name, &r.Content, &r.Tags, &r.LastRefreshedAt, &r.Relevance); err != nil {
			return nil, fmt.Errorf("scan mental model search: %w", err)
		}
		r.IsStale = isMentalModelStale(r.LastRefreshedAt)
		results = append(results, r)
	}
	return results, rows.Err()
}

// RefreshMentalModel regenerates the content of a mental model by re-running
// the reflect call on its source query, then re-embedding the query.
func (b *Brain) RefreshMentalModel(ctx context.Context, id int64) (*models.MentalModel, error) {
	m, err := b.GetMentalModel(ctx, id)
	if err != nil {
		return nil, err
	}

	nsSlug, err := b.namespaceSlug(ctx, m.NamespaceID)
	if err != nil {
		return nil, err
	}

	reflectResult, err := b.Reflect(ctx, m.SourceQuery, ReflectOptions{
		Namespaces: []string{nsSlug},
		Limit:      20,
		MaxTokens:  m.MaxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("refresh mental model content: %w", err)
	}

	now := time.Now()
	var updated models.MentalModel
	err = b.pool.QueryRow(ctx, `
		UPDATE mental_models
		SET content = $1, last_refreshed_at = $2, updated_at = $3
		WHERE id = $4 AND deleted_at IS NULL
		RETURNING `+mentalModelColumns,
		reflectResult.Answer, now, now, id).Scan(
		&updated.ID, &updated.NamespaceID, &updated.Name, &updated.SourceQuery, &updated.Content,
		&updated.Embedding, &updated.EmbeddingModel, &updated.Tags, &updated.MaxTokens,
		&updated.LastRefreshedAt, &updated.CreatedAt, &updated.UpdatedAt, &updated.DeletedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrMentalModelNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update mental model: %w", err)
	}
	return &updated, nil
}

// DeleteMentalModel soft-deletes a mental model.
func (b *Brain) DeleteMentalModel(ctx context.Context, id int64) error {
	tag, err := b.pool.Exec(ctx,
		`UPDATE mental_models SET deleted_at = now(), updated_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("delete mental model: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMentalModelNotFound
	}
	return nil
}

// UpdateMentalModelTags replaces the tags of a mental model.
func (b *Brain) UpdateMentalModelTags(ctx context.Context, id int64, tags []string) (*models.MentalModel, error) {
	var m models.MentalModel
	err := scanMentalModel(&m, b.pool.QueryRow(ctx, `
		UPDATE mental_models
		SET tags = $1, updated_at = now()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING `+mentalModelColumns, tags, id))
	if err == pgx.ErrNoRows {
		return nil, ErrMentalModelNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update mental model tags: %w", err)
	}
	return &m, nil
}

// namespaceSlug resolves a namespace ID to its slug.
func (b *Brain) namespaceSlug(ctx context.Context, nsID int64) (string, error) {
	var slug string
	err := b.pool.QueryRow(ctx, `SELECT slug FROM namespaces WHERE id = $1`, nsID).Scan(&slug)
	if err == pgx.ErrNoRows {
		return "", ErrNamespaceNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve namespace slug: %w", err)
	}
	return slug, nil
}

// isMentalModelStale reports whether a mental model should be refreshed. A
// model is stale when it has never been refreshed (created but content empty)
// or when it has not been refreshed in over 24 hours. This is a heuristic: the
// real signal is whether new in-scope memories arrived since the last refresh.
func isMentalModelStale(lastRefreshedAt *time.Time) bool {
	if lastRefreshedAt == nil {
		return true
	}
	return time.Since(*lastRefreshedAt) > 24*time.Hour
}
