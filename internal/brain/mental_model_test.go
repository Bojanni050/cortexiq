// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"testing"
	"time"
)

func TestIsMentalModelStale(t *testing.T) {
	t.Run("never refreshed is stale", func(t *testing.T) {
		if !isMentalModelStale(nil) {
			t.Error("nil lastRefreshedAt should be stale")
		}
	})

	t.Run("just refreshed is not stale", func(t *testing.T) {
		now := time.Now()
		if isMentalModelStale(&now) {
			t.Error("recently refreshed should not be stale")
		}
	})

	t.Run("refreshed 1 hour ago is not stale", func(t *testing.T) {
		h := time.Now().Add(-1 * time.Hour)
		if isMentalModelStale(&h) {
			t.Error("1 hour old should not be stale")
		}
	})

	t.Run("refreshed 25 hours ago is stale", func(t *testing.T) {
		h := time.Now().Add(-25 * time.Hour)
		if !isMentalModelStale(&h) {
			t.Error("25 hours old should be stale")
		}
	})

	t.Run("refreshed exactly 24 hours ago is stale", func(t *testing.T) {
		h := time.Now().Add(-24 * time.Hour)
		if !isMentalModelStale(&h) {
			t.Error("exactly 24 hours should be stale (boundary is exclusive)")
		}
	})
}

func TestFormatMentalModelsForReflect(t *testing.T) {
	t.Run("empty list returns empty string", func(t *testing.T) {
		if got := formatMentalModelsForReflect(nil); got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("single model is formatted with name and content", func(t *testing.T) {
		models := []MentalModelSearchResult{
			{ID: 1, Name: "Kafka Migration", Content: "We migrated to Kafka in March."},
		}
		got := formatMentalModelsForReflect(models)
		if got == "" {
			t.Fatal("expected non-empty output")
		}
		if !contains(got, "Kafka Migration") {
			t.Error("output should contain model name")
		}
		if !contains(got, "We migrated to Kafka in March.") {
			t.Error("output should contain model content")
		}
	})

	t.Run("stale model is marked", func(t *testing.T) {
		models := []MentalModelSearchResult{
			{ID: 1, Name: "Old Model", Content: "Outdated.", IsStale: true},
		}
		got := formatMentalModelsForReflect(models)
		if !contains(got, "STALE") {
			t.Error("stale model should be marked with [STALE]")
		}
	})

	t.Run("fresh model is not marked stale", func(t *testing.T) {
		models := []MentalModelSearchResult{
			{ID: 1, Name: "Fresh Model", Content: "Up to date.", IsStale: false},
		}
		got := formatMentalModelsForReflect(models)
		if contains(got, "STALE") {
			t.Error("fresh model should not be marked stale")
		}
	})

	t.Run("multiple models are numbered", func(t *testing.T) {
		models := []MentalModelSearchResult{
			{ID: 1, Name: "First", Content: "Content 1"},
			{ID: 2, Name: "Second", Content: "Content 2"},
		}
		got := formatMentalModelsForReflect(models)
		if !contains(got, "1.") || !contains(got, "2.") {
			t.Error("models should be numbered")
		}
	})
}

func TestReflectOptionsMentalModels(t *testing.T) {
	t.Run("SkipMentalModels defaults to false", func(t *testing.T) {
		opts := ReflectOptions{}
		if opts.SkipMentalModels {
			t.Error("SkipMentalModels should default to false")
		}
	})

	t.Run("MentalModelTags defaults to nil", func(t *testing.T) {
		opts := ReflectOptions{}
		if opts.MentalModelTags != nil {
			t.Error("MentalModelTags should default to nil")
		}
	})
}

func TestMentalModelSearchResult(t *testing.T) {
	t.Run("zero value", func(t *testing.T) {
		r := MentalModelSearchResult{}
		if r.ID != 0 || r.Name != "" || r.Content != "" || r.Relevance != 0 {
			t.Error("zero value should have zero fields")
		}
		if r.IsStale {
			t.Error("zero value should not be stale")
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
