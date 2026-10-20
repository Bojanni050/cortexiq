// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"errors"
	"fmt"

	"github.com/Bojanni050/cortexiq/internal/scope"
	"github.com/jackc/pgx/v5"
)

// Fact feedback is the one signal the evidence model cannot produce on its own.
// Observation and time say how well a fact is supported; only the caller can say
// whether it was actually right. It therefore moves rank and nothing else: it
// never touches confidence, validity or decay, because those are evidence the
// fact earned and a vote is not new evidence.
const (
	// feedbackStep is how much one net vote moves a fact's rank multiplier.
	feedbackStep = 0.25
	// feedbackFloor and feedbackCeiling bound that multiplier. A flood of
	// downvotes may push a fact to the back, never out of the result set: decay
	// retires facts, votes do not.
	feedbackFloor   = 0.5
	feedbackCeiling = 1.25
)

// ErrBadVerdict means the caller passed something other than +1 or -1.
var ErrBadVerdict = errors.New("brain: verdict must be 1 (helpful) or -1 (wrong)")

// FactFeedback is the outcome of one vote: the verdict that was recorded, the
// net over all principals, and the rank multiplier that net produces.
type FactFeedback struct {
	FactID   int64   `json:"fact_id"`
	Verdict  int     `json:"verdict"`
	Net      int     `json:"net"`
	Factor   float64 `json:"factor"`
	Replaced bool    `json:"replaced"`
}

// RateFact records one caller's verdict on one fact. Each principal keeps one
// standing vote: a caller that changes its mind replaces its own verdict instead
// of adding a second one, so no principal can win an argument by repetition, and
// the history stays readable in fact_feedback.
//
// It refuses read-only callers and facts that do not exist or were deleted, and
// it is the only way a vote is ever written — there is no path from recall or
// consolidation into this table.
func (b *Brain) RateFact(ctx context.Context, factID int64, verdict int, note string) (FactFeedback, error) {
	if verdict != 1 && verdict != -1 {
		return FactFeedback{}, ErrBadVerdict
	}
	if factID <= 0 {
		return FactFeedback{}, fmt.Errorf("brain: fact id must be positive")
	}
	if err := requireWrite(ctx); err != nil {
		return FactFeedback{}, err
	}

	var nsID int64
	err := b.pool.QueryRow(ctx,
		`SELECT namespace_id FROM facts WHERE id = $1 AND deleted_at IS NULL`,
		factID,
	).Scan(&nsID)
	if errors.Is(err, pgx.ErrNoRows) {
		return FactFeedback{}, fmt.Errorf("brain: fact %d does not exist", factID)
	}
	if err != nil {
		return FactFeedback{}, fmt.Errorf("brain: look up fact %d: %w", factID, err)
	}

	principal := principalName(ctx)

	var previous int
	err = b.pool.QueryRow(ctx,
		`SELECT verdict FROM fact_feedback
		 WHERE fact_id = $1 AND principal = $2
		 ORDER BY created_at DESC, id DESC LIMIT 1`,
		factID, principal,
	).Scan(&previous)
	replaced := err == nil && previous != verdict
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return FactFeedback{}, fmt.Errorf("brain: read previous verdict: %w", err)
	}

	if _, err := b.pool.Exec(ctx,
		`INSERT INTO fact_feedback (fact_id, namespace_id, principal, verdict, note)
		 VALUES ($1, $2, $3, $4, $5)`,
		factID, nsID, principal, verdict, note,
	); err != nil {
		return FactFeedback{}, fmt.Errorf("brain: record verdict: %w", err)
	}

	nets, err := b.feedbackNet(ctx, []int64{factID})
	if err != nil {
		return FactFeedback{}, err
	}
	net := nets[factID]
	return FactFeedback{
		FactID:   factID,
		Verdict:  verdict,
		Net:      net,
		Factor:   feedbackFactor(net),
		Replaced: replaced,
	}, nil
}

// feedbackNet returns the net verdict per fact, counting only the latest vote of
// each principal.
func (b *Brain) feedbackNet(ctx context.Context, ids []int64) (map[int64]int, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := b.pool.Query(ctx,
		`SELECT fact_id, sum(verdict)
		 FROM (
			SELECT DISTINCT ON (fact_id, principal) fact_id, principal, verdict
			FROM fact_feedback
			WHERE fact_id = ANY($1)
			ORDER BY fact_id, principal, created_at DESC, id DESC
		 ) latest
		 GROUP BY fact_id`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("brain: read feedback: %w", err)
	}
	defer rows.Close()

	out := make(map[int64]int, len(ids))
	for rows.Next() {
		var id int64
		var net int
		if err := rows.Scan(&id, &net); err != nil {
			return nil, fmt.Errorf("brain: scan feedback: %w", err)
		}
		out[id] = net
	}
	return out, rows.Err()
}

// feedbackFactor turns a net verdict into a rank multiplier, bounded at both
// ends and 1 for a tie.
func feedbackFactor(net int) float64 {
	f := 1 + feedbackStep*float64(net)
	if f < feedbackFloor {
		return feedbackFloor
	}
	if f > feedbackCeiling {
		return feedbackCeiling
	}
	return f
}

// applyFeedback attaches each fact's net verdict to the fused results. It runs
// before applyBoosts, which turns the net into a rank multiplier, so a downvoted
// fact can drop out of the answer before the list is cut to limit. It is best
// effort: a failed lookup leaves the ranking exactly as it was.
func (b *Brain) applyFeedback(ctx context.Context, rs []RecallResult) {
	var ids []int64
	for _, r := range rs {
		if r.Type == "fact" {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	nets, err := b.feedbackNet(ctx, ids)
	if err != nil {
		return
	}
	for i := range rs {
		if rs[i].Type != "fact" {
			continue
		}
		rs[i].Feedback = nets[rs[i].ID]
	}
}

// anyFeedback reports whether any result carries a vote, which is what forces
// applyBoosts to rescale even when recency and importance are switched off.
func anyFeedback(rs []RecallResult) bool {
	for _, r := range rs {
		if r.Feedback != 0 {
			return true
		}
	}
	return false
}

// principalName is the key a vote is stored under: the authenticated token's
// name, or "local" for a caller without one (a tokenless local setup, or the
// CLI). A missing name therefore shares one standing vote instead of creating a
// new identity per call.
func principalName(ctx context.Context) string {
	if p, ok := scope.From(ctx); ok && p.Name != "" {
		return p.Name
	}
	return "local"
}
