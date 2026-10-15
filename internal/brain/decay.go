// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
)

// DecayResult describes the outcome of a confidence decay run.
type DecayResult struct {
	FactsDecayed int `json:"facts_decayed"`
	FactsExpired int `json:"facts_expired"`
}

// DecayConfidence reduces confidence of facts not re-observed within the configured window.
// A fact counts as "recently seen" when it was updated or recalled inside the window, so
// knowledge that is actually used does not fade. Facts below the expiry threshold are
// expired (valid_until set). This is pure SQL — no LLM calls.
func (b *Brain) DecayConfidence(ctx context.Context, nsID int64) (DecayResult, error) {
	if b.config.DecayFactor <= 0 || b.config.DecayFactor >= 1 {
		return DecayResult{}, nil
	}

	var result DecayResult

	decayTag, err := b.pool.Exec(ctx,
		`UPDATE facts SET confidence = confidence * $3, updated_at = now()
		 WHERE namespace_id = $1 AND deleted_at IS NULL AND valid_until IS NULL
		 AND GREATEST(updated_at, COALESCE(last_recalled_at, updated_at)) < now() - $2::interval`,
		nsID, b.config.Window.String(), b.config.DecayFactor,
	)
	if err != nil {
		return result, fmt.Errorf("decay confidence: %w", err)
	}
	result.FactsDecayed = int(decayTag.RowsAffected())

	expireTag, err := b.pool.Exec(ctx,
		`UPDATE facts SET valid_until = now(), updated_at = now()
		 WHERE namespace_id = $1 AND deleted_at IS NULL AND valid_until IS NULL
		 AND confidence < $2`,
		nsID, b.config.ExpiryThreshold,
	)
	if err != nil {
		return result, fmt.Errorf("expire low-confidence facts: %w", err)
	}
	result.FactsExpired = int(expireTag.RowsAffected())

	return result, nil
}
