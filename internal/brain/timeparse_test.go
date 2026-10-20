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
