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

// DecayConfidence reduces the standing of facts not re-observed within the configured window.
// A fact counts as "recently seen" when it was updated or recalled inside the window, so
// knowledge that is actually used does not fade. Facts below the expiry threshold are
// expired (valid_until set). This is pure SQL — no LLM calls.
//
// By default the thing that decays is confidence, and confidence is also what expires
// the fact. With DecaySplit on, disuse lowers strength instead and expiry looks at
// strength; confidence then moves on evidence only.
func (b *Brain) DecayConfidence(ctx context.Context, nsID int64) (DecayResult, error) {
	if b.config.DecayFactor <= 0 || b.config.DecayFactor >= 1 {
		return DecayResult{}, nil
	}

	// column is one of two literals, never caller input.
	column := "confidence"
	if b.config.DecaySplit {
		column = "strength"
	}

	var result DecayResult

	decayTag, err := b.pool.Exec(ctx,
		`UPDATE facts SET `+column+` = `+column+` * $3, updated_at = now()
		 WHERE namespace_id = $1 AND deleted_at IS NULL AND valid_until IS NULL
		 AND GREATEST(updated_at, COALESCE(last_recalled_at, updated_at)) < now() - $2::interval`,
		nsID, b.config.Window.String(), b.config.DecayFactor,
	)
	if err != nil {
		return result, fmt.Errorf("decay confidence: %w", err)
	}
	result.FactsDecayed = int(decayTag.RowsAffected())

	// Under the split, expiry means disuse, and only that is reversible: the
	// marker lets an observation of the same claim bring the fact back.
	marker := ""
	if b.config.DecaySplit {
		marker = ", expired_by_disuse_at = now()"
	}
	expireTag, err := b.pool.Exec(ctx,
		`UPDATE facts SET valid_until = now(), known_until = now(), updated_at = now()`+marker+`
		 WHERE namespace_id = $1 AND deleted_at IS NULL AND valid_until IS NULL
		 AND `+column+` < $2`,
		nsID, b.config.ExpiryThreshold,
	)
	if err != nil {
		return result, fmt.Errorf("expire low-confidence facts: %w", err)
	}
	result.FactsExpired = int(expireTag.RowsAffected())

	return result, nil
}
