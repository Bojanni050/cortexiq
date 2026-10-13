package observability

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ModelPrice is the price per one million tokens for one model.
type ModelPrice struct {
	InputPerM  float64
	OutputPerM float64
}

var (
	pricesMu sync.RWMutex
	prices   = map[string]ModelPrice{}

	reasonerCost = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reasoner_cost_total",
			Help: "Estimated reasoner cost in the configured price unit, from token usage",
		}, []string{"stage", "model"},
	)
	reasonerUnpriced = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reasoner_unpriced_calls_total",
			Help: "Reasoner calls with token usage but no configured price, so cost is not counted",
		}, []string{"stage", "model"},
	)
)

// ParsePrices parses "model=input:output,..." where prices are per million
// tokens. Model names may contain slashes and hyphens; the input/output pair is
// separated by a colon to keep those safe.
func ParsePrices(s string) (map[string]ModelPrice, error) {
	out := map[string]ModelPrice{}
	for _, entry := range strings.Split(s, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		model, rest, ok := strings.Cut(entry, "=")
		model = strings.TrimSpace(model)
		if !ok || model == "" {
			return nil, fmt.Errorf("model prices: %q is not model=input:output", entry)
		}
		in, outTok, ok := strings.Cut(rest, ":")
		if !ok {
			return nil, fmt.Errorf("model prices: %q is missing ':output'", entry)
		}
		inPrice, err := strconv.ParseFloat(strings.TrimSpace(in), 64)
		if err != nil || inPrice < 0 {
			return nil, fmt.Errorf("model prices: invalid input price in %q", entry)
		}
		outPrice, err := strconv.ParseFloat(strings.TrimSpace(outTok), 64)
		if err != nil || outPrice < 0 {
			return nil, fmt.Errorf("model prices: invalid output price in %q", entry)
		}
		out[model] = ModelPrice{InputPerM: inPrice, OutputPerM: outPrice}
	}
	return out, nil
}

// SetModelPrices installs the price table and materialises zero-valued cost
// series for the configured models, so the dashboard shows them immediately.
func SetModelPrices(p map[string]ModelPrice) {
	pricesMu.Lock()
	prices = p
	pricesMu.Unlock()
	for model := range p {
		for _, stage := range reasonerStages {
			reasonerCost.WithLabelValues(stage, model)
			reasonerUnpriced.WithLabelValues(stage, model)
		}
	}
}

func priceFor(model string) (ModelPrice, bool) {
	pricesMu.RLock()
	defer pricesMu.RUnlock()
	price, ok := prices[model]
	return price, ok
}

// recordReasonerCost adds the estimated cost of one request. It returns false
// when the model has no configured price, so those calls stay visible.
func recordReasonerCost(stage, model string, promptTokens, completionTokens int64) bool {
	price, ok := priceFor(model)
	if !ok {
		reasonerUnpriced.WithLabelValues(stage, model).Inc()
		return false
	}
	cost := float64(promptTokens)/1e6*price.InputPerM + float64(completionTokens)/1e6*price.OutputPerM
	reasonerCost.WithLabelValues(stage, model).Add(cost)
	return true
}
