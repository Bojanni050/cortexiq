// Modifications copyright (c) 2026 Bojan van der Heide
package observability

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// memStore is the in-memory CounterStore used by the tests.
type memStore struct {
	mu     sync.Mutex
	values []CounterValue
}

func (m *memStore) Load(context.Context) ([]CounterValue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CounterValue, len(m.values))
	copy(out, m.values)
	return out, nil
}

func (m *memStore) Save(_ context.Context, values []CounterValue) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values = append(m.values, values...)
	return nil
}

// gatheredCounter reads one counter series from the default registry.
func gatheredCounter(t *testing.T, name string, want map[string]string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if labelsMatch(metric, want) {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func labelsMatch(metric *dto.Metric, want map[string]string) bool {
	if len(metric.GetLabel()) != len(want) {
		return false
	}
	for _, pair := range metric.GetLabel() {
		if want[pair.GetName()] != pair.GetValue() {
			return false
		}
	}
	return true
}

// Restore must add the saved totals back into the live counters, including the
// cost counters defined in pricing.go.
func TestRestoreAddsPersistedTotals(t *testing.T) {
	namespace := "restore-test"
	model := "restore-test-model"
	store := &memStore{values: []CounterValue{
		{Name: "consolidation_facts_created_total", Labels: map[string]string{"namespace": namespace}, Value: 42},
		{Name: "reasoner_cost_total", Labels: map[string]string{"stage": "structured", "model": model}, Value: 1.5},
		// A histogram and an unknown counter are not restorable and must be
		// skipped without failing the whole restore.
		{Name: "consolidation_duration_seconds", Labels: map[string]string{"namespace": namespace}, Value: 99},
		{Name: "http_requests_total", Labels: map[string]string{"path": "/"}, Value: 7},
	}}

	if err := Restore(context.Background(), store); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := gatheredCounter(t, "consolidation_facts_created_total", map[string]string{"namespace": namespace}); got != 42 {
		t.Fatalf("facts counter = %v, want 42", got)
	}
	if got := gatheredCounter(t, "reasoner_cost_total", map[string]string{"stage": "structured", "model": model}); got != 1.5 {
		t.Fatalf("cost counter = %v, want 1.5", got)
	}
}

// Flush must save this package's counter series and leave runtime metrics and
// the duration histogram out of the snapshot.
func TestFlushPersistsOwnCounters(t *testing.T) {
	namespace := "flush-test"
	labels := map[string]string{"namespace": namespace}
	before := gatheredCounter(t, "consolidation_facts_created_total", labels)
	RecordConsolidation(Observation{Namespace: namespace, FactsCreated: 7, LLMCalls: 3, Duration: 2 * time.Second})

	store := &memStore{}
	if err := Flush(context.Background(), store); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	var facts *CounterValue
	for i, v := range store.values {
		switch {
		case v.Name == "consolidation_facts_created_total" && v.Labels["namespace"] == namespace:
			facts = &store.values[i]
		case v.Name == "consolidation_duration_seconds",
			v.Name == "go_goroutines",
			v.Name == "process_cpu_seconds_total":
			t.Fatalf("Flush saved %q, which it must not persist", v.Name)
		}
	}
	if facts == nil {
		t.Fatal("Flush did not save consolidation_facts_created_total")
	}
	if want := before + 7; facts.Value != want {
		t.Fatalf("saved facts = %v, want %v", facts.Value, want)
	}
}

// The stored key is the JSON label set; encoding/json sorts the keys, so the
// same series always reads back as the same row.
func TestLabelsRoundTrip(t *testing.T) {
	labels := map[string]string{"namespace": "/projects/cortexiq", "stage": "structured"}
	raw, err := encodeLabels(labels)
	if err != nil {
		t.Fatalf("encodeLabels: %v", err)
	}
	if want := `{"namespace":"/projects/cortexiq","stage":"structured"}`; raw != want {
		t.Fatalf("encodeLabels = %s, want %s", raw, want)
	}
	back, err := decodeLabels(raw)
	if err != nil {
		t.Fatalf("decodeLabels: %v", err)
	}
	if len(back) != len(labels) {
		t.Fatalf("decodeLabels = %v, want %v", back, labels)
	}
	for k, v := range labels {
		if back[k] != v {
			t.Fatalf("decodeLabels[%q] = %q, want %q", k, back[k], v)
		}
	}

	empty, err := encodeLabels(nil)
	if err != nil || empty != "{}" {
		t.Fatalf("encodeLabels(nil) = %q, %v; want {}", empty, err)
	}
	if decoded, err := decodeLabels(empty); err != nil || len(decoded) != 0 {
		t.Fatalf("decodeLabels({}) = %v, %v; want empty", decoded, err)
	}
	if _, err := decodeLabels("not json"); err == nil {
		t.Fatal("decodeLabels accepted invalid JSON")
	}
}
