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
	id, _, err := b.RememberWithDecision(ctx, namespaceSlug, content, opts)
	return id, err
}

// RememberWithDecision is RememberWith plus the write filter's verdict. The
// returned WriteDecision is always populated (even when the filter is off, in
// which case Action is "store"); callers that want to surface warnings to the
// user (CLI, MCP, dashboard) use this variant, while RememberWith keeps the
// simple (id, error) signature for callers that do not care.
//
// The filter runs BEFORE the embedding call: a write carrying a secret or a
// terminal dump is rejected without paying for an embedding it never needed.
// In warn mode with WriteFilterRedactSecrets the redacted form is stored
// instead of the original, so the secret never reaches the database even when
// the write is allowed through.
func (b *Brain) RememberWithDecision(ctx context.Context, namespaceSlug, content string, opts RememberOptions) (int64, WriteDecision, error) {
	var decision WriteDecision
	if err := requireWrite(ctx); err != nil {
		return 0, decision, err
	}
	if err := validateContent(content); err != nil {
		return 0, decision, err
	}
	importance := opts.Importance
	if importance < 0 || importance > 1 || importance != importance {
		return 0, decision, fmt.Errorf("importance must be between 0 and 1, got %v", importance)
	}
	tags, err := NormalizeTags(opts.Tags)
	if err != nil {
		return 0, decision, err
	}

	// The write filter is the second gate in front of every write path (after
	// requireWrite), so the store never fills with leaked secrets or machine
	// noise. It runs before the social filter and before the embedding call:
	// a rejected write costs nothing.
	decision = classifyWrite(content, importance, b.config.WriteFilter)
	if err := applyWriteDecision(decision); err != nil {
		return 0, decision, err
	}
	if decision.Action == "warn" && decision.Redacted != "" && b.config.WriteFilterRedactSecrets {
		content = decision.Redacted
	}

	// The social filter is the one gate in front of every write path (MCP,
	// HTTP, CLI), so the store never fills with "dank je wel" in the first
	// place; see socialCloser. It is per-namespace overridable: a chat-like
	// namespace may want to keep short exchanges. The lookup needs the
	// namespace to exist, so it runs after the ID is resolved below.
	socialFilter := b.config.SocialFilter

	if err := validatePath(namespaceSlug); err != nil {
		return 0, decision, err
	}

	nsID, err := b.resolveNamespaceID(ctx, namespaceSlug)
	if err != nil {
		return 0, decision, err
	}

	if eff, err := b.EffectiveNamespaceConfig(ctx, namespaceSlug); err == nil && eff != nil {
		socialFilter = eff.SocialFilter()
	}
	if socialFilter && socialCloser(content) {
		return 0, decision, ErrSocialCloser
	}

	occurred := time.Now().UTC()
	if opts.OccurredAt != nil {
		occurred = *opts.OccurredAt
	}

	vec, err := b.embedder.Embed(ctx, content)
	if err != nil {
		return 0, decision, fmt.Errorf("embed: %w", err)
	}

	var id int64
	err = b.pool.QueryRow(ctx,
		`INSERT INTO episodes (namespace_id, content, embedding, embedding_model, occurred_at, importance, tags)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		nsID, content, pgvector.NewVector(vec), b.embedder.Model(), occurred, importance, tagsOrEmpty(tags),
	).Scan(&id)
	if err != nil {
		return 0, decision, fmt.Errorf("insert episode: %w", err)
	}
	// Let the consolidator pick this up right away, unless this is an eval
	// write: /eval/ episodes exist to measure recall and must not turn into
	// facts while the evaluation is still running.
	if !strings.HasPrefix(namespaceSlug, "/eval/") {
		b.noteEpisode(nsID)
	}
	return id, decision, nil
}

// findEpisode returns the episode closest to query across the given
// namespaces, without changing it. Soft-deleted episodes are invisible, so a
// claim that was already forgotten or retracted cannot be matched by accident.
func (b *Brain) findEpisode(ctx context.Context, namespaceSlugs []string, query string) (id, nsID int64, content string, err error) {
	if err := validateContent(query); err != nil {
		return 0, 0, "", err
	}
	for _, slug := range namespaceSlugs {
		if err := validatePath(slug); err != nil {
			return 0, 0, "", err
		}
	}

	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return 0, 0, "", err
	}

	vec, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return 0, 0, "", fmt.Errorf("embed: %w", err)
	}

	if len(nsIDs) == 1 {
		err = b.pool.QueryRow(ctx,
			`SELECT id, namespace_id, content FROM episodes
			 WHERE namespace_id = $1 AND deleted_at IS NULL AND embedding IS NOT NULL
			 ORDER BY embedding <=> $2 LIMIT 1`,
			nsIDs[0], pgvector.NewVector(vec),
		).Scan(&id, &nsID, &content)
	} else {
		err = b.pool.QueryRow(ctx,
			`SELECT id, namespace_id, content FROM episodes
			 WHERE namespace_id = ANY($1) AND deleted_at IS NULL AND embedding IS NOT NULL
			 ORDER BY embedding <=> $2 LIMIT 1`,
			nsIDs, pgvector.NewVector(vec),
		).Scan(&id, &nsID, &content)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, "", ErrEpisodeNotFound
	}
	if err != nil {
		return 0, 0, "", fmt.Errorf("find episode: %w", err)
	}
	return id, nsID, content, nil
}

// ForgetEpisode soft-deletes the episode that best matches the query across the given namespaces.
// If namespaceSlugs is empty, searches all namespaces.
//
// Forget says "stop using this" and makes no claim about whether it was true.
// Use RetractEpisode when the memory was wrong and the correction matters.
func (b *Brain) ForgetEpisode(ctx context.Context, namespaceSlugs []string, query string) error {
	id, _, _, err := b.findEpisode(ctx, namespaceSlugs, query)
	if err != nil {
		return err
	}
	if _, err := b.pool.Exec(ctx,
		"UPDATE episodes SET deleted_at = now() WHERE id = $1", id,
	); err != nil {
		return fmt.Errorf("soft delete episode: %w", err)
	}
	return nil
}

// PurgeEpisodeByQuery hard-deletes the episode that best matches query. It
// exists for the MCP `forget` tool, where the caller names a memory by what it
// says, not by an id it has never seen.
func (b *Brain) PurgeEpisodeByQuery(ctx context.Context, namespaceSlugs []string, query string, opts PurgeOptions) (PurgeCounts, error) {
	id, _, _, err := b.findEpisode(ctx, namespaceSlugs, query)
	if err != nil {
		return PurgeCounts{}, err
	}
	return b.PurgeEpisode(ctx, id, opts)
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
