// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"errors"
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

// ErrRejectedValue means the fact was refused because the same value was
// rejected earlier in this namespace and the rejection has not been lifted.
// The concrete error is a *RejectedValueError, which says which retraction and
// why; errors.Is matches this sentinel.
var ErrRejectedValue = errors.New("brain: this value was rejected earlier")

// RejectedValueError carries the retraction that blocked a fact.
type RejectedValueError struct {
	RetractionID int64
	Reason       string
	RetractedAt  time.Time
}

func (e *RejectedValueError) Error() string {
	return fmt.Sprintf("brain: this value was rejected on %s (retraction %d): %s",
		e.RetractedAt.UTC().Format("2006-01-02"), e.RetractionID, e.Reason)
}

func (e *RejectedValueError) Is(target error) bool { return target == ErrRejectedValue }

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
	if err := checkRejectedValue(ctx, q, in); err != nil {
		return 0, err
	}
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

// checkRejectedValue refuses a fact whose value was rejected before. The key is
// the value, not the row: a rejected claim asserted again in a new write, or
// extracted again from a new episode, is a different row with the same value,
// and only the value tells them apart from a true restatement.
//
// The scope is the exact namespace. A rejection in one project says nothing
// about another. The evaluation harness writes into a throwaway namespace and
// is exempt. A lookup that fails fails the write: a check that fails open
// would let the value back in without a trace, which is the failure this exists
// to prevent. The caller records the error and retries.
func checkRejectedValue(ctx context.Context, q factRower, in factInsert) error {
	if in.Origin == originEval {
		return nil
	}
	key, ok := valueKey(in.Content)
	if !ok {
		return nil
	}
	var r RejectedValueError
	err := q.QueryRow(ctx,
		`SELECT id, reason, created_at FROM retractions
		  WHERE namespace_id = $1 AND value_key = $2 AND lifted_at IS NULL
		  ORDER BY id DESC LIMIT 1`,
		in.NamespaceID, key,
	).Scan(&r.RetractionID, &r.Reason, &r.RetractedAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("check rejected value (%s): %w", in.Origin, err)
	}
	return &r
}
