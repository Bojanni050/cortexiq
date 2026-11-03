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
	// ExternalKey makes the write keyed: the caller's own id for this memory
	// ("gaia:hypothesis:42"), unique per namespace among live episodes. Writing
	// the same content again is a no-op. Writing different content is refused
	// (ErrExternalKeyExists) unless Supersedes is set, in which case the old
	// episode is retired and the new one replaces it in one transaction.
	ExternalKey string
	// ExternalRefs are opaque pointers to where the memory came from outside
	// CortexIQ ("commit:bf575f8"). Facts consolidated from this episode inherit
	// them. They are stored and returned verbatim, never resolved.
	ExternalRefs []string
	// Supersedes allows a keyed write to replace an episode holding different
	// content. It requires ExternalKey.
	Supersedes bool
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
	out, err := b.RememberWithOutcome(ctx, namespaceSlug, content, opts)
	return out.ID, out.Decision, err
}

// RememberOutcome is what a write did, for callers that must tell a new memory
// from a replayed one or from a superseding version.
type RememberOutcome struct {
	// ID is the live episode that now holds the content. For an Unchanged write
	// it is the existing episode; no row was written.
	ID       int64
	Decision WriteDecision
	// Unchanged is true when ExternalKey already held exactly this content, so
	// the write was a replay and changed nothing.
	Unchanged bool
	// SupersededID is the episode this write retired under the same ExternalKey,
	// or 0 when it replaced nothing.
	SupersededID int64
	// FactsEnded counts derived facts closed because the superseded episode was
	// their only live source.
	FactsEnded int64
}

// RememberWithOutcome is RememberWithDecision with the full outcome. With an
// ExternalKey the write is keyed: see RememberOptions.
func (b *Brain) RememberWithOutcome(ctx context.Context, namespaceSlug, content string, opts RememberOptions) (RememberOutcome, error) {
	var decision WriteDecision
	if err := requireWrite(ctx); err != nil {
		return RememberOutcome{Decision: decision}, err
	}
	if err := validateContent(content); err != nil {
		return RememberOutcome{Decision: decision}, err
	}
	importance := opts.Importance
	if importance < 0 || importance > 1 || importance != importance {
		return RememberOutcome{Decision: decision}, fmt.Errorf("importance must be between 0 and 1, got %v", importance)
	}
	tags, err := NormalizeTags(opts.Tags)
	if err != nil {
		return RememberOutcome{Decision: decision}, err
	}
	externalKey, err := NormalizeExternalKey(opts.ExternalKey)
	if err != nil {
		return RememberOutcome{Decision: decision}, err
	}
	externalRefs, err := NormalizeExternalRefs(opts.ExternalRefs)
	if err != nil {
		return RememberOutcome{Decision: decision}, err
	}
	if opts.Supersedes && externalKey == "" {
		return RememberOutcome{Decision: decision}, ErrSupersedesNeedsKey
	}

	// The write filter is the second gate in front of every write path (after
	// requireWrite), so the store never fills with leaked secrets or machine
	// noise. It runs before the social filter and before the embedding call:
	// a rejected write costs nothing.
	decision = classifyWrite(content, importance, b.config.WriteFilter)
	if err := applyWriteDecision(decision); err != nil {
		return RememberOutcome{Decision: decision}, err
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
		return RememberOutcome{Decision: decision}, err
	}

	nsID, err := b.resolveNamespaceID(ctx, namespaceSlug)
	if err != nil {
		return RememberOutcome{Decision: decision}, err
	}

	if eff, err := b.EffectiveNamespaceConfig(ctx, namespaceSlug); err == nil && eff != nil {
		socialFilter = eff.SocialFilter()
	}
	if socialFilter && socialCloser(content) {
		return RememberOutcome{Decision: decision}, ErrSocialCloser
	}

	occurred := time.Now().UTC()
	if opts.OccurredAt != nil {
		occurred = *opts.OccurredAt
	}

	// A keyed replay is decided before the embedding call: writing the same
	// content under the same key again must cost nothing, which is what makes a
	// rebuild from scratch cheap. The check is repeated under a row lock below.
	if externalKey != "" {
		var existingID int64
		var existingContent string
		err := b.pool.QueryRow(ctx,
			`SELECT id, content FROM episodes
			  WHERE namespace_id = $1 AND external_key = $2 AND deleted_at IS NULL`,
			nsID, externalKey,
		).Scan(&existingID, &existingContent)
		switch {
		case err == nil && existingContent == content:
			return RememberOutcome{ID: existingID, Decision: decision, Unchanged: true}, nil
		case err == nil && !opts.Supersedes:
			return RememberOutcome{Decision: decision}, ErrExternalKeyExists
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return RememberOutcome{Decision: decision}, fmt.Errorf("look up external key: %w", err)
		}
	}

	vec, err := b.embedder.Embed(ctx, content)
	if err != nil {
		return RememberOutcome{Decision: decision}, fmt.Errorf("embed: %w", err)
	}

	out, err := b.insertEpisode(ctx, nsID, content, vec, occurred, importance, tags, externalKey, externalRefs, opts.Supersedes)
	out.Decision = decision
	if err != nil {
		return out, err
	}
	// Let the consolidator pick this up right away, unless this is an eval
	// write: /eval/ episodes exist to measure recall and must not turn into
	// facts while the evaluation is still running.
	if !out.Unchanged && !strings.HasPrefix(namespaceSlug, "/eval/") {
		b.noteEpisode(nsID)
	}
	return out, nil
}

