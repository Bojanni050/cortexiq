// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

var (
	ErrMentalModelNotFound = fmt.Errorf("brain: mental model not found")
	// ErrMentalModelNotGrounded means the synthesis found no memories behind
	// the topic. A refresh in that state keeps the existing content instead of
	// replacing a good summary with "I don't know".
	ErrMentalModelNotGrounded = errors.New("brain: no memories found to ground the mental model")
	// ErrMentalModelRefreshInFlight means this process is already synthesizing
	// the model. Callers should retry later rather than queue a second LLM call.
	ErrMentalModelRefreshInFlight = errors.New("brain: mental model refresh already in progress")
)

const mentalModelColumns = `id, namespace_id, name, source_query, content, embedding,
	embedding_model, tags, max_tokens, last_refreshed_at, last_memory_seen_at,
	last_refresh_failed_at, last_refresh_error, based_on, created_at, updated_at, deleted_at`

// maxMentalModelRefreshesPerConsolidation bounds the LLM work one consolidation
// run may add. Mental models are refreshed oldest-first, so a backlog drains
// over several runs instead of turning one consolidation into an unbounded bill.
const maxMentalModelRefreshesPerConsolidation = 3

// minAutomaticMentalModelRefresh is how often the consolidation sweep may touch
// the same model. A global model is stale as soon as anything anywhere changes,
// so without this floor it would be re-synthesized on every five-minute run.
// An explicit refresh ignores the floor.
const minAutomaticMentalModelRefresh = time.Hour

// MentalModelSearchResult is a mental model hit from a similarity search.
type MentalModelSearchResult struct {
	ID               int64      `json:"id"`
	NamespaceID      *int64     `json:"namespace_id,omitempty"`
	Name             string     `json:"name"`
	Content          string     `json:"content"`
	Tags             []string   `json:"tags"`
	Relevance        float32    `json:"relevance"`
	LastRefreshedAt  *time.Time `json:"last_refreshed_at,omitempty"`
	LastMemorySeenAt *time.Time `json:"last_memory_seen_at,omitempty"`
	// BasedOn is carried for the staleness check and not exposed: the search
	// result is a snippet, and the grounding is available on mental_model_get.
	BasedOn     models.BasedOn `json:"-"`
	IsStale     bool           `json:"is_stale"`
	StaleReason string         `json:"stale_reason,omitempty"`
}

// CreateMentalModelOptions configures a new mental model.
type CreateMentalModelOptions struct {
	Name        string
	SourceQuery string
	Tags        []string
	MaxTokens   int
	// NamespaceID files the model in a namespace and scopes it to that
	// namespace's subtree. Nil makes it global: it reads every namespace and is
	// available to reflect from every namespace.
	NamespaceID *int64
}

// MentalModelRefreshSummary reports what an automatic sweep did. Individual
// failures never abort the sweep: one broken model must not keep the rest stale.
type MentalModelRefreshSummary struct {
	Refreshed int      `json:"refreshed"`
	Failed    int      `json:"failed"`
	Skipped   int      `json:"skipped"`
	Errors    []string `json:"errors,omitempty"`
}

// rowScanner is implemented by pgx.Row and pgx.Rows, so one scan function serves
// Get, List and the RETURNING clauses.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanMentalModel(m *models.MentalModel, row rowScanner) error {
	var (
		basedOn        []byte
		lastRefreshErr *string
	)
	if err := row.Scan(
		&m.ID, &m.NamespaceID, &m.Name, &m.SourceQuery, &m.Content, &m.Embedding,
		&m.EmbeddingModel, &m.Tags, &m.MaxTokens, &m.LastRefreshedAt, &m.LastMemorySeenAt,
		&m.LastRefreshFailedAt, &lastRefreshErr, &basedOn,
		&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt,
	); err != nil {
		return err
	}
	if lastRefreshErr != nil {
		m.LastRefreshError = *lastRefreshErr
	}
	if len(basedOn) > 0 {
		if err := json.Unmarshal(basedOn, &m.BasedOn); err != nil {
			return fmt.Errorf("decode mental model based_on: %w", err)
		}
	}
	return nil
}

