package observability

import (
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
	// pricesFold indexes the same table by lower-cased model id. Providers
	// disagree about the case of an id ("akash/GLM-5.3" vs "akash/glm-5.3"),
	// and a case mismatch must not silently turn a call unpriced.
	pricesFold = map[string]ModelPrice{}

	reasonerCost = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reasoner_cost_total",
			Help: "Estimated reasoner cost in USD, from token usage",
		}, []string{"stage", "model"},
	)
	reasonerUnpriced = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reasoner_unpriced_calls_total",
			Help: "Reasoner calls with token usage but no known price, so cost is not counted",
		}, []string{"stage", "model"},
	)
	reasonerPricedModels = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "reasoner_priced_models",
			Help: "Models in the price table, read from the providers' catalogues",
		},
	)
)

// SetModelPrices replaces the price table. The table is read from the
// providers themselves (see internal/modelprice), so it holds the whole
// catalogue rather than the two or three models a hand-written table named.
//
// No series are materialised here on purpose: a catalogue of several hundred
// models times every stage would be thousands of empty series in Prometheus.
// A series appears the first time a model is actually called.
func SetModelPrices(p map[string]ModelPrice) {
	folded := make(map[string]ModelPrice, len(p))
	for model, price := range p {
		folded[strings.ToLower(model)] = price
	}

	pricesMu.Lock()
	prices = p
	pricesFold = folded
	pricesMu.Unlock()

	reasonerPricedModels.Set(float64(len(p)))
}

func priceFor(model string) (ModelPrice, bool) {
	pricesMu.RLock()
	defer pricesMu.RUnlock()
	if price, ok := prices[model]; ok {
		return price, true
	}
	price, ok := pricesFold[strings.ToLower(model)]
	return price, ok
}

// recordReasonerCost adds the estimated cost of one request. It returns false
// when the model has no known price, so those calls stay visible.
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
