package brain

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Bojanni050/cortexiq/internal/models"
)

const (
	defaultEntityContextLimit = 50
	maxEntityContextLimit     = 200
)

// ErrEntityNameRequired means an entity lookup came without a name.
var ErrEntityNameRequired = errors.New("brain: entity name is required")

// EntityRelation is one live relation that has the entity on either end.
type EntityRelation struct {
	ID         int64   `json:"id"`
	From       string  `json:"from"`
	Type       string  `json:"type"`
	To         string  `json:"to"`
	Confidence float32 `json:"confidence"`
	FactID     *int64  `json:"fact_id,omitempty"`
}

// EntityContext is everything the memory holds under one exact spelling of an
// entity: the facts that name it, the relations it takes part in, and the
// episodes those facts were consolidated from. The Clean-up tab shows it so a
// reader can check what a spelling stands for before merging or removing it.
type EntityContext struct {
	Name           string           `json:"name"`
	Facts          []models.Fact    `json:"-"`
	FactsTotal     int              `json:"facts_total"`
	Relations      []EntityRelation `json:"relations"`
	RelationsTotal int              `json:"relations_total"`
	Episodes       []models.Episode `json:"-"`
	EpisodesTotal  int              `json:"episodes_total"`
}

// EntityContext reads one spelling, exactly as stored (surrounding spaces
// aside), because the Clean-up tab lists spellings separately and its counts
// must match what is shown here. Newest first; each list holds at most limit
// rows and reports its real total.
func (b *Brain) EntityContext(ctx context.Context, namespaceSlugs []string, name string, limit int) (*EntityContext, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrEntityNameRequired
	}
	if limit <= 0 {
		limit = defaultEntityContextLimit
	}
	if limit > maxEntityContextLimit {
		limit = maxEntityContextLimit
	}
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}
	out := &EntityContext{Name: name, Facts: []models.Fact{}, Relations: []EntityRelation{}, Episodes: []models.Episode{}}

	rows, err := b.pool.Query(ctx,
		`SELECT id, namespace_id, content, confidence, entity, property, value,
		        valid_from, valid_until, created_at, updated_at, tags, count(*) OVER()
		   FROM facts
		  WHERE namespace_id = ANY($1) AND deleted_at IS NULL AND btrim(entity) = $2
		  ORDER BY created_at DESC, id DESC LIMIT $3`, nsIDs, name, limit)
	if err != nil {
		return nil, fmt.Errorf("entity facts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var f models.Fact
		if err := rows.Scan(&f.ID, &f.NamespaceID, &f.Content, &f.Confidence, &f.Entity, &f.Property, &f.Value,
			&f.ValidFrom, &f.ValidUntil, &f.CreatedAt, &f.UpdatedAt, &f.Tags, &out.FactsTotal); err != nil {
			return nil, fmt.Errorf("scan entity fact: %w", err)
		}
		out.Facts = append(out.Facts, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	rel, err := b.pool.Query(ctx,
		`SELECT id, btrim(from_entity), relation_type, btrim(to_entity), confidence, source_fact_id, count(*) OVER()
		   FROM relationships
		  WHERE namespace_id = ANY($1) AND deleted_at IS NULL
		    AND (btrim(from_entity) = $2 OR btrim(to_entity) = $2)
		  ORDER BY confidence DESC, id LIMIT $3`, nsIDs, name, limit)
	if err != nil {
		return nil, fmt.Errorf("entity relations: %w", err)
	}
	defer rel.Close()
	for rel.Next() {
		var r EntityRelation
		if err := rel.Scan(&r.ID, &r.From, &r.Type, &r.To, &r.Confidence, &r.FactID, &out.RelationsTotal); err != nil {
			return nil, fmt.Errorf("scan entity relation: %w", err)
		}
		out.Relations = append(out.Relations, r)
	}
	if err := rel.Err(); err != nil {
		return nil, err
	}
	rel.Close()

	// The episodes behind the facts that name the entity and behind the facts its
	// relations were read from: both are evidence for the spelling.
	eps, err := b.pool.Query(ctx,
		`SELECT e.id, e.namespace_id, e.content, e.occurred_at, e.created_at, e.tags, count(*) OVER()
		   FROM episodes e
		  WHERE e.deleted_at IS NULL AND e.namespace_id = ANY($1)
		    AND EXISTS (
		      SELECT 1 FROM fact_sources s
		       WHERE s.episode_id = e.id AND s.fact_id IN (
		         SELECT f.id FROM facts f
		          WHERE f.namespace_id = ANY($1) AND f.deleted_at IS NULL AND btrim(f.entity) = $2
		         UNION
		         SELECT r.source_fact_id FROM relationships r
		          WHERE r.namespace_id = ANY($1) AND r.deleted_at IS NULL AND r.source_fact_id IS NOT NULL
		            AND (btrim(r.from_entity) = $2 OR btrim(r.to_entity) = $2)))
		  ORDER BY e.occurred_at DESC, e.id DESC LIMIT $3`, nsIDs, name, limit)
	if err != nil {
		return nil, fmt.Errorf("entity episodes: %w", err)
	}
	defer eps.Close()
	for eps.Next() {
		var e models.Episode
		if err := eps.Scan(&e.ID, &e.NamespaceID, &e.Content, &e.OccurredAt, &e.CreatedAt, &e.Tags, &out.EpisodesTotal); err != nil {
			return nil, fmt.Errorf("scan entity episode: %w", err)
		}
		out.Episodes = append(out.Episodes, e)
	}
	return out, eps.Err()
}
