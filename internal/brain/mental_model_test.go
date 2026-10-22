// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"errors"
	"testing"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
)

func TestMentalModelStaleReason(t *testing.T) {
	now := time.Now()
	watermark := now.Add(-time.Hour)
	older := now.Add(-2 * time.Hour)

	cases := []struct {
		name       string
		refreshed  *time.Time
		watermark  *time.Time
		newest     time.Time
		newestSeen bool
		retracted  bool
		want       string
	}{
		{"never refreshed", nil, &watermark, now, true, false, "never_refreshed"},
		{"no watermark", &now, nil, now, true, false, "no_watermark"},
		{"new memory after watermark", &now, &watermark, now, true, false, "new_memories"},
		{"no new memory", &now, &watermark, older, true, false, ""},
		{"no memories at all", &now, &watermark, time.Time{}, false, false, ""},
		{"retracted grounding", &now, &watermark, older, true, true, "retracted_grounding"},
		{"new memory wins over retraction", &now, &watermark, now, true, true, "new_memories"},
		{"memory exactly at watermark is seen", &now, &watermark, watermark, true, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mentalModelStaleReason(tc.refreshed, tc.watermark, tc.newest, tc.newestSeen, tc.retracted)
			if got != tc.want {
				t.Fatalf("reason = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBasedOnFromSources(t *testing.T) {
	got := basedOnFromSources([]ReflectSource{
		{Type: "fact", ID: 3},
		{Type: "episode", ID: 9},
		{Type: "fact", ID: 1},
		{Type: "fact", ID: 3}, // duplicate, must collapse
		{Type: "goal", ID: 7},
		{Type: "unknown", ID: 99},
	})

	if want := []int64{1, 3}; !equalIDs(got.Facts, want) {
		t.Errorf("facts = %v, want %v", got.Facts, want)
	}
	if want := []int64{9}; !equalIDs(got.Episodes, want) {
		t.Errorf("episodes = %v, want %v", got.Episodes, want)
	}
	if want := []int64{7}; !equalIDs(got.Goals, want) {
		t.Errorf("goals = %v, want %v", got.Goals, want)
	}
	if len(got.Failures) != 0 {
		t.Errorf("failures = %v, want empty", got.Failures)
	}
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGroundedReflect(t *testing.T) {
	cases := []struct {
		name    string
		result  *ReflectResult
		wantErr bool
	}{
		{"nil result", nil, true},
		{"no sources", &ReflectResult{Answer: "something"}, true},
		{"empty answer", &ReflectResult{Sources: []ReflectSource{{Type: "fact", ID: 1}}}, true},
		{"grounded", &ReflectResult{
			Answer:  "something",
			Sources: []ReflectSource{{Type: "fact", ID: 1}},
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := groundedReflect(tc.result)
			if (err != nil) != tc.wantErr {
				t.Fatalf("groundedReflect() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, ErrMentalModelNotGrounded) {
				t.Fatalf("error = %v, want ErrMentalModelNotGrounded", err)
			}
		})
	}
}

func TestMentalModelAutoRefreshPaused(t *testing.T) {
	old := time.Now().Add(-2 * time.Hour)
	recent := time.Now()
	failed := time.Now().Add(-time.Hour)

	if mentalModelAutoRefreshPaused(models.MentalModel{}) {
		t.Error("no failure should not pause")
	}
	if mentalModelAutoRefreshPaused(models.MentalModel{
		LastRefreshedAt:     &old,
		LastRefreshFailedAt: &failed,
	}) != true {
		t.Error("a failure after the last success should pause")
	}
	if mentalModelAutoRefreshPaused(models.MentalModel{
		LastRefreshedAt:     &recent,
		LastRefreshFailedAt: &failed,
	}) {
		t.Error("a failure older than the stored content should not pause")
	}
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
			{ID: 1, Name: "Old Model", Content: "Outdated.", IsStale: true, StaleReason: "new_memories"},
		}
		got := formatMentalModelsForReflect(models)
		if !contains(got, "STALE") {
			t.Error("stale model should be marked with [STALE]")
		}
		if !contains(got, "new_memories") {
			t.Error("stale model should say why it is stale")
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
