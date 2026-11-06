package main

import (
	"strings"
	"testing"
)

// render() swallows template errors and returns an empty string, so a typo in a
// define name shows up as a tool with no description instead of a crash. These
// assertions are the only thing that catches that.
func TestHypothesisEventPromptsRender(t *testing.T) {
	desc := render("list_hypothesis_events_description")
	if strings.TrimSpace(desc) == "" {
		t.Fatal("list_hypothesis_events_description is empty")
	}
	for _, want := range []string{"consolidator", "evidence", "rejected"} {
		if !strings.Contains(desc, want) {
			t.Fatalf("description does not mention %q", want)
		}
	}

	if strings.TrimSpace(render("list_hypothesis_events_id")) == "" {
		t.Fatal("list_hypothesis_events_id is empty")
	}
}
