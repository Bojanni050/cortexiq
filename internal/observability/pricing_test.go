package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	return m.GetCounter().GetValue()
}

func TestParsePrices(t *testing.T) {
	prices, err := ParsePrices("edenai/qwen3-30b-a3b-instruct-2507=0.20:0.60, text-embedding-3-small=0.02:0")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(prices) != 2 {
		t.Fatalf("want 2 entries, got %d", len(prices))
	}
	got := prices["edenai/qwen3-30b-a3b-instruct-2507"]
	if got.InputPerM != 0.20 || got.OutputPerM != 0.60 {
		t.Fatalf("unexpected price for reasoner model: %+v", got)
	}
	if prices["text-embedding-3-small"].OutputPerM != 0 {
		t.Fatalf("expected zero output price")
	}
}

func TestParsePricesInvalid(t *testing.T) {
	for _, in := range []string{"no-equals", "model=1", "model=a:b", "model=1:-2", "=1:2"} {
		if _, err := ParsePrices(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

func TestRecordReasonerCost(t *testing.T) {
	model := "test-model-priced"
	SetModelPrices(map[string]ModelPrice{model: {InputPerM: 1, OutputPerM: 2}})

	counter := reasonerCost.WithLabelValues("structured", model)
	before := counterValue(t, counter)
	if !recordReasonerCost("structured", model, 1_000_000, 1_000_000) {
		t.Fatal("expected the model to be priced")
	}
	if diff := counterValue(t, counter) - before; diff < 2.999 || diff > 3.001 {
		t.Fatalf("expected cost 3.0, got %v", diff)
	}
}

func TestRecordReasonerCostUnpriced(t *testing.T) {
	model := "test-model-unpriced"
	unpriced := reasonerUnpriced.WithLabelValues("patterns", model)
	before := counterValue(t, unpriced)
	if recordReasonerCost("patterns", model, 1000, 1000) {
		t.Fatal("expected the model to be unpriced")
	}
	if after := counterValue(t, unpriced); after != before+1 {
		t.Fatalf("expected unpriced counter +1, got %v -> %v", before, after)
	}
}
