package reasoner

import "testing"

// The grounding gate refuses a summary that is (nearly) a literal copy of a source
// text, and nothing else. It used to refuse anything above 80 percent word overlap,
// which also dropped correct, self-contained sentences.
func TestValidateFactGrounding(t *testing.T) {
	source := "CortexIQ 0.1.88 draait op strato-vps en filtert op occurred_at met from en to, terwijl Hindsight temporal_window alleen rankt"
	cases := []struct {
		name    string
		summary string
		wantErr bool
	}{
		{"exact copy", source, true},
		{"copy inside a longer text", "filtert op occurred_at met from en to", true},
		// The overlap is a bag of words, so the same words in another order still count
		// as a copy: reordering is not a rewrite.
		{"same words in another order", "Hindsight temporal_window rankt alleen, terwijl CortexIQ 0.1.88 op strato-vps op occurred_at filtert met from en to", true},
		// 9 of 10 words occur in the source: refused at the old 0.80, allowed now.
		{"90 percent overlap", "cortexiq filtert occurred hindsight rankt alleen temporal window terwijl spike", false},
		{"rewrite in English", "CortexIQ 0.1.88 on strato-vps filters on occurred_at with from and to, while Hindsight temporal_window only ranks", false},
		{"empty summary", "", false},
	}
	for _, c := range cases {
		err := validateFactGrounding(&StructuredFact{Summary: c.summary}, []string{source})
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err = %v, want error %v", c.name, err, c.wantErr)
		}
	}
}
