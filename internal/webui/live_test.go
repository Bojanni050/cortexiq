package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// liveSource serves the gauges the scheduler publishes.
func liveSource(t *testing.T, body string) *httptest.Server {
	t.Helper()
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(src.Close)
	return src
}

func liveRequest(t *testing.T, h *Handler) consolidationLiveView {
	t.Helper()
	mux := http.NewServeMux()
	h.Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/consolidation/live", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/consolidation/live = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var view consolidationLiveView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	return view
}

// The Runs page asks whether a round is going on now; the run rows are written
// when it ends, so the answer has to come from the loop's own gauges.
func TestConsolidationLiveReportsRunningRound(t *testing.T) {
	now := time.Now()
	src := liveSource(t, fmt.Sprintf(`# TYPE consolidation_schedule_info gauge
consolidation_schedule_info{mode="interval"} 1
# TYPE consolidation_schedule_interval_seconds gauge
consolidation_schedule_interval_seconds 300
# TYPE consolidation_run_active gauge
consolidation_run_active 1
# TYPE consolidation_run_started_timestamp_seconds gauge
consolidation_run_started_timestamp_seconds %d
# TYPE consolidation_run_namespace_id gauge
consolidation_run_namespace_id 12
# TYPE consolidation_next_run_timestamp_seconds gauge
consolidation_next_run_timestamp_seconds %d
# TYPE consolidation_retry_timestamp_seconds gauge
consolidation_retry_timestamp_seconds 0
`, now.Add(-30*time.Second).Unix(), now.Add(90*time.Second).Unix()))

	view := liveRequest(t, &Handler{MetricsSource: src.URL})

	if !view.Available || view.Source != "mcp" {
		t.Fatalf("available = %v, source = %q; want a readable mcp peer", view.Available, view.Source)
	}
	if !view.Running || view.NamespaceID != 12 || view.Mode != "interval" || view.IntervalSeconds != 300 {
		t.Fatalf("view = %+v, want a running round in namespace 12 on a 5m timer", view)
	}
	if view.RunningFor < 29 || view.RunningFor > 32 {
		t.Errorf("running_for_seconds = %v, want about 30", view.RunningFor)
	}
	if view.NextRunIn < 88 || view.NextRunIn > 92 {
		t.Errorf("next_run_in_seconds = %v, want about 90", view.NextRunIn)
	}
	// The timer is 0 when nothing is armed, which must not read as "retry now".
	if view.RetryIn != -1 {
		t.Errorf("retry_in_seconds = %v, want -1 for no armed retry", view.RetryIn)
	}
}

// In events-only mode there is no timer to count down to, and the page must say
// that instead of inventing a moment.
func TestConsolidationLiveHasNoCountdownInEventsMode(t *testing.T) {
	src := liveSource(t, `# TYPE consolidation_schedule_info gauge
consolidation_schedule_info{mode="events"} 1
# TYPE consolidation_schedule_interval_seconds gauge
consolidation_schedule_interval_seconds 0
# TYPE consolidation_run_active gauge
consolidation_run_active 0
# TYPE consolidation_next_run_timestamp_seconds gauge
consolidation_next_run_timestamp_seconds 0
`)

	view := liveRequest(t, &Handler{MetricsSource: src.URL})

	if !view.Available || view.Running {
		t.Fatalf("view = %+v, want an idle loop that answered", view)
	}
	if view.NextRunIn != -1 || view.IntervalSeconds != 0 {
		t.Fatalf("next_run_in_seconds = %v, interval = %v; an events-only schedule has no timer", view.NextRunIn, view.IntervalSeconds)
	}
}

// A peer without these gauges (an older release, or any other process) leaves
// the page with no answer rather than a fabricated one.
func TestConsolidationLiveWithoutGaugesIsUnavailable(t *testing.T) {
	src := liveSource(t, "# TYPE go_goroutines gauge\ngo_goroutines 42\n")

	view := liveRequest(t, &Handler{MetricsSource: src.URL})

	if view.Available {
		t.Fatalf("available = true without the schedule gauges: %+v", view)
	}
	if view.Error != "" {
		t.Fatalf("error = %q, want none: a peer that answered is not an error", view.Error)
	}
}

func TestConsolidationLiveRejectsWrites(t *testing.T) {
	mux := http.NewServeMux()
	(&Handler{}).Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/consolidation/live", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/v1/consolidation/live = %d, want 405", rec.Code)
	}
}
