// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestFormatMemoriesForReflect(t *testing.T) {
	tests := []struct {
		name    string
		results []RecallResult
		want    []string // substrings that must appear
		notWant []string // substrings that must not appear
	}{
		{
			name:    "empty results",
			results: nil,
			want:    nil,
		},
		{
			name: "fact with confidence",
			results: []RecallResult{
				{Type: "fact", Content: "The queue is kafka", Confidence: 0.9, Score: 0.8},
			},
			want: []string{"[fact (confidence 0.90)]", "The queue is kafka", "1."},
		},
		{
			name: "episode without confidence label",
			results: []RecallResult{
				{Type: "episode", Content: "User said hello", Score: 0.5},
			},
			want:    []string{"[episode]", "User said hello"},
			notWant: []string{"confidence"},
		},
		{
			name: "fact with occurred_at",
			results: []RecallResult{
				{Type: "episode", Content: "Deployed v2", OccurredAt: "2025-03-15T10:00:00Z", Score: 0.7},
			},
			want: []string{"(occurred: 2025-03-15T10:00:00Z)"},
		},
		{
			name: "fact with valid_from",
			results: []RecallResult{
				{Type: "fact", Content: "Uses Redis", ValidFrom: "2025-01-01", Confidence: 0.8, Score: 0.9},
			},
			want: []string{"(valid from: 2025-01-01)"},
		},
		{
			name: "multiple results are numbered",
			results: []RecallResult{
				{Type: "fact", Content: "First fact", Confidence: 0.8, Score: 0.9},
				{Type: "fact", Content: "Second fact", Confidence: 0.7, Score: 0.8},
				{Type: "episode", Content: "Third episode", Score: 0.6},
			},
			want: []string{"1.", "2.", "3.", "First fact", "Second fact", "Third episode"},
		},
		{
			name: "failure type",
			results: []RecallResult{
				{Type: "failure", Content: "Tried X, failed because Y", Score: 0.5},
			},
			want: []string{"[failure]", "Tried X, failed because Y"},
		},
		{
			name: "goal type",
			results: []RecallResult{
				{Type: "goal", Content: "Migrate to k8s", Score: 0.5},
			},
			want: []string{"[goal]", "Migrate to k8s"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMemoriesForReflect(tt.results)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("formatMemoriesForReflect() missing %q in:\n%s", want, got)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(got, notWant) {
					t.Errorf("formatMemoriesForReflect() should not contain %q in:\n%s", notWant, got)
				}
			}
		})
	}
}

func TestReflectResultFields(t *testing.T) {
	result := ReflectResult{
		Answer: "The queue is kafka.",
		Sources: []ReflectSource{
			{Type: "fact", ID: 42, Content: "the queue is kafka", Score: 0.9},
			{Type: "episode", ID: 7, Content: "we switched to kafka last month", Score: 0.7},
		},
		RecallCount: 2,
		DurationMs:  150,
	}

	if result.Answer != "The queue is kafka." {
		t.Errorf("Answer = %q, want %q", result.Answer, "The queue is kafka.")
	}
	if len(result.Sources) != 2 {
		t.Errorf("len(Sources) = %d, want 2", len(result.Sources))
	}
	if result.Sources[0].Type != "fact" || result.Sources[0].ID != 42 {
		t.Errorf("Sources[0] = %+v, want fact 42", result.Sources[0])
	}
	if result.RecallCount != 2 {
		t.Errorf("RecallCount = %d, want 2", result.RecallCount)
	}
	if result.DurationMs != 150 {
		t.Errorf("DurationMs = %d, want 150", result.DurationMs)
	}
}

func TestReflectOptionsDefaults(t *testing.T) {
	now := time.Now()
	opts := ReflectOptions{
		Namespaces: []string{"/projects/myapp"},
		Limit:      20,
		Context:    "Working on the migration",
		AsOf:       &now,
	}

	if len(opts.Namespaces) != 1 || opts.Namespaces[0] != "/projects/myapp" {
		t.Errorf("Namespaces = %v, want [/projects/myapp]", opts.Namespaces)
	}
	if opts.Limit != 20 {
		t.Errorf("Limit = %d, want 20", opts.Limit)
	}
	if opts.Context != "Working on the migration" {
		t.Errorf("Context = %q, want %q", opts.Context, "Working on the migration")
	}
	if opts.AsOf == nil {
		t.Error("AsOf should not be nil")
	}
}

// TestReflectEmptyQuery verifies that an empty query is rejected before any
// recall or LLM call happens.
func TestReflectEmptyQuery(t *testing.T) {
	b := &Brain{}
	_, err := b.Reflect(context.Background(), "", ReflectOptions{})
	if err == nil {
		t.Fatal("Reflect with empty query should return an error")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error = %q, want it to mention 'empty'", err.Error())
	}

	_, err = b.Reflect(context.Background(), "   ", ReflectOptions{})
	if err == nil {
		t.Fatal("Reflect with whitespace-only query should return an error")
	}
}

func TestReflectSourceFields(t *testing.T) {
	src := ReflectSource{
		Type:    "fact",
		ID:      123,
		Content: "the database is postgres",
		Score:   0.95,
	}
	if src.Type != "fact" {
		t.Errorf("Type = %q, want %q", src.Type, "fact")
	}
	if src.ID != 123 {
		t.Errorf("ID = %d, want 123", src.ID)
	}
	if src.Content != "the database is postgres" {
		t.Errorf("Content = %q, want %q", src.Content, "the database is postgres")
	}
	if src.Score != 0.95 {
		t.Errorf("Score = %f, want 0.95", src.Score)
	}
}
