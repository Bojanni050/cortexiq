// Package schedule decides when the background consolidator runs: after new
// remember events, on a timer, or both. The setting is global and lives in the
// settings table so the dashboard can change it without a restart or a flag.
package schedule

import (
	"fmt"
	"time"
)

// Mode is how the consolidator decides to run.
type Mode string

const (
	// ModeEvents runs only after new episodes are remembered.
	ModeEvents Mode = "events"
	// ModeEventsInterval runs after new episodes and on a timer as a safety
	// net, so a missed event or a write from another process still gets picked
	// up.
	ModeEventsInterval Mode = "events_interval"
	// ModeInterval runs only on a timer: the behaviour from before the setting
	// existed.
	ModeInterval Mode = "interval"
)

// DefaultInterval is what an untouched install has always used.
const DefaultInterval = 5 * time.Minute

// Interval choices the dashboard offers. Anything else is rejected so a
// hand-edited row cannot hammer the reasoner.
var Intervals = []time.Duration{
	30 * time.Second,
	time.Minute,
	2 * time.Minute,
	3 * time.Minute,
	4 * time.Minute,
	5 * time.Minute,
	10 * time.Minute,
	30 * time.Minute,
}

// Config is the global consolidation schedule.
type Config struct {
	Mode Mode `json:"mode"`
	// IntervalSeconds is only used when the mode has a timer.
	IntervalSeconds int `json:"interval_seconds"`
}

// DefaultConfig is interval-only every five minutes, matching the old flag
// default.
func DefaultConfig() Config {
	return Config{Mode: ModeInterval, IntervalSeconds: int(DefaultInterval / time.Second)}
}

// ConfigFromInterval builds the timer-only config a CLI flag describes.
func ConfigFromInterval(d time.Duration) Config {
	return Config{Mode: ModeInterval, IntervalSeconds: int(d / time.Second)}
}

// Interval is the timer period. It is meaningless in ModeEvents.
func (c Config) Interval() time.Duration {
	return time.Duration(c.IntervalSeconds) * time.Second
}

// UsesTimer reports whether a periodic run is part of this mode.
func (c Config) UsesTimer() bool { return c.Mode != ModeEvents }

// TriggersOnEvents reports whether a remember event starts a run.
func (c Config) TriggersOnEvents() bool { return c.Mode != ModeInterval }

// Validate rejects a mode or interval the scheduler cannot honour.
func (c Config) Validate() error {
	switch c.Mode {
	case ModeEvents, ModeEventsInterval, ModeInterval:
	default:
		return fmt.Errorf("schedule: unknown mode %q (want events, events_interval or interval)", c.Mode)
	}
	for _, d := range Intervals {
		if c.IntervalSeconds == int(d/time.Second) {
			return nil
		}
	}
	return fmt.Errorf("schedule: interval must be one of 30s, 1m, 2m, 3m, 4m, 5m, 10m, 30m; got %ds", c.IntervalSeconds)
}

// IntervalChoices returns the allowed periods in seconds, oldest first, for
// the API and the dashboard.
func IntervalChoices() []int {
	out := make([]int, len(Intervals))
	for i, d := range Intervals {
		out[i] = int(d / time.Second)
	}
	return out
}

// Modes returns the allowed modes for the API and the dashboard.
func Modes() []Mode {
	return []Mode{ModeEvents, ModeEventsInterval, ModeInterval}
}
