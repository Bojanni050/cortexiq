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
	// manualPoll is how often the rates entered in the dashboard are re-read.
	// They live in the database because the process that records the costs is
	// not always the process the dashboard writes from, so a save cannot be
	// signalled in memory. A minute is close enough to "in use" without a query
	// per request.
	manualPoll = time.Minute
)

// Source is one OpenAI-compatible endpoint whose catalogue states prices.
type Source struct {
	Name    string
	BaseURL string
	APIKey  string
}

// PriceStore is the rate table the dashboard writes. It is a fallback, not an
// override: see mergePrices.
type PriceStore interface {
	ManualPrices(ctx context.Context) ([]llmreg.ManualPrice, error)
}

// Refresher reads the catalogues and publishes the merged price table.
type Refresher struct {
	sources []Source
	client  *http.Client
	log     *slog.Logger
	// manual is the dashboard's fallback table. Nil means this process has no
	// such table, which is also the case in tests that only read catalogues.
	manual PriceStore

	mu sync.Mutex
	// bySource remembers the last good table per provider, so one provider
	// failing does not erase the prices the others gave us.
	bySource map[string]map[string]observability.ModelPrice
	// catalogueFold indexes the folded model ids that at least one catalogue
	// prices. It is what lets the dashboard say whether a rate entered by hand
	// is in use or has been taken over by a catalogue.
	catalogueFold map[string]bool
	// lastManual keeps the last table read from the dashboard, so a failed read
	// does not turn priced calls into unpriced ones for a minute.
	lastManual map[string]observability.ModelPrice
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
		sources:       unique,
		client:        &http.Client{Timeout: timeout},
		log:           log,
		bySource:      map[string]map[string]observability.ModelPrice{},
		catalogueFold: map[string]bool{},
	}
}

// SetManualPrices attaches the dashboard's fallback table. Call it before Start:
// it is the one field that is not guarded by the lock because the refresher is
// built and configured before its goroutine runs.
func (r *Refresher) SetManualPrices(store PriceStore) { r.manual = store }

// Refresh reads every catalogue and installs the merged table. It returns how
// many models the catalogues price; zero means nothing could be read this time,
// which is the signal to retry sooner rather than in six hours.
func (r *Refresher) Refresh(ctx context.Context) int {
	type fetched struct {
		name  string
		table map[string]observability.ModelPrice
		err   error
	}
	// The fetches run outside the lock: a catalogue that hangs for the timeout
	// must not hold up a rate saved in the dashboard meanwhile.
	results := make([]fetched, 0, len(r.sources))
	for _, s := range r.sources {
		table, err := r.fetch(ctx, s)
		results = append(results, fetched{name: s.Name, table: table, err: err})
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	read := 0
	for _, res := range results {
		if res.err != nil {
			r.log.Warn("model prices: catalogue unreadable, keeping the last known prices",
				"provider", res.name, "err", res.err)
			continue
		}
		read++
		r.bySource[res.name] = res.table
	}
	r.reindexLocked()
	models, catalogued := r.publishLocked(ctx)
	if read == 0 {
		// Every catalogue failed. The cached tables and the dashboard rates
		// still describe the same prices, so this only applies a rate saved in
		// the meantime; the zero answer asks Start to try again soon.
		return 0
	}
	r.log.Info("model prices refreshed",
		"models", models, "from_catalogues", catalogued, "providers", read)
	return catalogued
}

// Republish installs the table again without touching the catalogues, for when
// the rates entered in the dashboard changed. Other processes pick the same rows
// up within manualPoll.
func (r *Refresher) Republish(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.publishLocked(ctx)
}

// CataloguePriced reports whether a provider catalogue publishes a rate for this
// model, which decides whether a rate entered in the dashboard is in use or has
// been taken over. Names compare case-insensitively, like the cost lookup.
func (r *Refresher) CataloguePriced(model string) bool {
	if model == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.catalogueFold[strings.ToLower(model)]
}

// publishLocked merges, installs, and reports the merged size and how many of
// those models came from a catalogue.
func (r *Refresher) publishLocked(ctx context.Context) (models, catalogued int) {
	catalogue := r.catalogueLocked()
	merged := mergePrices(r.manualLocked(ctx), catalogue)
	observability.SetModelPrices(merged)
	return len(merged), len(catalogue)
}

// catalogueLocked flattens the per-provider tables into one, in source order, so
// the last provider to price a model wins — the same precedence the table had
// before manual rates existed.
func (r *Refresher) catalogueLocked() map[string]observability.ModelPrice {
	out := map[string]observability.ModelPrice{}
	for _, s := range r.sources {
		for model, price := range r.bySource[s.Name] {
			out[model] = price
		}
	}
	return out
}

// reindexLocked rebuilds the folded set of catalogue-priced model ids.
func (r *Refresher) reindexLocked() {
	fold := make(map[string]bool, len(r.catalogueFold))
	for _, s := range r.sources {
		for model := range r.bySource[s.Name] {
			fold[strings.ToLower(model)] = true
		}
	}
	r.catalogueFold = fold
}

// manualLocked reads the dashboard's rates. A read that fails keeps the last
// table: losing it would silently turn priced calls into unpriced ones.
func (r *Refresher) manualLocked(ctx context.Context) map[string]observability.ModelPrice {
	if r.manual == nil {
		return nil
	}
	rows, err := r.manual.ManualPrices(ctx)
	if err != nil {
		r.log.Warn("model prices: cannot read the rates entered in the dashboard, keeping the last known", "err", err)
		return r.lastManual
	}
	out := make(map[string]observability.ModelPrice, len(rows))
	for _, p := range rows {
		out[p.Model] = observability.ModelPrice{InputPerM: p.InputPerMTok, OutputPerM: p.OutputPerMTok}
	}
	r.lastManual = out
	return out
}

// mergePrices puts the catalogues first. A published rate outranks a typed one
// even when the two spell the model's name with different capitalisation: the
// cost lookup folds case, so an exact-hit manual row would otherwise quietly
// overrule the catalogue for that spelling.
func mergePrices(manual, catalogue map[string]observability.ModelPrice) map[string]observability.ModelPrice {
	out := make(map[string]observability.ModelPrice, len(manual)+len(catalogue))
	published := make(map[string]bool, len(catalogue))
	for model, price := range catalogue {
		out[model] = price
		published[strings.ToLower(model)] = true
	}
	for model, price := range manual {
		if published[strings.ToLower(model)] {
			continue
		}
		out[model] = price
	}
	return out
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

// Start runs the refresher until ctx is cancelled: the catalogues once
// immediately and then on the interval, and the dashboard's own rates once a
// minute. It never returns an error, because a missing price table costs an
// estimate, not a request.
func (r *Refresher) Start(ctx context.Context) {
	if len(r.sources) == 0 && r.manual == nil {
		r.log.Warn("model prices: no provider to read prices from, cost estimates stay empty")
		return
	}
	if len(r.sources) > 0 {
		go r.refreshLoop(ctx)
	}
	if r.manual != nil {
		go r.manualLoop(ctx)
	}
}

func (r *Refresher) refreshLoop(ctx context.Context) {
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
}

// manualLoop picks up a rate saved by another process. A save from this process
// is published at once by the handler, so the first read here is only about
// starting from the stored table.
func (r *Refresher) manualLoop(ctx context.Context) {
	r.Republish(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(manualPoll):
			r.Republish(ctx)
		}
	}
}
