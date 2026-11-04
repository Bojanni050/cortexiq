// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"errors"
	"strings"
	"testing"
)

func TestClassifyWrite_Off(t *testing.T) {
	// In off mode the filter is a no-op: every write stores, even one full of
	// secrets and terminal output.
	d := classifyWrite("password = hunter2secret\nCollecting numpy", 0.5, WriteFilterOff)
	if d.Action != "store" {
		t.Errorf("off mode: got action %q, want store", d.Action)
	}
	if d.NoiseScore != 0 {
		t.Errorf("off mode: noise score should be 0, got %v", d.NoiseScore)
	}
}

func TestClassifyWrite_EmptyMode(t *testing.T) {
	// An empty mode string is the same as off (the default).
	d := classifyWrite("password = hunter2secret", 0.5, "")
	if d.Action != "store" {
		t.Errorf("empty mode: got action %q, want store", d.Action)
	}
}

func TestClassifyWrite_WarnSecret(t *testing.T) {
	d := classifyWrite("my key is ghp_abcdefghijklmnopqrstuvwxyz1234 ok", 0.5, WriteFilterWarn)
	if d.Action != "warn" {
		t.Errorf("warn mode secret: got %q, want warn", d.Action)
	}
	if len(d.SecretLabels) == 0 {
		t.Error("secret labels should be populated")
	}
	if d.Redacted == "" {
		t.Error("redacted form should be populated for secrets")
	}
	if strings.Contains(d.Redacted, "ghp_abcdefghijklmnopqrstuvwxyz1234") {
		t.Error("redacted form still contains the secret")
	}
	if !strings.Contains(d.Redacted, "[REDACTED:github_token]") {
		t.Errorf("redacted form should contain the marker: %q", d.Redacted)
	}
}

func TestClassifyWrite_StrictSecret(t *testing.T) {
	d := classifyWrite("aws key AKIAIOSFODNN7EXAMPLE here", 0.5, WriteFilterStrict)
	if d.Action != "reject" {
		t.Errorf("strict mode secret: got %q, want reject", d.Action)
	}
	err := applyWriteDecision(d)
	if !errors.Is(err, ErrWriteFiltered) {
		t.Errorf("applyWriteDecision should return ErrWriteFiltered, got %v", err)
	}
}

func TestClassifyWrite_WarnTerminalOutput(t *testing.T) {
	d := classifyWrite("Collecting numpy\n  Downloading numpy-1.26.0.tar.gz", 0.5, WriteFilterWarn)
	if d.Action != "warn" {
		t.Errorf("warn mode terminal: got %q, want warn", d.Action)
	}
}

func TestClassifyWrite_StrictTerminalOutput(t *testing.T) {
	d := classifyWrite("Collecting numpy\n  Downloading numpy-1.26.0.tar.gz", 0.5, WriteFilterStrict)
	if d.Action != "reject" {
		t.Errorf("strict mode terminal: got %q, want reject", d.Action)
	}
}

func TestClassifyWrite_StrictStackTrace(t *testing.T) {
	d := classifyWrite("Traceback (most recent call last):\n  File \"app.py\", line 42, in <module>", 0.5, WriteFilterStrict)
	if d.Action != "reject" {
		t.Errorf("strict mode stack trace: got %q, want reject", d.Action)
	}
}

func TestClassifyWrite_StrictModerateNoiseStores(t *testing.T) {
	// Strict only rejects the clearly-bad writes (secrets, terminal output,
	// stack traces). Moderate noise (low importance, trivial keyword) still
	// stores in strict mode: rejecting those would silently drop real
	// memories.
	d := classifyWrite("ok", 0.1, WriteFilterStrict)
	if d.Action != "store" {
		t.Errorf("strict mode moderate noise: got %q, want store", d.Action)
	}
}

func TestClassifyWrite_WarnModerateNoise(t *testing.T) {
	d := classifyWrite("ok", 0.1, WriteFilterWarn)
	if d.Action != "warn" {
		t.Errorf("warn mode moderate noise: got %q, want warn", d.Action)
	}
}