// basedOnFromSources turns the memories a reflect call read into the grounding
// record stored on the model. Ids are sorted and de-duplicated so the JSON is
// stable and comparable between refreshes.
func basedOnFromSources(sources []ReflectSource) models.BasedOn {
	var out models.BasedOn
	for _, s := range sources {
		switch s.Type {
		case "fact":
			out.Facts = append(out.Facts, s.ID)
		case "episode":
			out.Episodes = append(out.Episodes, s.ID)
		case "failure":
			out.Failures = append(out.Failures, s.ID)
		case "goal":
			out.Goals = append(out.Goals, s.ID)
		}
	}
	out.Facts = sortedUniqueIDs(out.Facts)
	out.Episodes = sortedUniqueIDs(out.Episodes)
	out.Failures = sortedUniqueIDs(out.Failures)
	out.Goals = sortedUniqueIDs(out.Goals)
	return out
}

func sortedUniqueIDs(ids []int64) []int64 {
	if len(ids) < 2 {
		return ids
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := ids[:1]
	for _, id := range ids[1:] {
		if id != out[len(out)-1] {
			out = append(out, id)
		}
	}
	return out
}

func marshalBasedOn(b models.BasedOn) ([]byte, error) {
	data, err := json.Marshal(b)
	if err != nil {
		return nil, fmt.Errorf("encode mental model based_on: %w", err)
	}
	return data, nil
}

// groundedReflect rejects a synthesis that has nothing behind it. A model built
// on zero memories is worse than no model: reflect would trust it as curated
// knowledge, and the prompt would carry a confident "no information" answer.
func groundedReflect(result *ReflectResult) error {
	if result == nil || len(result.Sources) == 0 {
		return ErrMentalModelNotGrounded
	}
	if strings.TrimSpace(result.Answer) == "" {
		return ErrMentalModelNotGrounded
	}
	return nil
}

// memoryCutoff returns the database clock, the reference for every watermark.
// It has to be database time because updated_at/created_at defaults are database
// time too; an application clock a few seconds ahead would silently skip
// memories written in the gap.
func (b *Brain) memoryCutoff(ctx context.Context) (time.Time, error) {
	var cutoff time.Time
	if err := b.pool.QueryRow(ctx, "SELECT now()").Scan(&cutoff); err != nil {
		return time.Time{}, fmt.Errorf("read database clock: %w", err)
	}
	return cutoff, nil
}

// CreateMentalModel creates a mental model and generates its content by running
// a reflect call on the source query. The mental model is embedded on its
// source query for semantic search.
//
// With opts.NamespaceID nil the model is global: the synthesis reads every
// namespace, staleness follows every namespace, and reflect finds it from any
// namespace. With a namespace it is scoped to that namespace and its children.
func (b *Brain) CreateMentalModel(ctx context.Context, opts CreateMentalModelOptions) (*models.MentalModel, error) {
	name, query, tags, maxTokens, err := createMentalModelInput(opts)
	if err != nil {
		return nil, err
	}

	// "/" is the whole store; it needs no namespace row and works on an install
	// that never created one (prod has no "/" namespace).
	scope := []string{"/"}
	if opts.NamespaceID != nil {
		slug, err := b.namespaceSlug(ctx, *opts.NamespaceID)
		if err != nil {
			return nil, err
		}
		scope = []string{slug}
	}

	// Take the watermark before the synthesis reads anything: a memory written
	// while the LLM is running stays newer than the stamp and is picked up by
	// the next refresh instead of being lost.
	cutoff, err := b.memoryCutoff(ctx)
	if err != nil {
		return nil, err
	}

	reflectResult, err := b.Reflect(ctx, query, ReflectOptions{
		Namespaces: scope,
		Limit:      20,
		MaxTokens:  maxTokens,
		// A mental model is synthesized from raw memories. Its siblings would
		// make it a copy of what other models already believe.
		SkipMentalModels: true,
	})
	if err != nil {
		return nil, fmt.Errorf("generate mental model content: %w", err)
	}
	if err := groundedReflect(reflectResult); err != nil {
		return nil, fmt.Errorf("create mental model %q: %w", name, err)
	}

	basedOn, err := marshalBasedOn(basedOnFromSources(reflectResult.Sources))
	if err != nil {
		return nil, err
	}

	vec, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed mental model query: %w", err)
	}

	var m models.MentalModel
	err = scanMentalModel(&m, b.pool.QueryRow(ctx, `
		INSERT INTO mental_models (namespace_id, name, source_query, content, embedding, embedding_model, tags, max_tokens, last_refreshed_at, last_memory_seen_at, based_on)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), $9, $10)
		RETURNING `+mentalModelColumns,
		opts.NamespaceID, name, query, reflectResult.Answer,
		pgvector.NewVector(vec), b.embedder.Model(), tags, maxTokens, cutoff, basedOn))
	if err != nil {
		return nil, fmt.Errorf("insert mental model: %w", err)
	}
	return &m, nil
}

