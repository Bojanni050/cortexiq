// Modifications copyright (c) 2026 Bojan van der Heide
package hygiene

import (
	"strings"
	"testing"
)

func TestScoreContent_Empty(t *testing.T) {
	s := ScoreContent("", 0.5, "")
	if s.Value != ScoreEmpty {
		t.Errorf("empty content: got %v, want %v", s.Value, ScoreEmpty)
	}
	if len(s.Reasons) != 1 || s.Reasons[0] != "empty_content" {
		t.Errorf("empty content reasons: got %v", s.Reasons)
	}

	s = ScoreContent("   \n\t  ", 0.5, "")
	if s.Value != ScoreEmpty {
		t.Errorf("whitespace-only content: got %v, want %v", s.Value, ScoreEmpty)
	}
}

func TestScoreContent_Secret(t *testing.T) {
	tests := []struct {
		name    string
		content string
		label   string
	}{
		{"github token", "my key is ghp_abcdefghijklmnopqrstuvwxyz1234", "github_token"},
		{"aws key", "AKIAIOSFODNN7EXAMPLE", "aws_access_key"},
		{"openai key", "sk-projabcdefghijklmnopqrstuvwxyz", "openai_key"},
		{"slack token", "xoxb-1234567890-abcdefghijkl", "slack_token"},
		{"google api key", "AIza" + strings.Repeat("a", 35), "google_api_key"},
		{"private key block", "-----BEGIN RSA PRIVATE KEY-----\nMIIE...", "private_key_block"},
		{"password assignment", "password = hunter2secret", "secret_assignment"},
		{"postgres conn string", "postgres://user:secretpw@db.example.com:5432/app", "connection_string_credentials"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := ScoreContent(tt.content, 0.5, "")
			if s.Value != ScoreSecret {
				t.Errorf("got score %v, want %v", s.Value, ScoreSecret)
			}
			found := false
			for _, l := range s.SecretLabels {
				if l == tt.label {
					found = true
				}
			}
			if !found {
				t.Errorf("secret label %q not in %v", tt.label, s.SecretLabels)
			}
			if got := SuggestedAction(s); got != ActionFlag {
				t.Errorf("suggested action = %q, want %q", got, ActionFlag)
			}
		})
	}
}

func TestScoreContent_TerminalOutput(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"npm collecting", "Collecting numpy\n  Downloading numpy-1.26.0.tar.gz"},
		{"npm warn", "npm warn deprecated foo@1.0.0: this is deprecated"},
		{"ls -la", "-rw-r--r-- 1 user group 1234 Oct 27 10:00 file.txt\ntotal 48"},
		{"go downloading", "go: downloading github.com/some/module v1.2.3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := ScoreContent(tt.content, 0.5, "")
			if s.Value < ScoreTerminal {
				t.Errorf("got %v, want >= %v", s.Value, ScoreTerminal)
			}
			hasReason := false
			for _, r := range s.Reasons {
				if r == "terminal_output" {
					hasReason = true
				}
			}
			if !hasReason {
				t.Errorf("reasons %v missing terminal_output", s.Reasons)
			}
		})
	}
}

func TestScoreContent_StackTrace(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"python traceback", "Traceback (most recent call last):\n  File \"app.py\", line 42, in <module>\n    foo()"},
		{"go panic", "panic: runtime error: index out of range\n\ngoroutine 1 [running]:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := ScoreContent(tt.content, 0.5, "")
			if s.Value < ScoreStackTrace {
				t.Errorf("got %v, want >= %v", s.Value, ScoreStackTrace)
			}
		})
	}
}

func TestScoreContent_TrivialKeyword(t *testing.T) {
	tests := []string{"ok", "OK", "dank", "done", "yep", "prima", "goed"}
	for _, content := range tests {
		t.Run(content, func(t *testing.T) {
			s := ScoreContent(content, 0.5, "")
			if s.Value < ScoreTrivial {
				t.Errorf("got %v, want >= %v for %q", s.Value, ScoreTrivial, content)
			}
		})
	}
}

func TestScoreContent_NoisySource(t *testing.T) {
	for _, source := range []string{"heartbeat", "cron", "debug", "terminal", "ping"} {
		t.Run(source, func(t *testing.T) {
			s := ScoreContent("some real content here that is long enough", 0.5, source)
			if s.Value < ScoreNoisySource {
				t.Errorf("got %v, want >= %v for source %q", s.Value, ScoreNoisySource, source)
			}
		})
	}
}

