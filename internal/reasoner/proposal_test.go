package reasoner

import (
	"strings"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/models"
)

func TestValidateProposedHypothesesDropsWhatCannotBeTested(t *testing.T) {
	raw := []jsonProposedHypothesis{
		{Content: "", VerificationPlan: "check the logs"},
		{Content: "De API is traag", VerificationPlan: "  "},
		{Content: "De API is traag door inefficiënte queries", VerificationPlan: "meet de responstijd", Confidence: 0.9},
	}

	got := validateProposedHypotheses(raw, nil, 2)

	if len(got) != 1 {
		t.Fatalf("want 1 usable proposal, got %d: %+v", len(got), got)
	}
	if got[0].Confidence != 0.5 {
		t.Fatalf("confidence should be capped at 0.5, got %v", got[0].Confidence)
	}
	if got[0].VerificationPlan != "meet de responstijd" {
		t.Fatalf("verification plan lost: %q", got[0].VerificationPlan)
	}
}

func TestValidateProposedHypothesesRespectsMax(t *testing.T) {
	raw := []jsonProposedHypothesis{
		{Content: "De deploy faalt op vrijdag", VerificationPlan: "a"},
		{Content: "Het geheugen groeit met de tijd", VerificationPlan: "b"},
		{Content: "De retries veroorzaken de vertraging", VerificationPlan: "c"},
	}

	if got := validateProposedHypotheses(raw, nil, 2); len(got) != 2 {
		t.Fatalf("want 2 proposals, got %d", len(got))
	}
	if got := validateProposedHypotheses(raw, nil, 0); len(got) != 0 {
		t.Fatalf("max 0 should propose nothing, got %d", len(got))
	}
}

// The failure this guards against is the log filling with the same doubt in
// different words, which reads like new evidence while being none.
func TestValidateProposedHypothesesSkipsRestatements(t *testing.T) {
	existing := []string{
		"De dashboard-API is traag door inefficiënte queries.",
		"Het team deployt op vrijdag.",
	}
	raw := []jsonProposedHypothesis{
		{Content: "de dashboard api is traag vanwege inefficiënte queries", VerificationPlan: "meet"},
		{Content: "Het team deployt op vrijdag", VerificationPlan: "kijk in de logs"},
		{Content: "De caching-laag maskeert de traagheid", VerificationPlan: "zet caching uit"},
	}

	got := validateProposedHypotheses(raw, existing, 5)

	if len(got) != 1 {
		t.Fatalf("want only the new belief, got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Content, "caching-laag") {
		t.Fatalf("kept the wrong proposal: %q", got[0].Content)
	}
}

// Two proposals in the same batch that say the same thing are as bad as one
// restating an existing hypothesis.
func TestValidateProposedHypothesesDedupesWithinTheBatch(t *testing.T) {
	raw := []jsonProposedHypothesis{
		{Content: "De traagheid komt door de database", VerificationPlan: "a"},
		{Content: "De traagheid komt door de database", VerificationPlan: "b"},
	}

	if got := validateProposedHypotheses(raw, nil, 5); len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
}

func TestProposalParseAcceptsBothShapes(t *testing.T) {
	wrapped := `{"hypotheses": [{"content": "a", "verification_plan": "b"}]}`
	if got, err := proposalParse(wrapped); err != nil || len(got) != 1 {
		t.Fatalf("object shape: %v %d", err, len(got))
	}

	bare := `[{"content": "a", "verification_plan": "b"}]`
	if got, err := proposalParse(bare); err != nil || len(got) != 1 {
		t.Fatalf("array shape: %v %d", err, len(got))
	}

	empty := `{"hypotheses": []}`
	if got, err := proposalParse(empty); err != nil || len(got) != 0 {
		t.Fatalf("empty list: %v %d", err, len(got))
	}
}

func TestProposalPromptNamesTheFactsAndTheTrap(t *testing.T) {
	facts := []models.Fact{{ID: 12, Content: "De dashboard-API geeft timeouts"}, {ID: 15, Content: "De database is traag"}}
	prompt := proposalPrompt(facts, []string{"Het team deployt op vrijdag"}, 2)

	for _, want := range []string{"#12", "#15", "Het team deployt op vrijdag", "at most 2", "restates a fact"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt does not mention %q", want)
		}
	}
}