// createMentalModelInput validates and normalizes the caller's options. Shared
// by the blocking and the queued create so both reject the same things.
func createMentalModelInput(opts CreateMentalModelOptions) (name, query string, tags []string, maxTokens int, err error) {
	name = strings.TrimSpace(opts.Name)
	if name == "" {
		return "", "", nil, 0, fmt.Errorf("brain: mental model name cannot be empty")
	}
	query = strings.TrimSpace(opts.SourceQuery)
	if query == "" {
		return "", "", nil, 0, fmt.Errorf("brain: mental model source query cannot be empty")
	}
	maxTokens = opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	// tags is NOT NULL: a model created without tags stores an empty array, not
	// NULL. Callers building their tag list with append hand us nil.
	tags = opts.Tags
	if tags == nil {
		tags = []string{}
	}
	return name, query, tags, maxTokens, nil
}

// CreateMentalModelQueued inserts an empty model and returns immediately. Its
// content is generated by RefreshMentalModel, which the caller runs in the
// background.
//
// The row deliberately has no watermark and no last_refreshed_at: until the
// refresh finishes it reads as "never refreshed", so a process that dies before
// generating it leaves it for the consolidation sweep instead of a model that
// looks fresh but holds nothing.
func (b *Brain) CreateMentalModelQueued(ctx context.Context, opts CreateMentalModelOptions) (*models.MentalModel, error) {
	name, query, tags, maxTokens, err := createMentalModelInput(opts)
	if err != nil {
		return nil, err
	}
	// Fail a bad namespace at the call, not in the background where nobody sees it.
	if opts.NamespaceID != nil {
		if _, err := b.namespaceSlug(ctx, *opts.NamespaceID); err != nil {
			return nil, err
		}
	}

	vec, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed mental model query: %w", err)
	}

	var m models.MentalModel
	err = scanMentalModel(&m, b.pool.QueryRow(ctx, `
		INSERT INTO mental_models (namespace_id, name, source_query, embedding, embedding_model, tags, max_tokens)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+mentalModelColumns,
		opts.NamespaceID, name, query, pgvector.NewVector(vec), b.embedder.Model(), tags, maxTokens))
	if err != nil {
		return nil, fmt.Errorf("insert mental model: %w", err)
	}
	return &m, nil
}

// getMentalModel reads a model without computing its freshness.
func (b *Brain) getMentalModel(ctx context.Context, id int64) (*models.MentalModel, error) {
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

// GetMentalModel returns a mental model by ID, with its freshness computed.
func (b *Brain) GetMentalModel(ctx context.Context, id int64) (*models.MentalModel, error) {
	m, err := b.getMentalModel(ctx, id)
	if err != nil {
		return nil, err
	}
	items := []models.MentalModel{*m}
	if err := b.annotateMentalModels(ctx, items); err != nil {
		return nil, err
	}
	return &items[0], nil
}

// ListMentalModels returns mental models in the given namespaces, optionally
// filtered by tags (any match), with freshness computed for each.
func (b *Brain) ListMentalModels(ctx context.Context, namespaceSlugs []string, tags []string, page Pagination) ([]models.MentalModel, error) {
	nsIDs, err := b.ResolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}
	if len(nsIDs) == 0 {
		return nil, nil
	}
	page = page.Sanitize()

	where := "deleted_at IS NULL AND (namespace_id IS NULL OR namespace_id = ANY($1))"
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
		if err := scanMentalModel(&m, rows); err != nil {
			return nil, fmt.Errorf("scan mental model: %w", err)
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := b.annotateMentalModels(ctx, result); err != nil {
		return nil, err
	}
	return result, nil
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
	where := "deleted_at IS NULL AND (namespace_id IS NULL OR namespace_id = ANY($1))"
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

	where := "deleted_at IS NULL AND (namespace_id IS NULL OR namespace_id = ANY($1)) AND embedding IS NOT NULL"
	args := []any{nsIDs, pgvector.NewVector(vec)}
	argIdx := 3
	if len(tags) > 0 {
		where += fmt.Sprintf(" AND tags && $%d", argIdx)
		args = append(args, tags)
		argIdx++
	}
	args = append(args, limit)

	rows, err := b.pool.Query(ctx, `
		SELECT id, namespace_id, name, content, tags, last_refreshed_at, last_memory_seen_at, based_on,
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
		var (
			r       MentalModelSearchResult
			basedOn []byte
		)
		if err := rows.Scan(&r.ID, &r.NamespaceID, &r.Name, &r.Content, &r.Tags,
			&r.LastRefreshedAt, &r.LastMemorySeenAt, &basedOn, &r.Relevance); err != nil {
			return nil, fmt.Errorf("scan mental model search: %w", err)
		}
		if len(basedOn) > 0 {
			if err := json.Unmarshal(basedOn, &r.BasedOn); err != nil {
				return nil, fmt.Errorf("decode mental model based_on: %w", err)
			}
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := b.annotateMentalModelSearch(ctx, results); err != nil {
		return nil, err
	}
	return results, nil
}

// RefreshMentalModel regenerates the content of a mental model by re-running
// the reflect call on its source query.
//
// A refresh never overwrites good content with an ungrounded answer: when the
// synthesis finds no memories, the old content is kept and the failure is
// stamped on the row. Callers get an error either way, but the model survives.
func (b *Brain) RefreshMentalModel(ctx context.Context, id int64) (*models.MentalModel, error) {
	release, busy := b.beginMentalModelRefresh(id)
	if busy {
		return nil, ErrMentalModelRefreshInFlight
	}
	defer release()

	m, err := b.getMentalModel(ctx, id)
	if err != nil {
		return nil, err
	}

	// A global model (no namespace) re-reads everything; a scoped one re-reads
	// its own subtree.
	scope := []string{"/"}
	if m.NamespaceID != nil {
		slug, err := b.namespaceSlug(ctx, *m.NamespaceID)
		if err != nil {
			return nil, err
		}
		scope = []string{slug}
	}

	cutoff, err := b.memoryCutoff(ctx)
	if err != nil {
		return nil, err
	}

	reflectResult, err := b.Reflect(ctx, m.SourceQuery, ReflectOptions{
		Namespaces:       scope,
		Limit:            20,
		MaxTokens:        m.MaxTokens,
		SkipMentalModels: true,
	})
	if err != nil {
		return nil, fmt.Errorf("refresh mental model content: %w", err)
	}
	if err := groundedReflect(reflectResult); err != nil {
		b.recordMentalModelRefreshFailure(ctx, id, err)
		return nil, fmt.Errorf("refresh mental model %q: %w (existing content kept)", m.Name, err)
	}

	basedOn, err := marshalBasedOn(basedOnFromSources(reflectResult.Sources))
	if err != nil {
		return nil, err
	}

	var updated models.MentalModel
	err = scanMentalModel(&updated, b.pool.QueryRow(ctx, `
		UPDATE mental_models
		SET content = $1, based_on = $2, last_refreshed_at = now(), last_memory_seen_at = $3,
		    last_refresh_failed_at = NULL, last_refresh_error = NULL, updated_at = now()
		WHERE id = $4 AND deleted_at IS NULL
		RETURNING `+mentalModelColumns,
		reflectResult.Answer, basedOn, cutoff, id))
	if err == pgx.ErrNoRows {
		return nil, ErrMentalModelNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update mental model: %w", err)
	}
	return &updated, nil
}

// recordMentalModelRefreshFailure stamps a refused refresh so the dashboard and
// the automatic sweep can see it. Bookkeeping is best effort: the caller already
// has the real error and must not lose it to a second failure.
func (b *Brain) recordMentalModelRefreshFailure(ctx context.Context, id int64, cause error) {
	_, _ = b.pool.Exec(ctx, `
		UPDATE mental_models
		SET last_refresh_failed_at = now(), last_refresh_error = $2, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL`,
		id, truncateError(cause.Error(), 500))
}

func truncateError(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// beginMentalModelRefresh claims the per-model refresh slot. The second return
// value is true when another goroutine already owns it.
func (b *Brain) beginMentalModelRefresh(id int64) (release func(), busy bool) {
	b.mmMu.Lock()
	defer b.mmMu.Unlock()
	if _, ok := b.mmRefreshing[id]; ok {
		return func() {}, true
	}
	b.mmRefreshing[id] = struct{}{}
	return func() {
		b.mmMu.Lock()
		delete(b.mmRefreshing, id)
		b.mmMu.Unlock()
	}, false
}

// refreshStaleMentalModels refreshes the models a consolidation run can reach:
// the ones filed in this namespace plus the global ones. It runs at the end of a
// consolidation, so a new fact reaches the summaries without an agent having to
// remember to refresh them.
//
// Automatic refresh skips a model whose last refresh failed: retrying the same
// prompt on every consolidation only spends the same money to fail the same way.
// An explicit RefreshMentalModel call is never paused and clears the mark on
// success. It also rate-limits itself to one synthesis per model per hour: a
// global model sees every namespace, and without the floor every consolidation
// run would buy it again.
func (b *Brain) refreshStaleMentalModels(ctx context.Context, nsID int64) (MentalModelRefreshSummary, error) {
	items, err := b.listMentalModelsForSweep(ctx, nsID)
	if err != nil {
		return MentalModelRefreshSummary{}, err
	}

	// Oldest first, never-refreshed first: a backlog drains in the order that
	// leaves the least stale knowledge behind.
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i].LastRefreshedAt, items[j].LastRefreshedAt
		if a == nil {
			return b != nil
		}
		if b == nil {
			return false
		}
		return a.Before(*b)
	})

	now := time.Now()
	var out MentalModelRefreshSummary
	for i := range items {
		m := &items[i]
		if !m.IsStale || mentalModelAutoRefreshPaused(*m) || !automaticRefreshDue(*m, now) {
			out.Skipped++
			continue
		}
		if out.Refreshed >= maxMentalModelRefreshesPerConsolidation {
			out.Skipped++
			continue
		}
		if _, err := b.RefreshMentalModel(ctx, m.ID); err != nil {
			if errors.Is(err, ErrMentalModelRefreshInFlight) {
				out.Skipped++
				continue
			}
			out.Failed++
			out.Errors = append(out.Errors, fmt.Sprintf("refresh mental model %d (%s): %v", m.ID, m.Name, err))
			continue
		}
		out.Refreshed++
	}
	return out, nil
}

