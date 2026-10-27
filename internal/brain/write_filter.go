// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Bojanni050/cortexiq/internal/hygiene"
)

// Write filter modes. The filter runs in front of every write path (MCP, HTTP,
// CLI) so the store never fills with leaked secrets or machine noise in the
// first place — the same gate the social filter already is for greetings.
//
//   - "off"    (default): no filtering, every write is stored as before.
//   - "warn":  writes proceed, but the decision carries the reasons so the
//     caller can surface them (CLI prints a warning, MCP returns metadata).
//   - "strict": writes classified as reject (secret detected, or noise above
//     the reject threshold) are refused with ErrWriteFiltered.
const (
	WriteFilterOff    = "off"
	WriteFilterWarn   = "warn"
	WriteFilterStrict = "strict"
)

// ErrWriteFiltered means the write filter refused the content. The reason is
// in the error message; the caller reports it as skipped, not as a failure.
var ErrWriteFiltered = fmt.Errorf("brain: write refused by filter")

// WriteDecision is what the filter concluded about one write. Callers inspect
// it before persisting: Action says what to do, Reasons says why.
type WriteDecision struct {
	// Action is "store", "warn" or "reject".
	Action string `json:"action"`
	// Reasons lists every rule that fired (secret labels, noise reasons).
	Reasons []string `json:"reasons,omitempty"`
	// SecretLabels names the kinds of secret found (labels only, never values).
	SecretLabels []string `json:"secret_labels,omitempty"`
	// NoiseScore is the hygiene noise score of the content (0..1).
	NoiseScore float64 `json:"noise_score"`
	// Redacted is the content with secrets replaced by [REDACTED:label]
	// placeholders. In strict mode a rejected write is never stored, so this
	// matters mostly for warn mode: the caller may store the redacted form.
	Redacted string `json:"redacted,omitempty"`
}

// ignorePatterns are extra regex patterns the operator can configure (via
// CORTEXIQ_WRITE_IGNORE_PATTERNS, newline- or comma-separated). A write whose
// content matches any of them is rejected in strict mode and warned about in
// warn mode. They are matched with re.search semantics (case-insensitive).
var ignorePatterns []*regexp.Regexp

// SetWriteIgnorePatterns replaces the configured ignore patterns. It is called
// once at startup from the bootstrap; an empty slice disables the feature.
func SetWriteIgnorePatterns(patterns []string) {
	ignorePatterns = nil
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			// A broken pattern is a config error, not a runtime one: skip it
			// rather than crash the write path. The operator sees it in the
			// startup log via the bootstrap warning.
			continue
		}
		ignorePatterns = append(ignorePatterns, re)
	}
}

// WriteIgnorePatterns returns the currently active ignore patterns (for
// inspection / debugging).
func WriteIgnorePatterns() []*regexp.Regexp {
	return ignorePatterns
}

// classifyWrite scores content and decides what to do with it. mode is one of
// WriteFilterOff/Warn/Strict. importance is the caller-assigned importance.
//
// The decision is deterministic: no LLM, no embeddings, no network.
func classifyWrite(content string, importance float32, mode string) WriteDecision {
	if mode == WriteFilterOff || mode == "" {
		return WriteDecision{Action: "store"}
	}

	score := hygiene.ScoreContent(content, float64(importance), "")
	decision := WriteDecision{
		NoiseScore:   score.Value,
		Reasons:      score.Reasons,
		SecretLabels: score.SecretLabels,
	}

	// Custom ignore patterns fire independently of the hygiene scorer. An
	// operator-configured pattern is an explicit "this must not be stored", so
	// in strict mode it always rejects regardless of the noise score.
	ignoreMatch := false
	for _, re := range ignorePatterns {
		if re.MatchString(content) {
			ignoreMatch = true
			decision.Reasons = append(decision.Reasons, "ignore_pattern_match")
			if decision.NoiseScore < hygiene.ScoreNoisePattern {
				decision.NoiseScore = hygiene.ScoreNoisePattern
			}
			break
		}
	}
	if ignoreMatch && mode == WriteFilterStrict {
		decision.Action = "reject"
		return decision
	}

	// Secrets are always a reject in strict mode, a warn otherwise.
	if len(score.SecretLabels) > 0 {
		decision.Redacted = redactForStorage(content)
		if mode == WriteFilterStrict {
			decision.Action = "reject"
		} else {
			decision.Action = "warn"
		}
		return decision
	}

	// High noise (>= archive threshold) is a reject in strict mode.
	if decision.NoiseScore >= hygiene.ScoreTerminal {
		if mode == WriteFilterStrict {
			decision.Action = "reject"
		} else {
			decision.Action = "warn"
		}
		return decision
	}

	// Moderate noise: warn in warn mode, store in strict mode (strict only
	// rejects the clearly-bad writes: secrets and terminal/stack-trace output).
	if decision.NoiseScore >= hygiene.ScoreLowImportance && mode == WriteFilterWarn {
		decision.Action = "warn"
		return decision
	}

	decision.Action = "store"
	return decision
}

// redactForStorage replaces every secret match with a [REDACTED:label]
// placeholder so the stored content cannot leak the value it was flagged for.
func redactForStorage(content string) string {
	return hygiene.Redact(content)
}

// applyWriteDecision returns the error for a rejected write, or nil to proceed.
// In warn mode it returns nil (the write proceeds) but the caller should
// surface decision.Reasons.
func applyWriteDecision(decision WriteDecision) error {
	if decision.Action == "reject" {
		return fmt.Errorf("%w: %s", ErrWriteFiltered, strings.Join(decision.Reasons, ", "))
	}
	return nil
}
