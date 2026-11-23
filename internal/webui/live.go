package webui

import (
	"context"
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/Bojanni050/cortexiq/internal/schedule"
)

// consolidationLiveView is what the Runs page needs and the run rows cannot
// answer: whether a round is going on right now, and when the next one is due.
//
// A run row is written when the run ends, so a run in flight is invisible in
// the table — exactly during the minutes a reader is waiting for it. The numbers
// come from the consolidator's metrics endpoint because the timer exists in that
// process and nowhere else.
type consolidationLiveView struct {
	// Available is false when the gauges were not found: this process does not
	// run the consolidator and could not reach the one that does.
	Available bool   `json:"available"`
	Source    string `json:"source"`
	Error     string `json:"error,omitempty"`
	Mode      string `json:"mode,omitempty"`
	// IntervalSeconds is the timer period; 0 in events-only mode.
	IntervalSeconds float64 `json:"interval_seconds"`
	Running         bool    `json:"running"`
	NamespaceID     int64   `json:"namespace_id,omitempty"`
	// RunningFor is how long the current round has been going.
	RunningFor float64 `json:"running_for_seconds"`
	// NextRunIn is the countdown to the timer's next round; negative when no
	// timer is armed.
	NextRunIn float64 `json:"next_run_in_seconds"`
	// RetryIn is the countdown to an extra round that retries work still
	// waiting behind a failed batch; negative when none is armed.
	RetryIn float64   `json:"retry_in_seconds"`
	Now     time.Time `json:"now"`
}

// consolidationLive reports the loop's state. It is read-only, so the same
// credential that may open the page may ask.
func (h *Handler) consolidationLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	writeJSON(w, http.StatusOK, h.liveState(r.Context()))
}

// liveState reads the gauges the scheduler publishes. It never fails: a page
// that cannot see the consolidator says so, which is better than a countdown to
// a moment nobody can confirm.
func (h *Handler) liveState(ctx context.Context) consolidationLiveView {
	now := time.Now()
	view := consolidationLiveView{NextRunIn: -1, RetryIn: -1, Now: now.UTC()}

	samples, source, err := h.gatherMetrics(ctx)
	view.Source = source
	if err != nil {
		view.Error = err.Error()
		return view
	}
	info, ok := gaugeSample(samples, "consolidation_schedule_info")
	if !ok {
		return view
	}
	view.Available = true
	view.Mode = info.Labels["mode"]
	if s, ok := gaugeSample(samples, "consolidation_schedule_interval_seconds"); ok {
		view.IntervalSeconds = s.Value
	}
	if s, ok := gaugeSample(samples, "consolidation_run_active"); ok {
		view.Running = s.Value >= 1
	}
	if s, ok := gaugeSample(samples, "consolidation_run_namespace_id"); ok && s.Value > 0 {
		view.NamespaceID = int64(s.Value)
	}
	if s, ok := gaugeSample(samples, "consolidation_run_started_timestamp_seconds"); ok {
		view.RunningFor = secondsSince(now, s.Value)
	}
	if s, ok := gaugeSample(samples, "consolidation_next_run_timestamp_seconds"); ok && view.Mode != string(schedule.ModeEvents) {
		view.NextRunIn = secondsUntil(now, s.Value)
	}
	if s, ok := gaugeSample(samples, "consolidation_retry_timestamp_seconds"); ok {
		view.RetryIn = secondsUntil(now, s.Value)
	}
	return view
}

// gaugeSample finds one sample by name. These metrics are single-series, so no
// label filter is needed; a name that is absent means the process publishing it
// is not the one we reached.
func gaugeSample(samples []metricSampleJSON, name string) (metricSampleJSON, bool) {
	for _, sample := range samples {
		if sample.Name == name {
			return sample, true
		}
	}
	return metricSampleJSON{}, false
}

// secondsUntil is how long from now until a Unix timestamp: clamped at zero, and
// negative for "no such moment" so the caller can tell the two apart.
func secondsUntil(now time.Time, unix float64) float64 {
	if unix <= 0 {
		return -1
	}
	left := time.Unix(0, int64(unix*float64(time.Second))).Sub(now).Seconds()
	if left < 0 {
		return 0
	}
	return math.Round(left*10) / 10
}

// secondsSince is the mirror image: how long ago a Unix timestamp was.
func secondsSince(now time.Time, unix float64) float64 {
	if unix <= 0 {
		return 0
	}
	spent := now.Sub(time.Unix(0, int64(unix*float64(time.Second)))).Seconds()
	if spent < 0 {
		return 0
	}
	return math.Round(spent*10) / 10
}
