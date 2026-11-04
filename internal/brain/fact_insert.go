// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

// Fact origins. They say which path a fact came in through. They change no
// behaviour today; they exist so a rule that must treat one path differently
// (an import restoring history, a harness fixture) has something to branch on
// in the one place every fact passes.
const (
	originConsolidation = "consolidation"
	originHypothesis    = "hypothesis"
	originFailure       = "failure_pattern"
	originImport        = "import"
	originEval          = "eval"
)

// factRower is the part of a pgx pool or transaction insertFact needs, so a
// caller that is already inside a transaction can write its fact in it.
type factRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// factInsert is everything a fact is born with. Zero values mean what the
// column default means: no entity, no validity bounds, created now, neutral
// importance, no tags and no refs.
type factInsert struct {
	Origin      string
	NamespaceID int64
	Content     string
	Embedding   []float32
	// EmbeddingModel is the model that produced Embedding.
	EmbeddingModel string
	Confidence     float32

	Entity, Property, Value *string

	ValidFrom, ValidUntil, KnownUntil *time.Time
	// CreatedAt is nil for "now"; an import restoring history sets it.
	CreatedAt *time.Time

	// Importance 0 is a real value; callers that have no opinion pass
	// DefaultImportance.
	Importance   float32
	Tags         []string
	ExternalRefs []string
}

// insertFact is the single place a row enters the facts table. Every path that
// creates a fact (consolidation, a confirmed hypothesis, a failure pattern, an
// import, an evaluation fixture) goes through it, so a check added here — a
// rejected-value tombstone, a source requirement — applies to all of them at
// once. TestFactsAreInsertedInOnePlace keeps it that way.
func (b *Brain) insertFact(ctx context.Context, q factRower, in factInsert) (int64, error) {
	var id int64
	err := q.QueryRow(ctx,
		`INSERT INTO facts (namespace_id, content, embedding, embedding_model, confidence,
		                    entity, property, value, valid_from, valid_until, known_until,
		                    created_at, importance, tags, external_refs)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
		         COALESCE($12, now()), $13, $14, $15)
		 RETURNING id`,
		in.NamespaceID, in.Content, pgvector.NewVector(in.Embedding), in.EmbeddingModel, in.Confidence,
		in.Entity, in.Property, in.Value, in.ValidFrom, in.ValidUntil, in.KnownUntil,
		in.CreatedAt, in.Importance, tagsOrEmpty(in.Tags), refsOrEmpty(in.ExternalRefs),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert fact (%s): %w", in.Origin, err)
	}
	return id, nil
}
