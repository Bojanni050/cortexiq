// Modifications copyright (c) 2026 Bojan van der Heide
// Package modelprice keeps the cost estimate's price table in step with what
// the providers actually charge.
//
// The table used to be an environment variable, which meant it went stale the
// moment a model was swapped: the cost tile then counted nothing, and nobody
// noticed because an unpriced call only increments a counter. The catalogue a
// provider already serves is the authority, so it is read from there.
package modelprice

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Bojanni050/cortexiq/internal/llmreg"
	"github.com/Bojanni050/cortexiq/internal/observability"
)

const (
	// interval is how often the catalogues are re-read. Provider prices move
	// on the order of months, so an hourly refresh would be pure noise.
	interval = 6 * time.Hour
	// retry is the wait after a refresh that produced nothing at all, so a
	// provider that was briefly unreachable does not leave the table empty for
	// six hours.
	retry = 5 * time.Minute
	// timeout bounds one catalogue fetch. A provider that hangs must not hold
	// the refresher for the rest of the day.
	timeout = 30 * time.Second
)

// Source is one OpenAI-compatible endpoint whose catalogue states prices.
type Source struct {
	Name    string
	BaseURL string
	APIKey  string
}

// Refresher reads the catalogues and publishes the merged price table.
type Refresher struct {
	sources []Source
	client  *http.Client
	log     *slog.Logger

	mu sync.Mutex
	// bySource remembers the last good table per provider, so one provider
	// failing does not erase the prices the others gave us.
	bySource map[string]map[string]observability.ModelPrice
}

// NewRefresher drops sources that point at the same endpoint, because two
// entries would only fetch the same catalogue twice and race to write it.
func NewRefresher(log *slog.Logger, sources []Source) *Refresher {
	if log == nil {
		log = slog.Default()
	}
	seen := map[string]bool{}
	unique := make([]Source, 0, len(sources))
	for _, s := range sources {
		base := strings.TrimRight(strings.TrimSpace(s.BaseURL), "/")
		if base == "" || seen[base] {
			continue
		}
		seen[base] = true
		s.BaseURL = base
		unique = append(unique, s)
	}
	return &Refresher{
		sources:  unique,
		client:   &http.Client{Timeout: timeout},
		log:      log,
		bySource: map[string]map[string]observability.ModelPrice{},
	}
}

// Refresh reads every catalogue and installs the merged table. It returns how
// many models ended up priced; zero means nothing could be read, in which case
// the previous table is left untouched.
func (r *Refresher) Refresh(ctx context.Context) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	merged := map[string]observability.ModelPrice{}
	read := 0
	for _, s := range r.sources {
		table, err := r.fetch(ctx, s)
		if err != nil {
			r.log.Warn("model prices: catalogue unreadable, keeping the last known prices",
				"provider", s.Name, "url", s.BaseURL, "err", err)
			for model, price := range r.bySource[s.Name] {
				merged[model] = price
			}
			continue
		}
		read++
		r.bySource[s.Name] = table
		for model, price := range table {
			merged[model] = price
		}
	}
	if read == 0 {
		return 0
	}
	observability.SetModelPrices(merged)
	r.log.Info("model prices refreshed", "models", len(merged), "providers", read)
	return len(merged)
}

func (r *Refresher) fetch(ctx context.Context, s Source) (map[string]observability.ModelPrice, error) {
	models, err := llmreg.ListModels(ctx, r.client, llmreg.Provider{
		Name:    s.Name,
		BaseURL: s.BaseURL,
		APIKey:  s.APIKey,
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]observability.ModelPrice, len(models))
	for _, m := range models {
		// A model with no published price stays out of the table rather than
		// being priced at zero, which would look like a free call.
		if m.InputPerMTok <= 0 && m.OutputPerMTok <= 0 {
			continue
		}
		out[m.ID] = observability.ModelPrice{
			InputPerM:  m.InputPerMTok,
			OutputPerM: m.OutputPerMTok,
		}
	}
	return out, nil
}

// Start runs the refresher until ctx is cancelled: once immediately, then on
// the interval. It never returns an error, because a missing price table costs
// an estimate, not a request.
func (r *Refresher) Start(ctx context.Context) {
	if len(r.sources) == 0 {
		r.log.Warn("model prices: no provider to read prices from, cost estimates stay empty")
		return
	}
	go func() {
		wait := time.Duration(0)
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			if r.Refresh(ctx) == 0 {
				wait = retry
				continue
			}
			wait = interval
		}
	}()
}
