// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"reflect"
	"testing"
)

// A question becomes the terms a regularity may be matched on. Short words and
// stop words are dropped: they match almost everything and would drag unrelated
// patterns into every answer.
func TestQueryTermsDropsStopAndShortWords(t *testing.T) {
	got := queryTerms("How does the CortexIQ deploy to production?")
	want := []string{"cortexiq", "deploy", "production"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("queryTerms = %v, want %v", got, want)
	}
}

// The list is capped so a long question does not turn into a full-text query
// with fifty branches.
func TestQueryTermsCapsTheList(t *testing.T) {
	got := queryTerms("alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima mike november oscar")
	if len(got) != 12 {
		t.Errorf("len(queryTerms) = %d, want 12", len(got))
	}
}

// A question with nothing usable returns no terms, which is what keeps a vague
// question from grounding on whatever happens to have a high confidence.
func TestQueryTermsStopsQuestionWithNothingToMatch(t *testing.T) {
	if got := queryTerms("what is it?"); len(got) != 0 {
		t.Errorf("queryTerms = %v, want none", got)
	}
}
