// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// insertRetractionRow is the one place a row enters retractions. It stores the
// value key next to the claim, so the retraction can be found by what it
// withdrew and not only by the row it withdrew. A claim with no usable key is
// stored without one and never blocks anything.
//
// judged is false for a withdrawal that made no claim about the world (a forget,
// an invalidate without a reason). Such a row is an audit entry: it never gets a
// key, so it can never block a value, and the table refuses the combination.
func insertRetractionRow(ctx context.Context, tx pgx.Tx, nsID int64, targetType string, targetID int64, content, reason, actor string, judged bool) (id int64, createdAt time.Time, err error) {
	var key *string
	if judged {
		if k, ok := valueKey(content); ok {
			key = &k
		}
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO retractions (namespace_id, target_type, target_id, content, reason, retracted_by, value_key, judged)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id, created_at`,
		nsID, targetType, targetID, content, reason, actor, key, judged,
	).Scan(&id, &createdAt)
	return id, createdAt, err
}

// backfillBatch bounds one pass of BackfillValueKeys.
const backfillBatch = 500

// BackfillValueKeys gives every retraction made before migration 00043 the key
// it would have been written with. Without it an old rejection would sit in the
// table and block nothing, which looks exactly like a tombstone that works.
//
// It is idempotent and meant to run at every start-up: it only reads rows with
// no key. Rows whose claim has no usable key (it normalises to nothing) keep a
// NULL key and are read again next time, which is cheap and bounded by the
// number of retractions. It returns how many rows it keyed.
func (b *Brain) BackfillValueKeys(ctx context.Context) (int, error) {
	total := 0
	var after int64
	for {
		rows, err := b.pool.Query(ctx,
			`SELECT id, content FROM retractions
			  WHERE value_key IS NULL AND judged AND id > $1
			  ORDER BY id LIMIT $2`, after, backfillBatch)
		if err != nil {
			return total, fmt.Errorf("backfill value keys: read: %w", err)
		}
		type pending struct {
			id  int64
			key string
		}
		var batch []pending
		var last int64
		n := 0
		for rows.Next() {
			var id int64
			var content string
			if err := rows.Scan(&id, &content); err != nil {
				rows.Close()
				return total, fmt.Errorf("backfill value keys: scan: %w", err)
			}
			last = id
			n++
			if k, ok := valueKey(content); ok {
				batch = append(batch, pending{id, k})
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return total, fmt.Errorf("backfill value keys: rows: %w", err)
		}
		if n == 0 {
			return total, nil
		}
		for _, p := range batch {
			if _, err := b.pool.Exec(ctx,
				`UPDATE retractions SET value_key = $2 WHERE id = $1 AND value_key IS NULL`, p.id, p.key,
			); err != nil {
				return total, fmt.Errorf("backfill value keys: update %d: %w", p.id, err)
			}
			total++
		}
		after = last
	}
}

var (
	// ErrRetractionNotFound means no retraction has this id.
	ErrRetractionNotFound = errors.New("brain: retraction not found")
	// ErrAlreadyLifted means the rejection was lifted before.
	ErrAlreadyLifted = errors.New("brain: this rejection was already lifted")
	// ErrNotJudged means the record is an audit entry of a forget, or of an
	// invalidate without a reason. It never blocked a value, so there is
	// nothing to lift.
	ErrNotJudged = errors.New("brain: this record made no judgement and blocks nothing; there is nothing to lift")
	// ErrLiftReasonRequired means a lift arrived without a reason. Taking a block
	// away is a decision someone has to be able to read back.
	ErrLiftReasonRequired = errors.New("brain: lifting a rejection needs a reason — say why the value may be stored again")
)

// retractionColumns is the column list scanRetraction reads, in order.
const retractionColumns = `id, namespace_id, target_type, target_id, content, reason, retracted_by, created_at,
	lifted_at, lifted_by, lift_reason, judged`

func scanRetraction(row pgx.Row) (Retraction, error) {
	var (
		r         Retraction
		createdAt time.Time
		liftedAt  *time.Time
	)
	if err := row.Scan(&r.ID, &r.NamespaceID, &r.TargetType, &r.TargetID, &r.Content, &r.Reason,
		&r.RetractedBy, &createdAt, &liftedAt, &r.LiftedBy, &r.LiftReason, &r.Judged); err != nil {
		return r, err
	}
	r.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	if liftedAt != nil {
		s := liftedAt.UTC().Format(time.RFC3339)
		r.LiftedAt = &s
	}
	return r, nil
}

// LiftRetraction lifts a rejection: the value may be stored again. The
// retraction stays as the record of what was withdrawn and why, and now also of
// who lifted it and why. Lifting does not bring the withdrawn row back; the
// value is allowed in again, and a later write stores it as a new fact.
func (b *Brain) LiftRetraction(ctx context.Context, id int64, reason, actor string) (*Retraction, error) {
	if err := requireWrite(ctx); err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, ErrLiftReasonRequired
	}
	if len(reason) > maxReasonLen {
		return nil, fmt.Errorf("brain: lift reason is longer than %d characters", maxReasonLen)
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "unknown"
	}

	tag, err := b.pool.Exec(ctx,
		`UPDATE retractions SET lifted_at = now(), lifted_by = $2, lift_reason = $3
		  WHERE id = $1 AND lifted_at IS NULL AND judged`, id, actor, reason)
	if err != nil {
		return nil, fmt.Errorf("lift retraction: %w", err)
	}
	r, err := scanRetraction(b.pool.QueryRow(ctx,
		`SELECT `+retractionColumns+` FROM retractions WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRetractionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lift retraction: read back: %w", err)
	}
	if tag.RowsAffected() == 0 {
		if !r.Judged {
			return nil, ErrNotJudged
		}
		return nil, ErrAlreadyLifted
	}
	return &r, nil
}
