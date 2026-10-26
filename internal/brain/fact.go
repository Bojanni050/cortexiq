// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/jackc/pgx/v5"
)

// QueryFacts returns facts across namespaces matching the given slug paths, within an optional time range.
// Each path matches itself and all descendants.
func (b *Brain) QueryFacts(ctx context.Context, namespaceSlugs []string, since, until *time.Time, page Pagination) ([]models.Fact, error) {
	return b.QueryFactsAsOf(ctx, namespaceSlugs, since, until, nil, page)
}

// factWhere builds the historical filter used by existing as-of queries.
func factWhere(nsIDs []int64, since, until, asOf *time.Time) (string, []any) {
	return factWhereWindow(nsIDs, since, until, asOf, nil, nil, nil, nil)
}

// factWhereWindow builds the filter QueryFactsWindow and CountFactsWindow share, so the total
// can never describe a different set than the page.
func factWhereWindow(nsIDs []int64, since, until, asOf, from, to, knownFrom, knownTo *time.Time) (string, []any) {
	where := "namespace_id = ANY($1) AND deleted_at IS NULL"
	args := []any{nsIDs}
	if since != nil {
		args = append(args, *since)
		where += fmt.Sprintf(" AND created_at >= $%d", len(args))
	}
	if until != nil {
		args = append(args, *until)
		where += fmt.Sprintf(" AND created_at <= $%d", len(args))
	}
	if asOf != nil {
		n := len(args) + 1
		where += fmt.Sprintf(" AND created_at <= $%d AND (known_until IS NULL OR known_until > $%d)", n, n)
		args = append(args, *asOf)
	}
	if knownTo != nil {
		args = append(args, *knownTo)
		where += fmt.Sprintf(" AND created_at < $%d", len(args))
	}
	if knownFrom != nil {
		args = append(args, *knownFrom)
		where += fmt.Sprintf(" AND (known_until IS NULL OR known_until > $%d)", len(args))
	}
	fromParam, toParam := "'-infinity'::timestamptz", "'infinity'::timestamptz"
	if from != nil {
		args = append(args, *from)
		fromParam = fmt.Sprintf("$%d", len(args))
	}
	if to != nil {
		args = append(args, *to)
		toParam = fmt.Sprintf("$%d", len(args))
	}
	if from != nil || to != nil {
		where += fmt.Sprintf(" AND COALESCE(valid_from, created_at) < %s AND (valid_until IS NULL OR valid_until > %s)", toParam, fromParam)
	}
	return where, args
}

// checkFactWindows rejects empty or contradictory time arguments.
func checkFactWindows(asOf, from, to, knownFrom, knownTo *time.Time) error {
	if from != nil && to != nil && !from.Before(*to) {
		return fmt.Errorf("invalid time window: from must be before to")
	}
	if knownFrom != nil && knownTo != nil && !knownFrom.Before(*knownTo) {
		return fmt.Errorf("invalid knowledge window: known_from must be before known_to")
	}
	if asOf != nil && (knownFrom != nil || knownTo != nil) {
		return fmt.Errorf("as_of cannot be combined with known_from/known_to: as_of is the point form of the same axis")
	}
	return nil
}

// QueryFactsAsOf is QueryFacts restricted to the facts the system held at asOf
// (created_at <= asOf < known_until), which includes facts that were replaced or
// expired since. A nil asOf lists every fact, like QueryFacts.
func (b *Brain) QueryFactsAsOf(ctx context.Context, namespaceSlugs []string, since, until, asOf *time.Time, page Pagination) ([]models.Fact, error) {
	return b.QueryFactsWindow(ctx, namespaceSlugs, since, until, asOf, nil, nil, nil, nil, nil, page)
}

// QueryFactsWindow filters facts whose validity intervals overlap [from,to) (valid
// time) and whose knowledge intervals overlap [knownFrom,knownTo) (knowledge
// time), while retaining since/until as created_at filters and asOf as the point
// form of the knowledge axis. asOf cannot be combined with knownFrom/knownTo.
func (b *Brain) QueryFactsWindow(ctx context.Context, namespaceSlugs []string, since, until, asOf, from, to, knownFrom, knownTo *time.Time, tags []string, page Pagination) ([]models.Fact, error) {
	if err := checkFactWindows(asOf, from, to, knownFrom, knownTo); err != nil {
		return nil, err
	}
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}

	page = page.Sanitize()

	where, args := factWhereWindow(nsIDs, since, until, asOf, from, to, knownFrom, knownTo)
	where, args = tagWhere(where, args, tags)
	query := `SELECT id, namespace_id, content, embedding, embedding_model, confidence,
	          entity, property, value, valid_from, valid_until, created_at, updated_at, deleted_at, tags
	          FROM facts WHERE ` + where +
		fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, page.Limit, page.Offset)

	rows, err := b.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query facts: %w", err)
	}
	defer rows.Close()

	var facts []models.Fact
	for rows.Next() {
		var f models.Fact
		if err := rows.Scan(
			&f.ID, &f.NamespaceID, &f.Content, &f.Embedding, &f.EmbeddingModel,
			&f.Confidence, &f.Entity, &f.Property, &f.Value,
			&f.ValidFrom, &f.ValidUntil,
			&f.CreatedAt, &f.UpdatedAt, &f.DeletedAt, &f.Tags,
		); err != nil {
			return nil, fmt.Errorf("scan fact: %w", err)
		}
		facts = append(facts, f)
	}
	return facts, rows.Err()
}

