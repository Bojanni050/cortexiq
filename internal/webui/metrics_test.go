package webui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The dashboard process in the split deployment never records consolidation or
// reasoner counters. With a source configured it must show the samples of the
// process that does, and authenticate against that endpoint.
func TestMetricsReadsConfiguredSource(t *testing.T) {
	var gotAuth string
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(`# HELP consolidation_facts_created_total Facts created.
# TYPE consolidation_facts_created_total counter
consolidation_facts_created_total{namespace="/projects/cortexiq"} 7
# TYPE reasoner_cost_total counter
reasoner_cost_total{stage="structured"} 0.5
`))
	}))
	defer src.Close()

	h := &Handler{MetricsSource: src.URL, MetricsToken: "tok"}
	samples, source, err := h.gatherMetrics(context.Background())
	if err != nil {
		t.Fatalf("gatherMetrics: %v", err)
	}
	if source != "mcp" {
		t.Fatalf("source = %q, want mcp", source)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("Authorization = %q, want Bearer tok", gotAuth)
	}
	cost, ok := sampleValue(samples, "reasoner_cost_total", "stage", "structured")
	if !ok || cost != 0.5 {
		t.Fatalf("reasoner_cost_total = %v (found %v), want 0.5", cost, ok)
	}
	if _, ok := sampleValue(samples, "consolidation_facts_created_total", "namespace", "/projects/cortexiq"); !ok {
		t.Fatalf("consolidation_facts_created_total missing from %v", samples)
	}
}

// A broken peer must not take the page down: the dashboard falls back to its
// own registry and says so.
func TestMetricsFallsBackToLocalRegistry(t *testing.T) {
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusServiceUnavailable)
	}))
	defer src.Close()

	h := &Handler{MetricsSource: src.URL}
	samples, source, err := h.gatherMetrics(context.Background())
	if err != nil {
		t.Fatalf("gatherMetrics: %v", err)
	}
	if source != "local" {
		t.Fatalf("source = %q, want local", source)
	}
	if _, ok := sampleValue(samples, "go_goroutines", "", ""); !ok {
		t.Fatalf("expected local runtime samples, got %d", len(samples))
	}
}

// The endpoint reports where the numbers came from, so the page can say it.
func TestMetricsEndpointReportsSource(t *testing.T) {
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("# TYPE reasoner_requests_total counter\nreasoner_requests_total{stage=\"structured\",status=\"ok\"} 3\n"))
	}))
	defer src.Close()

	mux := http.NewServeMux()
	(&Handler{MetricsSource: src.URL}).Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/metrics = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); !strings.Contains(got, `"source":"mcp"`) {
		t.Fatalf("response does not report the source: %s", got)
	}
}

func sampleValue(samples []metricSampleJSON, name, label, want string) (float64, bool) {
	for _, sample := range samples {
		if sample.Name != name {
			continue
		}
		if label != "" && sample.Labels[label] != want {
			continue
		}
		return sample.Value, true
	}
	return 0, false
}
