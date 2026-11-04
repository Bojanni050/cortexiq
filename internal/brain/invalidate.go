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

// ErrExternalKeyUnknown means no episode under this namespace ever held the
// external_key: there is nothing live and nothing retired. An invalidation
// that names a key the caller never wrote is a caller bug, and pretending it
// succeeded would hide a desynchronised mirror.
var ErrExternalKeyNotFound = errors.New("brain: no episode with this external_key in this namespace")

// InvalidateResult is the outcome of a keyed invalidation.
type InvalidateResult struct {
	// ID is the episode the call retired, or — on a repeat — the one that a
	// previous call already retired. No row was written on a repeat.
	ID int64 `json:"id"`
	// Content is the claim as it was stored, so the answer says what stopped
	// being true rather than only that something was.
	Content string `json:"content"`
	// FactsEnded counts derived facts closed because the retired episode was
	// their only live source.
	FactsEnded int64 `json:"facts_ended"`
	// AlreadyInvalidated is true when the key held no live episode; the call
	// was a replay and changed nothing.
	AlreadyInvalidated bool `json:"already_invalidated"`
	// Retraction is the recorded withdrawal. With a reason it is a judgement
	// (Judged true) and tombstones the claim; without one it is an audit entry
	// (Judged false) that blocks nothing. Omitted on a replay: nothing was written.
	Retraction *Retraction `json:"retraction,omitempty"`
}

// InvalidateKeyed retires the live episode holding the given external_key in
// the namespace — a pure invalidation, the shape of a rejection: no new
// content is written, the statement simply stops being current. Because the
// caller addresses the row by its own key (`gaia:hypothesis:42`), the call is
// a replay-safe step of a mirror rebuild: the second call reports
// AlreadyInvalidated instead of failing.
//
// It differs from a supersede write (remember + supersedes), which retires the
// old version only while putting a new one in its place, and from retract, which
// withdraws a memory by a content match. A reason is optional: when one is
// given, the withdrawal is recorded as a retraction, so a later reader sees
// what was rejected and why instead of silently finding nothing.
//
// The retirement, the fact closing and the retraction record happen in one
// transaction: a failed call must leave the old row live, never make the key
// resolve to nothing.
func (b *Brain) InvalidateKeyed(ctx context.Context, namespaceSlug, externalKey, reason, actor string) (InvalidateResult, error) {
	var out InvalidateResult

	if err := requireWrite(ctx); err != nil {
		return out, err
	}
	key, err := NormalizeExternalKey(externalKey)
	if err != nil {
		return out, err
	}
	if err := validatePath(namespaceSlug); err != nil {
		return out, err
	}
	var reasonPtr *string
	if s := strings.TrimSpace(reason); s != "" {
		if err := validateReason(s); err != nil {
			return out, err
		}
		reasonPtr = &s
	}

	nsID, err := b.resolveNamespaceID(ctx, namespaceSlug)
	if err != nil {
		return out, err
	}

	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return out, fmt.Errorf("invalidate: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var id int64
	var content string
	err = tx.QueryRow(ctx,
		`SELECT id, content FROM episodes
		  WHERE namespace_id = $1 AND external_key = $2 AND deleted_at IS NULL
		  FOR UPDATE`,
		nsID, key,
	).Scan(&id, &content)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Nothing live under the key. Did the key ever hold a live episode that
		// a previous call (or a forget, or a forget-like path) retired? Then
		// the invalidation is a replay and the answer points at that row.
		var retired int64
		var retiredContent string
		err = tx.QueryRow(ctx,
			`SELECT id, content FROM episodes
			  WHERE namespace_id = $1 AND external_key = $2 AND deleted_at IS NOT NULL
			  ORDER BY deleted_at DESC, id DESC LIMIT 1`,
			nsID, key,
		).Scan(&retired, &retiredContent)
		if errors.Is(err, pgx.ErrNoRows) {
			return out, ErrExternalKeyNotFound
		}
		if err != nil {
			return out, fmt.Errorf("find retired episode: %w", err)
		}
		out = InvalidateResult{ID: retired, Content: retiredContent, AlreadyInvalidated: true}
		if err := tx.Commit(ctx); err != nil {
			return out, fmt.Errorf("invalidate: commit: %w", err)
		}
		return out, nil
	case err != nil:
		return out, fmt.Errorf("lock external key: %w", err)
	}

	// Retire the episode. The key was unique among live rows, so once this row
	// is tombstoned the key resolves to nothing — exactly what a rejection is.
	if _, err := tx.Exec(ctx,
		`UPDATE episodes SET deleted_at = now() WHERE id = $1`, id,
	); err != nil {
		return out, fmt.Errorf("retire invalidated episode: %w", err)
	}

	// With a reason the withdrawal is a judgement and tombstones the claim.
	// Without one it is still recorded — who retired what, and when — but as an
	// unjudged audit entry that blocks nothing: the caller made no claim about
	// whether the statement was true.
	reasonText, judged := noJudgement, false
	if reasonPtr != nil {
		reasonText, judged = *reasonPtr, true
	}
	retraction, err := b.recordRetractionInTx(ctx, tx, id, nsID, content, reasonText, actor, judged)
	if err != nil {
		return out, err
	}

	ended, err := endFactsOfEpisode(ctx, tx, id)
	if err != nil {
		return out, err
	}
	out = InvalidateResult{ID: id, Content: content, FactsEnded: ended, Retraction: retraction}

	if err := tx.Commit(ctx); err != nil {
		return out, fmt.Errorf("invalidate: commit: %w", err)
	}
	return out, nil
}

// recordRetractionInTx books the withdrawal inside an open transaction; the
// caller commits it together with the retirement. A retraction recorded after
// an uncommitted retirement would contradict it: the row is already gone when
// the record lands.
func (b *Brain) recordRetractionInTx(ctx context.Context, tx pgx.Tx, targetID, nsID int64, contentText, reason, actor string, judged bool) (*Retraction, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "unknown"
	}
	r := &Retraction{
		NamespaceID: nsID,
		TargetType:  "episode",
		TargetID:    targetID,
		Content:     contentText,
		Reason:      reason,
		RetractedBy: actor,
		Judged:      judged,
	}
	id, createdAt, err := insertRetractionRow(ctx, tx, nsID, "episode", targetID, contentText, reason, actor, judged)
	if err != nil {
		return nil, fmt.Errorf("invalidate: record retraction: %w", err)
	}
	r.ID = id
	r.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	return r, nil
}