// mentalModelAutoRefreshPaused reports whether the last refresh failed after the
// last successful one. A failure older than the stored content is history, not
// a reason to stay paused.
func mentalModelAutoRefreshPaused(m models.MentalModel) bool {
	if m.LastRefreshFailedAt == nil {
		return false
	}
	return m.LastRefreshedAt == nil || m.LastRefreshFailedAt.After(*m.LastRefreshedAt)
}

// automaticRefreshDue reports whether the sweep may refresh this model again.
// Explicit refreshes do not consult it.
func automaticRefreshDue(m models.MentalModel, now time.Time) bool {
	return m.LastRefreshedAt == nil || now.Sub(*m.LastRefreshedAt) >= minAutomaticMentalModelRefresh
}

// listMentalModelsForSweep returns the models a namespace's consolidation is
// responsible for: the ones filed there, plus every global model. Global models
// are considered by every namespace, but automaticRefreshDue keeps that from
// buying the same synthesis several times per run.
func (b *Brain) listMentalModelsForSweep(ctx context.Context, nsID int64) ([]models.MentalModel, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT `+mentalModelColumns+` FROM mental_models
		WHERE deleted_at IS NULL AND (namespace_id = $1 OR namespace_id IS NULL)
		ORDER BY last_refreshed_at NULLS FIRST, id
		LIMIT $2`, nsID, MaxLimit)
	if err != nil {
		return nil, fmt.Errorf("list mental models for sweep: %w", err)
	}
	defer rows.Close()

	var result []models.MentalModel
	for rows.Next() {
		var m models.MentalModel
		if err := scanMentalModel(&m, rows); err != nil {
			return nil, fmt.Errorf("scan mental model: %w", err)
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := b.annotateMentalModels(ctx, result); err != nil {
		return nil, err
	}
	return result, nil
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
	if tags == nil {
		tags = []string{}
	}
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

// mentalModelScopeIDs returns the namespace a model lives in plus all of its
// descendants: the same scope reflect reads when it synthesizes the model, so a
// fact in a child namespace makes the parent's summary stale.
func (b *Brain) mentalModelScopeIDs(ctx context.Context, nsID int64) ([]int64, error) {
	slug, err := b.namespaceSlug(ctx, nsID)
	if err != nil {
		if errors.Is(err, ErrNamespaceNotFound) {
			return []int64{nsID}, nil
		}
		return nil, err
	}
	return b.resolveNamespaceIDs(ctx, []string{slug})
}

// latestMemoryTimes returns the newest memory timestamp per namespace, counting
// a fact's last edit and an episode's creation. Episodes have no updated_at, and
// treating them as immutable is correct: their text is never rewritten.
func (b *Brain) latestMemoryTimes(ctx context.Context, nsIDs []int64) (map[int64]time.Time, error) {
	if len(nsIDs) == 0 {
		return nil, nil
	}
	rows, err := b.pool.Query(ctx, `
		SELECT namespace_id, MAX(ts) FROM (
			SELECT namespace_id, MAX(updated_at) AS ts FROM facts
			 WHERE namespace_id = ANY($1) AND deleted_at IS NULL GROUP BY namespace_id
			UNION ALL
			SELECT namespace_id, MAX(created_at) AS ts FROM episodes
			 WHERE namespace_id = ANY($1) AND deleted_at IS NULL GROUP BY namespace_id
		) u GROUP BY namespace_id`, nsIDs)
	if err != nil {
		return nil, fmt.Errorf("latest memory times: %w", err)
	}
	defer rows.Close()

	out := make(map[int64]time.Time)
	for rows.Next() {
		var (
			nsID int64
			ts   time.Time
		)
		if err := rows.Scan(&nsID, &ts); err != nil {
			return nil, fmt.Errorf("scan latest memory time: %w", err)
		}
		out[nsID] = ts
	}
	return out, rows.Err()
}

// liveIDs returns which of the given ids still exist in table. table is a
// constant chosen by this package, never caller input.
func (b *Brain) liveIDs(ctx context.Context, table string, ids []int64) (map[int64]bool, error) {
	out := make(map[int64]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := b.pool.Query(ctx,
		`SELECT id FROM `+table+` WHERE id = ANY($1) AND deleted_at IS NULL`, ids)
	if err != nil {
		return nil, fmt.Errorf("check %s ids: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan %s id: %w", table, err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

// retractedGrounding reports, per model id, whether the stored content cites a
// memory that no longer exists. That is the one staleness signal no watermark
// can raise: a deleted row writes nothing.
func (b *Brain) retractedGrounding(ctx context.Context, states []mentalModelState) (map[int64]bool, error) {
	var (
		factIDs, episodeIDs, failureIDs, goalIDs []int64
	)
	for _, s := range states {
		factIDs = append(factIDs, s.BasedOn.Facts...)
		episodeIDs = append(episodeIDs, s.BasedOn.Episodes...)
		failureIDs = append(failureIDs, s.BasedOn.Failures...)
		goalIDs = append(goalIDs, s.BasedOn.Goals...)
	}
	live := make(map[string]map[int64]bool, 4)
	for _, tbl := range []struct {
		name string
		ids  []int64
	}{
		{"facts", factIDs},
		{"episodes", episodeIDs},
		{"failures", failureIDs},
		{"goals", goalIDs},
	} {
		ids, err := b.liveIDs(ctx, tbl.name, sortedUniqueIDs(tbl.ids))
		if err != nil {
			return nil, err
		}
		live[tbl.name] = ids
	}

	missing := func(table string, ids []int64) bool {
		for _, id := range ids {
			if !live[table][id] {
				return true
			}
		}
		return false
	}

	out := make(map[int64]bool, len(states))
	for _, s := range states {
		out[s.ID] = missing("facts", s.BasedOn.Facts) ||
			missing("episodes", s.BasedOn.Episodes) ||
			missing("failures", s.BasedOn.Failures) ||
			missing("goals", s.BasedOn.Goals)
	}
	return out, nil
}

// mentalModelState is the input to the freshness calculation, shared by the
// stored model and the search result so both answer staleness the same way.
type mentalModelState struct {
	ID int64
	// NamespaceID is nil for a global model, which reads every namespace.
	NamespaceID      *int64
	LastRefreshedAt  *time.Time
	LastMemorySeenAt *time.Time
	BasedOn          models.BasedOn
}

type mentalModelFreshness struct {
	IsStale        bool
	Reason         string
	LatestMemoryAt *time.Time
}

// mentalModelStaleReason explains why a model is out of date, or returns "".
// Split out from the queries so the rule is testable without a database.
func mentalModelStaleReason(lastRefreshed, watermark *time.Time, newest time.Time, newestSeen, retracted, grounded bool) string {
	switch {
	case lastRefreshed == nil:
		return "never_refreshed"
	case watermark == nil:
		return "no_watermark"
	case !grounded:
		// Models created before based_on existed have content nobody can
		// account for, and the old create path stored "no information is
		// available" when recall found nothing. Treat them as stale so the next
		// consolidation either re-grounds them or records that it could not.
		return "no_grounding"
	case newestSeen && newest.After(*watermark):
		return "new_memories"
	case retracted:
		return "retracted_grounding"
	default:
		return ""
	}
}

// basedOnEmpty reports whether a model has no recorded grounding. Only models
// from before the watermark column can be in that state.
func basedOnEmpty(b models.BasedOn) bool {
	return len(b.Facts) == 0 && len(b.Episodes) == 0 && len(b.Failures) == 0 && len(b.Goals) == 0
}

// computeFreshness answers "is this model behind its memories?" for a batch.
// Scopes are resolved once per distinct namespace (and once for all namespaces,
// for global models) and the two expensive checks (newest memory, retracted
// grounding) are one round-trip each for the whole batch, so a reflect call does
// not pay a query per model.
func (b *Brain) computeFreshness(ctx context.Context, states []mentalModelState) ([]mentalModelFreshness, error) {
	out := make([]mentalModelFreshness, len(states))
	if len(states) == 0 {
		return out, nil
	}

	scopes := make([][]int64, len(states))
	scoped := make(map[int64][]int64, len(states))
	var (
		allIDs   []int64
		scopeIDs []int64
	)
	for i, s := range states {
		if s.NamespaceID == nil {
			if allIDs == nil {
				ids, err := b.ResolveNamespaceIDs(ctx, []string{"/"})
				if err != nil {
					return nil, err
				}
				allIDs = ids
			}
			scopes[i] = allIDs
			scopeIDs = append(scopeIDs, allIDs...)
			continue
		}
		ids, ok := scoped[*s.NamespaceID]
		if !ok {
			var err error
			ids, err = b.mentalModelScopeIDs(ctx, *s.NamespaceID)
			if err != nil {
				return nil, err
			}
			scoped[*s.NamespaceID] = ids
		}
		scopes[i] = ids
		scopeIDs = append(scopeIDs, ids...)
	}

	latest, err := b.latestMemoryTimes(ctx, sortedUniqueIDs(scopeIDs))
	if err != nil {
		return nil, err
	}
	retracted, err := b.retractedGrounding(ctx, states)
	if err != nil {
		return nil, err
	}

	for i, s := range states {
		var (
			newest     time.Time
			newestSeen bool
		)
		for _, nsID := range scopes[i] {
			if ts, ok := latest[nsID]; ok && (!newestSeen || ts.After(newest)) {
				newest, newestSeen = ts, true
			}
		}
		reason := mentalModelStaleReason(s.LastRefreshedAt, s.LastMemorySeenAt, newest, newestSeen, retracted[s.ID], !basedOnEmpty(s.BasedOn))
		out[i] = mentalModelFreshness{IsStale: reason != "", Reason: reason}
		if newestSeen {
			ts := newest
			out[i].LatestMemoryAt = &ts
		}
	}
	return out, nil
}

func (b *Brain) annotateMentalModels(ctx context.Context, items []models.MentalModel) error {
	states := make([]mentalModelState, len(items))
	for i := range items {
		states[i] = mentalModelState{
			ID:               items[i].ID,
			NamespaceID:      items[i].NamespaceID,
			LastRefreshedAt:  items[i].LastRefreshedAt,
			LastMemorySeenAt: items[i].LastMemorySeenAt,
			BasedOn:          items[i].BasedOn,
		}
	}
	freshness, err := b.computeFreshness(ctx, states)
	if err != nil {
		return err
	}
	for i := range items {
		items[i].IsStale = freshness[i].IsStale
		items[i].StaleReason = freshness[i].Reason
		items[i].LatestMemoryAt = freshness[i].LatestMemoryAt
	}
	return nil
}

func (b *Brain) annotateMentalModelSearch(ctx context.Context, results []MentalModelSearchResult) error {
	states := make([]mentalModelState, len(results))
	for i := range results {
		states[i] = mentalModelState{
			ID:               results[i].ID,
			NamespaceID:      results[i].NamespaceID,
			LastRefreshedAt:  results[i].LastRefreshedAt,
			LastMemorySeenAt: results[i].LastMemorySeenAt,
			BasedOn:          results[i].BasedOn,
		}
	}
	freshness, err := b.computeFreshness(ctx, states)
	if err != nil {
		return err
	}
	for i := range results {
		results[i].IsStale = freshness[i].IsStale
		results[i].StaleReason = freshness[i].Reason
	}
	return nil
}
