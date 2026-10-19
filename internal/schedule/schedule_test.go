package schedule

import (
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{"events ignores the interval", Config{Mode: ModeEvents, IntervalSeconds: 30}, true},
		{"events plus interval", Config{Mode: ModeEventsInterval, IntervalSeconds: 600}, true},
		{"interval only", Config{Mode: ModeInterval, IntervalSeconds: 1800}, true},
		{"default", DefaultConfig(), true},
		{"unknown mode", Config{Mode: "sometimes", IntervalSeconds: 300}, false},
		{"empty mode", Config{IntervalSeconds: 300}, false},
		{"interval too short", Config{Mode: ModeInterval, IntervalSeconds: 5}, false},
		{"interval not offered", Config{Mode: ModeInterval, IntervalSeconds: 90}, false},
		{"interval zero", Config{Mode: ModeEventsInterval, IntervalSeconds: 0}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.ok && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if !tt.ok && err == nil {
				t.Fatalf("Validate() = nil, want an error")
			}
		})
	}
}

func TestConfigBehaviour(t *testing.T) {
	events := Config{Mode: ModeEvents, IntervalSeconds: 300}
	if events.UsesTimer() {
		t.Error("events mode must not run a timer")
	}
	if !events.TriggersOnEvents() {
		t.Error("events mode must react to remembers")
	}

	both := Config{Mode: ModeEventsInterval, IntervalSeconds: 600}
	if !both.UsesTimer() || !both.TriggersOnEvents() {
		t.Error("events_interval must use both triggers")
	}

	interval := Config{Mode: ModeInterval, IntervalSeconds: 60}
	if !interval.UsesTimer() {
		t.Error("interval mode must run a timer")
	}
	if interval.TriggersOnEvents() {
		t.Error("interval mode must ignore remembers")
	}
	if interval.Interval() != time.Minute {
		t.Errorf("Interval() = %s, want 1m", interval.Interval())
	}
}

func TestIntervalChoicesMatchIntervals(t *testing.T) {
	choices := IntervalChoices()
	if len(choices) != len(Intervals) {
		t.Fatalf("got %d choices for %d intervals", len(choices), len(Intervals))
	}
	for i, d := range Intervals {
		if choices[i] != int(d/time.Second) {
			t.Errorf("choice %d = %ds, want %s", i, choices[i], d)
		}
	}
	if len(Modes()) != 3 {
		t.Errorf("got %d modes, want 3", len(Modes()))
	}
}
