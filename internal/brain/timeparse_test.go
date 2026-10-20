// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestTemporalWindow(t *testing.T) {
	now := time.Date(2026, time.October, 20, 14, 30, 0, 0, time.FixedZone("test", 2*60*60))
	tests := []struct {
		query string
		from  time.Time
		to    time.Time
	}{
		{"yesterday", time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)},
		{"vandaag", time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)},
		{"last week", time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)},
		{"deze maand", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
		{"3 days ago", time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)},
		{"in maart 2025", time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC)},
		{"in July", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
		{"december 2024", time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			from, to := temporalWindow(tc.query, now)
			if from == nil || to == nil || !from.Equal(tc.from) || !to.Equal(tc.to) {
				t.Fatalf("window = (%v, %v), want (%v, %v)", from, to, tc.from, tc.to)
			}
		})
	}
	for _, query := range []string{"last year", "this week", "2 dagen geleden"} {
		if from, to := temporalWindow(query, now); from == nil || to == nil {
			t.Errorf("%q did not produce a time window", query)
		}
	}
	if from, to := temporalWindow("May I get the current API key?", now); from != nil || to != nil {
		t.Fatalf("non-temporal query unexpectedly parsed: %v, %v", from, to)
	}
}

func TestTemporalWindowDoesNotMatchRecallEvalQueries(t *testing.T) {
	file, err := os.Open("../../eval/recall_queries.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	before := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		var item struct {
			ID    string `json:"id"`
			Query string `json:"query"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			t.Fatal(err)
		}
		count++
		if from, to := temporalWindow(item.Query, before); from != nil || to != nil {
			t.Errorf("eval query %s acquired implicit window: %q", item.ID, item.Query)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 36 {
		t.Fatalf("read %d eval queries, want 36", count)
	}
}

func TestTemporalWindowLastMonthName(t *testing.T) {
	now := time.Date(2026, time.October, 20, 14, 30, 0, 0, time.UTC)
	tests := []struct {
		query string
		from  time.Time
	}{
		{"what happened last March", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		{"vorige maart", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		{"afgelopen december", time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)},
		// The current month named as "last" means a year ago.
		{"last October", time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC)},
		// An explicit year wins over "last".
		{"last march 2024", time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			from, to := temporalWindow(tc.query, now)
			if from == nil || to == nil || !from.Equal(tc.from) || !to.Equal(tc.from.AddDate(0, 1, 0)) {
				t.Fatalf("window = (%v, %v), want month starting %v", from, to, tc.from)
			}
		})
	}
}

func TestInferTimeAxes(t *testing.T) {
	now := time.Date(2026, time.October, 20, 14, 30, 0, 0, time.UTC)
	month := func(y int, m time.Month) *time.Time {
		d := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
		return &d
	}
	next := func(d *time.Time) *time.Time { n := d.AddDate(0, 1, 0); return &n }
	same := func(a, b *time.Time) bool {
		if a == nil || b == nil {
			return a == b
		}
		return a.Equal(*b)
	}
	mar, jan := month(2026, time.March), month(2026, time.January)

	tests := []struct {
		name  string
		query string
		known *time.Time // start of the expected knowledge month, nil for none
		valid *time.Time // start of the expected valid-time month, nil for none
	}{
		{"know last month name", "what did we know last March", mar, nil},
		{"know in month", "what did we know in March", mar, nil},
		{"dutch know", "wat wisten we in maart", mar, nil},
		{"was known", "what was known in March", mar, nil},
		{"as of", "as of March, which port did the dashboard use", mar, nil},
		{"fronted adverbial", "In March, what did we know about the dashboard", mar, nil},
		{"two axes", "what did we know in March about the outage in January", mar, jan},
		{"about ties the time to the subject", "what did we know about the outage in March", nil, mar},
		{"dutch over", "wat wisten we over de storing in maart", nil, mar},
		{"no cue is valid time", "what happened in March", nil, mar},
		{"present tense is not a cue", "what do we know about the outage in March", nil, mar},
		{"cue without expression", "what did we know about the dashboard port", nil, nil},
		{"neither", "which port does the dashboard use", nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			axes := inferTimeAxes(tc.query, now)
			if !same(axes.KnownFrom, tc.known) || (tc.known != nil && !same(axes.KnownTo, next(tc.known))) {
				t.Errorf("known = (%v, %v), want month %v", axes.KnownFrom, axes.KnownTo, tc.known)
			}
			if !same(axes.From, tc.valid) || (tc.valid != nil && !same(axes.To, next(tc.valid))) {
				t.Errorf("valid = (%v, %v), want month %v", axes.From, axes.To, tc.valid)
			}
		})
	}
}

func TestInferTimeAxesLeavesRecallEvalQueriesAlone(t *testing.T) {
	file, err := os.Open("../../eval/recall_queries.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var item struct {
			ID    string `json:"id"`
			Query string `json:"query"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			t.Fatal(err)
		}
		if axes := inferTimeAxes(item.Query, now); axes != (timeAxes{}) {
			t.Errorf("eval query %s acquired inferred time bounds: %q", item.ID, item.Query)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}
