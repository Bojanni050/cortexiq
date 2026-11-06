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

func TestSetModelPricesMatchesRegardlessOfCase(t *testing.T) {
	// Providers disagree about the case of an id, and a mismatch must not turn
	// a call into an unpriced one.
	SetModelPrices(map[string]ModelPrice{"akash/GLM-5.3": {InputPerM: 1, OutputPerM: 2}})
	defer SetModelPrices(nil)

	for _, model := range []string{"akash/GLM-5.3", "akash/glm-5.3", "AKASH/GLM-5.3"} {
		price, ok := priceFor(model)
		if !ok {
			t.Fatalf("%q is unpriced", model)
		}
		if price.InputPerM != 1 || price.OutputPerM != 2 {
			t.Fatalf("%q price = %+v", model, price)
		}
	}
	if _, ok := priceFor("other/model"); ok {
		t.Fatal("a model outside the table is priced")
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
