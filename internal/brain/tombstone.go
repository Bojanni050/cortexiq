// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// insertRetractionRow is the one place a row enters retractions. It stores the
// value key next to the claim, so the retraction can be found by what it
// withdrew and not only by the row it withdrew. A claim with no usable key is
// stored without one and never blocks anything.
func insertRetractionRow(ctx context.Context, tx pgx.Tx, nsID int64, targetType string, targetID int64, content, reason, actor string) (id int64, createdAt time.Time, err error) {
	var key *string
	if k, ok := valueKey(content); ok {
		key = &k
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO retractions (namespace_id, target_type, target_id, content, reason, retracted_by, value_key)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at`,
		nsID, targetType, targetID, content, reason, actor, key,
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
			  WHERE value_key IS NULL AND id > $1
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
