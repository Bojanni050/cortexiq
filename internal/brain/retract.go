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

var (
	// ErrReasonRequired means a retraction arrived without a reason. A
	// retraction is a claim about the world ("this was wrong"), so it carries
	// the correction that replaced it; without one it is just a delete.
	ErrReasonRequired = fmt.Errorf("brain: a retraction needs a reason — say what was wrong and what is true instead")
	// ErrAlreadyRetracted means this target already has a retraction record.
	ErrAlreadyRetracted = fmt.Errorf("brain: already retracted")
)

// Retraction is one recorded withdrawal. Content is the claim as it was
// stored, kept so the record says what was withdrawn rather than only that
// something was.
type Retraction struct {
	ID          int64  `json:"id"`
	NamespaceID int64  `json:"namespace_id"`
	TargetType  string `json:"target_type"`
	TargetID    int64  `json:"target_id"`
	Content     string `json:"content"`
	Reason      string `json:"reason"`
	RetractedBy string `json:"retracted_by"`
	CreatedAt   string `json:"created_at"`
}

const maxReasonLen = 2000

func validateReason(reason string) error {
	if strings.TrimSpace(reason) == "" {
		return ErrReasonRequired
	}
	if len(reason) > maxReasonLen {
		return fmt.Errorf("brain: reason exceeds %d characters", maxReasonLen)
	}
	return nil
}

// RetractEpisode withdraws the episode that best matches the query: it is
// soft-deleted like a forget, but the reason is recorded alongside the claim.
//
// Retract and forget are different statements. Forget means "stop using this",
// with no judgement about whether it was true. Retract means "this was wrong",
// and the record keeps what was withdrawn and why, so a later reader can see
// the correction instead of silently finding nothing.
func (b *Brain) RetractEpisode(ctx context.Context, namespaceSlugs []string, query, reason, actor string) (*Retraction, error) {
	if err := validateReason(reason); err != nil {
		return nil, err
	}
	id, nsID, content, err := b.findEpisode(ctx, namespaceSlugs, query)
	if err != nil {
		return nil, err
	}
	return b.recordRetraction(ctx, "episode", id, nsID, content, reason, actor, true)
}

// RetractEpisodeByID withdraws one episode by id. Callers that already hold the
// id — the dashboard, a previous listing — do not need to search for the memory
// first, and must not: two episodes can be close enough for a query to land on
// the wrong one.
func (b *Brain) RetractEpisodeByID(ctx context.Context, episodeID int64, reason, actor string) (*Retraction, error) {
	if err := validateReason(reason); err != nil {
		return nil, err
	}

	var (
		nsID    int64
		content string
	)
	err := b.pool.QueryRow(ctx,
		`SELECT namespace_id, content FROM episodes WHERE id = $1`, episodeID,
	).Scan(&nsID, &content)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEpisodeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load episode to retract: %w", err)
	}

	return b.recordRetraction(ctx, "episode", episodeID, nsID, content, reason, actor, true)
}

// RetractFact withdraws a stored fact by id.
func (b *Brain) RetractFact(ctx context.Context, factID int64, reason, actor string) (*Retraction, error) {
	if err := validateReason(reason); err != nil {
		return nil, err
	}

	var (
		nsID    int64
		content string
	)
	err := b.pool.QueryRow(ctx,
		`SELECT namespace_id, content FROM facts WHERE id = $1`, factID,
	).Scan(&nsID, &content)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFactNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load fact to retract: %w", err)
	}

	return b.recordRetraction(ctx, "fact", factID, nsID, content, reason, actor, true)
}

// recordRetraction writes the retraction and, when softDelete is set,
// tombstones the target in the same transaction. A retraction that is recorded
// but does not hide the memory would be worse than none at all.
func (b *Brain) recordRetraction(ctx context.Context, targetType string, targetID, nsID int64, content, reason, actor string, softDelete bool) (*Retraction, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "unknown"
	}

	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("retract: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var existing int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM retractions WHERE target_type = $1 AND target_id = $2`,
		targetType, targetID,
	).Scan(&existing); err != nil {
		return nil, fmt.Errorf("retract: check existing: %w", err)
	}
	if existing > 0 {
		return nil, ErrAlreadyRetracted
	}

	r := &Retraction{
		NamespaceID: nsID,
		TargetType:  targetType,
		TargetID:    targetID,
		Content:     content,
		Reason:      reason,
		RetractedBy: actor,
	}
	var createdAt time.Time
	if err := tx.QueryRow(ctx,
		`INSERT INTO retractions (namespace_id, target_type, target_id, content, reason, retracted_by)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at`,
		nsID, targetType, targetID, content, reason, actor,
	).Scan(&r.ID, &createdAt); err != nil {
		return nil, fmt.Errorf("retract: insert: %w", err)
	}
	r.CreatedAt = createdAt.UTC().Format(time.RFC3339)

	if softDelete {
		table := "episodes"
		if targetType == "fact" {
			table = "facts"
		}
		// table is one of two literals above, never caller input.
		if _, err := tx.Exec(ctx,
			"UPDATE "+table+" SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL", targetID,
		); err != nil {
			return nil, fmt.Errorf("retract: tombstone %s: %w", table, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("retract: commit: %w", err)
	}
	return r, nil
}

// Retractions lists recorded withdrawals in the given namespaces, newest
// first. It is how a reader finds the correction rather than the hole.
func (b *Brain) Retractions(ctx context.Context, namespaceSlugs []string, limit int) ([]Retraction, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}

	rows, err := b.pool.Query(ctx,
		`SELECT id, namespace_id, target_type, target_id, content, reason, retracted_by, created_at
		 FROM retractions WHERE namespace_id = ANY($1)
		 ORDER BY id DESC LIMIT $2`, nsIDs, limit)
	if err != nil {
		return nil, fmt.Errorf("list retractions: %w", err)
	}
	defer rows.Close()

	out := []Retraction{}
	for rows.Next() {
		var r Retraction
		var createdAt time.Time
		if err := rows.Scan(&r.ID, &r.NamespaceID, &r.TargetType, &r.TargetID,
			&r.Content, &r.Reason, &r.RetractedBy, &createdAt); err != nil {
			return nil, fmt.Errorf("list retractions: scan: %w", err)
		}
		r.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		out = append(out, r)
	}
	return out, rows.Err()
}
