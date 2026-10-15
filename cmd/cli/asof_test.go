package main

import (
	"testing"
	"time"
)

func TestParseAsOf(t *testing.T) {
	if got, err := parseAsOf("  "); got != nil || err != nil {
		t.Errorf("empty means now: got %v, %v", got, err)
	}
	got, err := parseAsOf("2026-09-01")
	if err != nil || !got.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("date: got %v, %v", got, err)
	}
	got, err = parseAsOf("2026-09-01T12:30:00+02:00")
	if err != nil || !got.Equal(time.Date(2026, 9, 1, 10, 30, 0, 0, time.UTC)) {
		t.Errorf("rfc3339: got %v, %v", got, err)
	}
	if _, err := parseAsOf("yesterday"); err == nil {
		t.Error("garbage should be rejected")
	}
}