// CountFacts counts the facts QueryFactsAsOf would return for the same filters,
// ignoring paging, so a caller can tell a full page from a truncated one.
func (b *Brain) CountFacts(ctx context.Context, namespaceSlugs []string, since, until, asOf *time.Time) (int, error) {
	return b.CountFactsWindow(ctx, namespaceSlugs, since, until, asOf, nil, nil, nil, nil, nil)
}

func (b *Brain) CountFactsWindow(ctx context.Context, namespaceSlugs []string, since, until, asOf, from, to, knownFrom, knownTo *time.Time, tags []string) (int, error) {
	if err := checkFactWindows(asOf, from, to, knownFrom, knownTo); err != nil {
		return 0, err
	}
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return 0, err
	}
	where, args := factWhereWindow(nsIDs, since, until, asOf, from, to, knownFrom, knownTo)
	where, args = tagWhere(where, args, tags)
	return b.countWhere(ctx, "facts", where, args...)
}

// UpdateFactConfidence updates the confidence score of a fact.
func (b *Brain) UpdateFactConfidence(ctx context.Context, factID int64, confidence float32) error {
	_, err := b.pool.Exec(ctx,
		"UPDATE facts SET confidence = $2, updated_at = now() WHERE id = $1",
		factID, confidence,
	)
	if err != nil {
		return fmt.Errorf("update fact confidence: %w", err)
	}
	return nil
}

// GetFact returns a single fact by ID.
func (b *Brain) GetFact(ctx context.Context, factID int64) (*models.Fact, error) {
	var f models.Fact
	err := b.pool.QueryRow(ctx,
		`SELECT id, namespace_id, content, embedding, embedding_model, confidence,
		 entity, property, value, valid_from, valid_until, created_at, updated_at, deleted_at, tags
		 FROM facts WHERE id = $1`,
		factID,
	).Scan(
		&f.ID, &f.NamespaceID, &f.Content, &f.Embedding, &f.EmbeddingModel,
		&f.Confidence, &f.Entity, &f.Property, &f.Value,
		&f.ValidFrom, &f.ValidUntil,
		&f.CreatedAt, &f.UpdatedAt, &f.DeletedAt, &f.Tags,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrFactNotFound
		}
		return nil, fmt.Errorf("get fact: %w", err)
	}
	return &f, nil
}

// QueryRelationships returns relationships across namespaces matching the given slug paths.
// Each path matches itself and all descendants.
func (b *Brain) QueryRelationships(ctx context.Context, namespaceSlugs []string, page Pagination) ([]models.Relationship, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}

	page = page.Sanitize()

	rows, err := b.pool.Query(ctx,
		`SELECT id, namespace_id, from_entity, relation_type, canonical_type, to_entity, confidence, source_fact_id, created_at, deleted_at
		 FROM relationships WHERE namespace_id = ANY($1) AND deleted_at IS NULL ORDER BY id LIMIT $2 OFFSET $3`,
		nsIDs, page.Limit, page.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query relationships: %w", err)
	}
	defer rows.Close()

	var rels []models.Relationship
	for rows.Next() {
		var r models.Relationship
		if err := rows.Scan(&r.ID, &r.NamespaceID, &r.FromEntity, &r.RelationType, &r.CanonicalType, &r.ToEntity, &r.Confidence, &r.SourceFactID, &r.CreatedAt, &r.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan relationship: %w", err)
		}
		rels = append(rels, r)
	}
	return rels, rows.Err()
}

// QueryPatterns returns patterns across namespaces matching the given slug paths.
// Each path matches itself and all descendants.
func (b *Brain) QueryPatterns(ctx context.Context, namespaceSlugs []string, page Pagination) ([]models.Pattern, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}

	page = page.Sanitize()

	rows, err := b.pool.Query(ctx,
		`SELECT id, namespace_id, content, confidence, source_fact_ids, source_rel_ids, coherence_score, created_at, updated_at, deleted_at
		 FROM patterns WHERE namespace_id = ANY($1) AND deleted_at IS NULL ORDER BY id LIMIT $2 OFFSET $3`,
		nsIDs, page.Limit, page.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query patterns: %w", err)
	}
	defer rows.Close()

	var patterns []models.Pattern
	for rows.Next() {
		var p models.Pattern
		if err := rows.Scan(&p.ID, &p.NamespaceID, &p.Content, &p.Confidence, &p.SourceFactIDs, &p.SourceRelIDs, &p.CoherenceScore, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan pattern: %w", err)
		}
		patterns = append(patterns, p)
	}
	return patterns, rows.Err()
}
