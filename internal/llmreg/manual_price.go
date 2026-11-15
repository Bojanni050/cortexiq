// Modifications copyright (c) 2026 Bojan van der Heide
package llmreg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrPriceInvalid means a manually entered rate cannot be used: no model name,
// a negative rate, or two rates that are both zero.
var ErrPriceInvalid = errors.New("llmreg: invalid price")

// ManualPrice is a rate entered in the dashboard for a model no provider
// catalogue prices. It is a fallback and not an override: when a catalogue
// starts publishing a rate for the same model, that rate wins and this row
// stops being used without being deleted.
type ManualPrice struct {
	Model         string  `json:"model"`
	InputPerMTok  float64 `json:"input_per_mtok"`
	OutputPerMTok float64 `json:"output_per_mtok"`
	UpdatedAt     string  `json:"updated_at"`
}

// ManualPrices lists the rates entered in the dashboard, ordered by model so the
// Settings table does not jump around after an edit.
func (s *Store) ManualPrices(ctx context.Context) ([]ManualPrice, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT model, input_per_mtok, output_per_mtok, updated_at
		 FROM model_prices ORDER BY model_key`)
	if err != nil {
		return nil, fmt.Errorf("list model prices: %w", err)
	}
	defer rows.Close()

	out := []ManualPrice{}
	for rows.Next() {
		var p ManualPrice
		var updated time.Time
		if err := rows.Scan(&p.Model, &p.InputPerMTok, &p.OutputPerMTok, &updated); err != nil {
			return nil, fmt.Errorf("list model prices: scan: %w", err)
		}
		p.UpdatedAt = updated.UTC().Format(time.RFC3339)
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetManualPrice stores one rate, replacing the row for the same model. The
// match is case-insensitive, like the cost lookup it feeds.
func (s *Store) SetManualPrice(ctx context.Context, model string, in, out float64) (ManualPrice, error) {
	if model = strings.TrimSpace(model); model == "" {
		return ManualPrice{}, fmt.Errorf("%w: a model name is required", ErrPriceInvalid)
	}
	if in < 0 || out < 0 {
		return ManualPrice{}, fmt.Errorf("%w: rates cannot be negative", ErrPriceInvalid)
	}
	if in == 0 && out == 0 {
		return ManualPrice{}, fmt.Errorf("%w: at least one rate must be above zero", ErrPriceInvalid)
	}

	var saved ManualPrice
	var updated time.Time
	err := s.pool.QueryRow(ctx,
		`INSERT INTO model_prices (model, input_per_mtok, output_per_mtok)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (model_key) DO UPDATE SET
		     model = EXCLUDED.model,
		     input_per_mtok = EXCLUDED.input_per_mtok,
		     output_per_mtok = EXCLUDED.output_per_mtok,
		     updated_at = now()
		 RETURNING model, input_per_mtok, output_per_mtok, updated_at`,
		model, in, out,
	).Scan(&saved.Model, &saved.InputPerMTok, &saved.OutputPerMTok, &updated)
	if err != nil {
		return ManualPrice{}, fmt.Errorf("save model price: %w", err)
	}
	saved.UpdatedAt = updated.UTC().Format(time.RFC3339)
	return saved, nil
}

// DeleteManualPrice removes a rate. Deleting a model that has no row is not an
// error: the dashboard's remove button must be safe to press twice.
func (s *Store) DeleteManualPrice(ctx context.Context, model string) error {
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM model_prices WHERE model_key = lower($1)`, strings.TrimSpace(model)); err != nil {
		return fmt.Errorf("delete model price: %w", err)
	}
	return nil
}