func TestClassifyWrite_CleanContentStores(t *testing.T) {
	content := "Bo prefers dark mode interfaces and always uses vim keybindings in every editor."
	for _, mode := range []string{WriteFilterOff, WriteFilterWarn, WriteFilterStrict} {
		d := classifyWrite(content, 0.8, mode)
		if d.Action != "store" {
			t.Errorf("mode %s: clean content got %q, want store (score %v, reasons %v)", mode, d.Action, d.NoiseScore, d.Reasons)
		}
	}
}

func TestClassifyWrite_IgnorePatterns(t *testing.T) {
	SetWriteIgnorePatterns([]string{`(?i)heartbeat ping \d+`})
	defer SetWriteIgnorePatterns(nil)

	d := classifyWrite("heartbeat ping 12345 received", 0.5, WriteFilterStrict)
	if d.Action != "reject" {
		t.Errorf("ignore pattern should reject in strict mode, got %q", d.Action)
	}
	hasReason := false
	for _, r := range d.Reasons {
		if r == "ignore_pattern_match" {
			hasReason = true
		}
	}
	if !hasReason {
		t.Errorf("reasons %v missing ignore_pattern_match", d.Reasons)
	}
}

func TestClassifyWrite_IgnorePatternsWarn(t *testing.T) {
	SetWriteIgnorePatterns([]string{`(?i)heartbeat ping \d+`})
	defer SetWriteIgnorePatterns(nil)

	d := classifyWrite("heartbeat ping 12345 received", 0.5, WriteFilterWarn)
	if d.Action != "warn" {
		t.Errorf("ignore pattern should warn in warn mode, got %q", d.Action)
	}
}

func TestClassifyWrite_IgnorePatternsOff(t *testing.T) {
	SetWriteIgnorePatterns([]string{`(?i)heartbeat ping \d+`})
	defer SetWriteIgnorePatterns(nil)

	d := classifyWrite("heartbeat ping 12345 received", 0.5, WriteFilterOff)
	if d.Action != "store" {
		t.Errorf("off mode: ignore patterns should not fire, got %q", d.Action)
	}
}

func TestSetWriteIgnorePatterns_BrokenPatternSkipped(t *testing.T) {
	// A broken regex is a config error, not a runtime one: it is skipped
	// rather than panicking the write path.
	SetWriteIgnorePatterns([]string{"[unclosed", "valid_pattern"})
	defer SetWriteIgnorePatterns(nil)

	if got := len(WriteIgnorePatterns()); got != 1 {
		t.Errorf("got %d patterns, want 1 (broken one skipped)", got)
	}
}

func TestApplyWriteDecision(t *testing.T) {
	tests := []struct {
		name     string
		decision WriteDecision
		wantErr  bool
	}{
		{"store proceeds", WriteDecision{Action: "store"}, false},
		{"warn proceeds", WriteDecision{Action: "warn"}, false},
		{"reject fails", WriteDecision{Action: "reject", Reasons: []string{"secret_detected"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := applyWriteDecision(tt.decision)
			if tt.wantErr && !errors.Is(err, ErrWriteFiltered) {
				t.Errorf("got %v, want ErrWriteFiltered", err)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("got %v, want nil", err)
			}
		})
	}
}

func TestRedactForStorage(t *testing.T) {
	content := "config: api_key = sk-projabcdefghijklmnopqrstuvwxyz end"
	out := redactForStorage(content)
	if strings.Contains(out, "sk-projabcdefghijklmnopqrstuvwxyz") {
		t.Errorf("redacted output still contains the secret: %q", out)
	}
	if !strings.Contains(out, "[REDACTED:") {
		t.Errorf("redacted output should contain a marker: %q", out)
	}
	// Non-secret parts are preserved.
	if !strings.Contains(out, "config:") || !strings.Contains(out, "end") {
		t.Errorf("redacted output lost non-secret parts: %q", out)
	}
}
