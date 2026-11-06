package reasoner

import (
	"strings"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/models"
)

func TestValidateCounterHypothesisRejectsRestatement(t *testing.T) {
	original := "The team has adopted TypeScript as their primary language"

	cases := []struct {
		name    string
		in      jsonCounterHypothesis
		wantErr string
	}{
		{"empty content", jsonCounterHypothesis{Content: "   "}, "empty"},
		{"exact restatement", jsonCounterHypothesis{Content: original}, "differ"},
		{"restatement with different case and padding", jsonCounterHypothesis{Content: "  " + strings.ToUpper(original) + " "}, "differ"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateCounterHypothesis(tc.in, original)
			if err == nil {
				t.Fatalf("want an error, got %+v", got)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not mention %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestValidateCounterHypothesisClampsConfidence(t *testing.T) {
	original := "The deploy pipeline runs on Fridays"
	cases := map[string]float32{"below floor": 0, "above ceiling": 1, "in range": 0.42}
	want := map[string]float32{"below floor": 0.05, "above ceiling": 0.95, "in range": 0.42}

	for name, conf := range cases {
		got, err := validateCounterHypothesis(jsonCounterHypothesis{
			Content:          "The deploy pipeline never runs on Fridays",
			VerificationPlan: "Check the deploy log for the day of week",
			Confidence:       conf,
		}, original)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.Confidence != want[name] {
			t.Errorf("%s: confidence %v, want %v", name, got.Confidence, want[name])
		}
	}
}

// The prompt is the whole feature: if the instruction to disagree and to produce
// a discriminating test goes missing, the model goes back to agreeing politely.
func TestCounterPromptAsksForADiscriminatingTest(t *testing.T) {
	h := models.Hypothesis{
		Content:          "Latency comes from the database",
		VerificationPlan: "Profile a request end to end",
	}
	evidence := []models.Fact{{ID: 7, Content: "p99 rose after the schema change"}}

	prompt := counterPrompt(h, evidence)

	for _, want := range []string{
		h.Content,
		h.VerificationPlan,
		"p99 rose after the schema change",
		"argue the opposite case",
		"INSTEAD",
		"DISTINGUISH",
		"genuine alternative",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}
}

func TestCounterPromptWithoutEvidenceSaysSo(t *testing.T) {
	prompt := counterPrompt(models.Hypothesis{Content: "x"}, nil)
	if !strings.Contains(prompt, "No stored facts were given.") {
		t.Fatalf("prompt should state that no facts were given:\n%s", prompt)
	}
}