func TestScoreContent_LikelyDump(t *testing.T) {
	// 40 lines of 50 chars = 2000 chars, no sentence-ending punctuation.
	// Newlines do not count as sentence breaks.
	var lines []string
	for i := 0; i < 40; i++ {
		lines = append(lines, strings.Repeat("x", 50))
	}
	content := strings.Join(lines, "\n")
	s := ScoreContent(content, 0.5, "")
	if s.Value < ScoreLikelyDump {
		t.Errorf("got %v, want >= %v", s.Value, ScoreLikelyDump)
	}
}

func TestScoreContent_LowImportance(t *testing.T) {
	s := ScoreContent("a real memory with actual content that is long enough to not be trivial", 0.1, "")
	if s.Value < ScoreLowImportance {
		t.Errorf("got %v, want >= %v", s.Value, ScoreLowImportance)
	}
}

func TestScoreContent_ValueClamp(t *testing.T) {
	// A memory that looks like a dump but contains a value keyword gets
	// clamped down.
	content := "This is a very long memory that goes on and on without any punctuation whatsoever " +
		"and we keep writing more and more text here to make it look like a dump but actually " +
		"it is a decision about the architecture of the system and how we prefer to structure it."
	s := ScoreContent(content, 0.5, "")
	if s.Value > ScoreValueClamp {
		t.Errorf("value keyword should clamp score to <= %v, got %v", ScoreValueClamp, s.Value)
	}
}

func TestScoreContent_ValueClampSkippedForSecret(t *testing.T) {
	// A secret outranks usefulness: the clamp must not apply.
	content := "password = hunter2secret and this is a decision about our architecture preferences"
	s := ScoreContent(content, 0.5, "")
	if s.Value != ScoreSecret {
		t.Errorf("secret should keep score at %v, got %v", ScoreSecret, s.Value)
	}
}

func TestScoreContent_CleanMemory(t *testing.T) {
	content := "Bo prefers dark mode interfaces and always uses vim keybindings in every editor."
	s := ScoreContent(content, 0.8, "")
	if s.Value > 0.1 {
		t.Errorf("clean memory should score low, got %v (reasons: %v)", s.Value, s.Reasons)
	}
	if got := SuggestedAction(s); got != ActionKeep {
		t.Errorf("suggested action = %q, want %q", got, ActionKeep)
	}
}

func TestSuggestedAction(t *testing.T) {
	tests := []struct {
		name  string
		score Score
		want  string
	}{
		{"secret flags", Score{Value: 0.9, SecretLabels: []string{"github_token"}}, ActionFlag},
		{"high score archives", Score{Value: 0.85}, ActionArchive},
		{"mid score reviews", Score{Value: 0.6}, ActionReview},
		{"low score keeps", Score{Value: 0.1}, ActionKeep},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SuggestedAction(tt.score); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPreview_RedactsSecrets(t *testing.T) {
	content := "my github token is ghp_abcdefghijklmnopqrstuvwxyz1234 and more text"
	p := Preview(content, 200)
	if strings.Contains(p, "ghp_abcdefghijklmnopqrstuvwxyz1234") {
		t.Errorf("preview leaked the secret: %q", p)
	}
	if !strings.Contains(p, "[REDACTED:github_token]") {
		t.Errorf("preview should contain the redaction marker: %q", p)
	}
}

func TestPreview_Truncates(t *testing.T) {
	content := strings.Repeat("a", 500)
	p := Preview(content, 100)
	if len([]rune(p)) != 101 { // 100 + ellipsis
		t.Errorf("got %d runes, want 101", len([]rune(p)))
	}
	if !strings.HasSuffix(p, "…") {
		t.Errorf("preview should end with ellipsis: %q", p[len(p)-10:])
	}
}

func TestPreview_ShortContentUnchanged(t *testing.T) {
	content := "short memory"
	p := Preview(content, 200)
	if p != content {
		t.Errorf("got %q, want %q", p, content)
	}
}

func TestScoreContent_NotAdditive(t *testing.T) {
	// Multiple weak signals must not compound into a false positive. Each rule
	// raises the score to at least its own value, but two 0.5 signals stay at
	// 0.5, not 1.0.
	s := ScoreContent("ok", 0.1, "heartbeat")
	if s.Value > ScoreTrivial {
		t.Errorf("score should not compound: got %v, want <= %v", s.Value, ScoreTrivial)
	}
}