// insertEpisode writes the episode in one transaction. For a keyed write that
// replaces an existing episode, retiring the old one and inserting the new one
// happen together: if anything fails the old episode is still live and the key
// still resolves to it, never to nothing.
func (b *Brain) insertEpisode(ctx context.Context, nsID int64, content string, vec []float32, occurred time.Time, importance float32, tags []string, externalKey string, externalRefs []string, supersedes bool) (RememberOutcome, error) {
	var out RememberOutcome

	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return out, fmt.Errorf("remember: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var oldID int64
	if externalKey != "" {
		var oldContent string
		err := tx.QueryRow(ctx,
			`SELECT id, content FROM episodes
			  WHERE namespace_id = $1 AND external_key = $2 AND deleted_at IS NULL
			  FOR UPDATE`,
			nsID, externalKey,
		).Scan(&oldID, &oldContent)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			oldID = 0
		case err != nil:
			return out, fmt.Errorf("lock external key: %w", err)
		case oldContent == content:
			// Another writer got here first with the same content.
			out.ID, out.Unchanged = oldID, true
			return out, nil
		case !supersedes:
			return out, ErrExternalKeyExists
		}
		// Retire the old episode before the insert: the unique index only
		// covers live rows, so the key is free again from here on.
		if oldID != 0 {
			if _, err := tx.Exec(ctx,
				`UPDATE episodes SET deleted_at = now() WHERE id = $1`, oldID,
			); err != nil {
				return out, fmt.Errorf("retire superseded episode: %w", err)
			}
		}
	}

	var key *string
	if externalKey != "" {
		key = &externalKey
	}
	var id int64
	err = tx.QueryRow(ctx,
		`INSERT INTO episodes (namespace_id, content, embedding, embedding_model, occurred_at, importance, tags, external_key, external_refs)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		nsID, content, pgvector.NewVector(vec), b.embedder.Model(), occurred, importance, tagsOrEmpty(tags), key, refsOrEmpty(externalRefs),
	).Scan(&id)
	if isUniqueViolation(err) {
		return RememberOutcome{}, ErrExternalKeyExists
	}
	if err != nil {
		return out, fmt.Errorf("insert episode: %w", err)
	}
	out.ID = id

	if oldID != 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE episodes SET superseded_by = $2 WHERE id = $1`, oldID, id,
		); err != nil {
			return out, fmt.Errorf("link superseded episode: %w", err)
		}
		// A fact whose only live source was the retired episode described the
		// old version. Close it the way decay and contradiction resolution do:
		// valid_until and known_until stop at the same instant.
		tag, err := tx.Exec(ctx,
			`UPDATE facts f
			    SET valid_until = COALESCE(f.valid_until, now()),
			        known_until = now(),
			        updated_at  = now()
			  WHERE f.deleted_at IS NULL
			    AND f.known_until IS NULL
			    AND EXISTS (SELECT 1 FROM fact_sources s
			                 WHERE s.fact_id = f.id AND s.episode_id = $1)
			    AND NOT EXISTS (SELECT 1 FROM fact_sources s
			                      JOIN episodes e ON e.id = s.episode_id
			                     WHERE s.fact_id = f.id AND e.id <> $1 AND e.deleted_at IS NULL)`,
			oldID,
		)
		if err != nil {
			return out, fmt.Errorf("end facts of superseded episode: %w", err)
		}
		out.SupersededID, out.FactsEnded = oldID, tag.RowsAffected()
	}

	if err := tx.Commit(ctx); err != nil {
		if isUniqueViolation(err) {
			return RememberOutcome{}, ErrExternalKeyExists
		}
		return RememberOutcome{}, fmt.Errorf("remember: commit: %w", err)
	}
	return out, nil
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
