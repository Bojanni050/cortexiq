// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"strings"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/models"
)

// A question's filler must not make it look related to every belief in memory.
func TestMatchTermsDropsFiller(t *testing.T) {
	terms := matchTerms("Waarom werkt mijn MCP-koppeling niet meer sinds de Norton-update?")

	for _, wanted := range []string{"koppeling", "norton", "sinds", "update"} {
		if _, ok := terms[wanted]; !ok {
			t.Errorf("expected %q to survive, got %v", wanted, keys(terms))
		}
	}
	for _, dropped := range []string{"waarom", "werkt", "mijn", "niet", "meer"} {
		if _, ok := terms[dropped]; ok {
			t.Errorf("%q is filler and must be dropped, got %v", dropped, keys(terms))
		}
	}
}

// Short words carry no subject, whatever language they are in.
func TestMatchTermsDropsShortWords(t *testing.T) {
	terms := matchTerms("de api is nu weg")
	if len(terms) != 0 {
		t.Fatalf("expected no surviving terms, got %v", keys(terms))
	}
}

// A belief that rests on a fact recall just returned is about the same thing,
// even when it words it differently. That has to beat a mere word overlap.
func TestHypothesisScorePrefersSharedFacts(t *testing.T) {
	asked := matchTerms("why does the connection drop")
	recalled := map[int64]struct{}{42: {}}

	onTopic := models.Hypothesis{
		Content:       "The tunnel is torn down after fifteen minutes of idleness",
		SourceFactIDs: []int64{42},
	}
	wordOnly := models.Hypothesis{
		Content: "The connection between charts and CSS variables is loose",
	}

	if hypothesisScore(onTopic, asked, recalled) <= hypothesisScore(wordOnly, asked, recalled) {
		t.Fatalf("a shared fact must outrank a shared word: fact=%d word=%d",
			hypothesisScore(onTopic, asked, recalled), hypothesisScore(wordOnly, asked, recalled))
	}
}

// A hypothesis with no connection to the question scores zero and is left out
// rather than padding the prompt.
func TestHypothesisScoreIgnoresUnrelatedBeliefs(t *testing.T) {
	asked := matchTerms("how many migrations does the database have")
	unrelated := models.Hypothesis{Content: "Bo prefers dark mode in every editor"}

	if got := hypothesisScore(unrelated, asked, map[int64]struct{}{}); got != 0 {
		t.Fatalf("expected 0 for an unrelated belief, got %d", got)
	}
}

func TestHypothesisFactIDsDedupesAcrossGroups(t *testing.T) {
	confirmed := int64(9)
	h := models.Hypothesis{
		SourceFactIDs:   []int64{1, 2},
		SupportedBy:     []int64{2, 3},
		ConfirmedFactID: &confirmed,
	}

	got := hypothesisFactIDs(h)
	want := []int64{1, 2, 3, 9}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

// The prompt has to say how thin the evidence is, because that is the whole
// reason the belief is in a separate section.
func TestFormatHypothesesForReflectMarksWeakSupport(t *testing.T) {
	counter := int64(11)
	out := formatHypothesesForReflect([]models.Hypothesis{
		{
			Content:          "Norton TLS interception breaks the desktop MCP client",
			Status:           "proposed",
			Confidence:       0.4,
			SupportFacts:     3,
			SupportEpisodes:  1,
			VerificationPlan: "disable interception for the binary",
		},
		{
			Content:    "The tunnel is not the cause",
			Status:     "testing",
			Confidence: 0.5,
			CounterOf:  &counter,
		},
	})

	for _, want := range []string{
		"single conversation",
		"counter to hypothesis #11",
		"would be settled by: disable interception",
		"no supporting facts yet",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected prompt to contain %q, got:\n%s", want, out)
		}
	}
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
