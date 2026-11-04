package webui

import (
	"errors"
	"net/http"
	"runtime"
	"time"

	"github.com/Bojanni050/cortexiq/internal/brain"
	"github.com/Bojanni050/cortexiq/internal/schedule"
	"github.com/Bojanni050/cortexiq/internal/version"
)

// serverStarted is set when the package initialises, which for a served binary
// is process start: the webui package only loads as part of the server, so the
// difference between "package init" and "program start" is race-noise with the
// honest answer the dashboard needs — how long has this process been up.
var serverStarted = time.Now()

// statusCheck is one component's pulse. A failed check answers with its error
// beside ok:false, so the dashboard can say what is wrong instead of only that
// something is.
type statusCheck struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// statusView is what /api/v1/status reports. Every field degrades to an absent
// value plus an error string rather than failing the whole answer: a server
// with a broken database must still be able to describe itself, which is when
// the reader needs this page most.
type statusView struct {
	// Build identity. Version "dev" is what a binary built from a working tree
	// reports, matching `cortexiq version` so the two cannot disagree.
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Built     string `json:"built"`
	GoVersion string `json:"go_version"`

	StartedAt     time.Time `json:"started_at"`
	UptimeSeconds float64   `json:"uptime_seconds"`

	// Database is the liveness pulse: a ping, not a workload query.
	Database statusCheck `json:"database"`

	// Schedule carries the consolidator's mode and interval; nil plus
	// ScheduleError when it could not be read.
	Schedule      *schedule.Config `json:"schedule,omitempty"`
	ScheduleError string           `json:"schedule_error,omitempty"`

	// Consolidation is the backlog (episodes waiting for extraction) and when
	// extraction last ran, computed over every namespace. Nil plus
	// ConsolidationError when it could not be read.
	Consolidation      *brain.ConsolidationStatus `json:"consolidation,omitempty"`
	ConsolidationError string                     `json:"consolidation_error,omitempty"`
}

// shortCommit trims the full SHA to the 7 characters the release notes use.
// The build site stores the full SHA; the dashboard has no room for it.
func shortCommit(c string) string {
	if c == "" || c == "none" {
		return "none"
	}
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

// check runs one health probe and turns its outcome into a statusCheck. The
// nil-Brain branch exists for the tests in this package, which construct the
// handler without one: the report says "brain not configured" rather than
// claiming either health or a failure that did not happen.
func check(err error) statusCheck {
	if err == nil {
		return statusCheck{OK: true}
	}
	return statusCheck{OK: false, Error: err.Error()}
}

// status answers "is CortexIQ healthy": what binary is running, how long it has
// been up, whether the database answers, and what the consolidator is doing.
// It is a dashboard endpoint, not a load balancer probe — the existing health
// endpoints stay the cheap machine-readable ones, and this one gathers a
// fuller picture.
func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	v := statusView{
		Version:   version.Version,
		Commit:    shortCommit(version.Commit),
		Built:     version.Date,
		GoVersion: runtime.Version(),

		StartedAt:     serverStarted,
		UptimeSeconds: time.Since(serverStarted).Seconds(),
	}
	if h.Brain != nil {
		v.Database = check(h.Brain.Health(r.Context()))
		if st, err := h.Brain.ConsolidationStatus(r.Context(), namespacesParam(r)); err == nil {
			v.Consolidation = st
		} else {
			v.ConsolidationError = err.Error()
		}
		if cfg, err := h.Brain.ScheduleConfig(r.Context()); err == nil {
			v.Schedule = &cfg
		} else {
			v.ScheduleError = err.Error()
		}
	} else {
		v.Database = statusCheck{OK: false, Error: "brain not configured"}
	}
	writeJSON(w, http.StatusOK, v)
}
