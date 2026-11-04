// Modifications copyright (c) 2026 Bojan van der Heide
// Package noise scores memory content for noise: terminal output, stack
// traces, heartbeat pings, trivial acknowledgements and leaked secrets.
//
// It is the detector behind the write filter in internal/brain: the filter
// asks how noisy a write is and, when a secret is found, stores the redacted
// form instead. The scorer is deterministic — regex and arithmetic only, no
// LLM, no embeddings, no network — so it runs anywhere, costs nothing, and
// gives the same answer twice. It never touches storage: deciding what to do
// with a write belongs to the caller.
package noise

import (
	"regexp"
	"strings"
)

// Score is the noise score of one memory, 0..1. Higher means more likely noise.
type Score struct {
	// Value is in 0..1.
	Value float64 `json:"value"`
	// Reasons lists every rule that fired, e.g. "secret_detected:github_token",
	// "terminal_output", "trivial_keyword".
	Reasons []string `json:"reasons"`
	// SecretLabels names the kinds of secret found (labels only, never values).
	SecretLabels []string `json:"secret_labels,omitempty"`
}

// secretPattern pairs a label with the regex that detects it. The label is
// what ends up in the audit log; the matched value is never recorded.
type secretPattern struct {
	label string
	re    *regexp.Regexp
}

// secretPatterns covers the ten labelled secret shapes a memory must never
// silently keep: API key prefixes, cloud credentials, tokens, private keys
// and connection strings with embedded passwords.
var secretPatterns = []secretPattern{
	{"aws_access_key", regexp.MustCompile(`\b(AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{"github_token", regexp.MustCompile(`\b(ghp|gho|ghu|ghs|ghr|github_pat)_[A-Za-z0-9_]{20,}\b`)},
	{"slack_token", regexp.MustCompile(`\b(xox[baprs]-[A-Za-z0-9-]{10,})\b`)},
	{"google_api_key", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)},
	{"stripe_key", regexp.MustCompile(`\b[sr]k_(live|test)_[0-9A-Za-z]{16,}\b`)},
	{"openai_key", regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}\b`)},
	{"anthropic_key", regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}\b`)},
	{"jwt", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)},
	{"private_key_block", regexp.MustCompile(`-----BEGIN (RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY-----`)},
	{"secret_assignment", regexp.MustCompile(`(?i)\b(password|passwd|passphrase|secret|api[_-]?key|apikey|access[_-]?token|auth[_-]?token|client[_-]?secret)\b\s*[:=]\s*[^\s,;]{6,}`)},
	{"connection_string_credentials", regexp.MustCompile(`(?i)\b(postgres|postgresql|mysql|mongodb(?:\+srv)?|redis|amqp)://[^:/\s]+:[^@\s]+@`)},
}

// terminalPatterns mark content that is machine output pasted into a
// conversation rather than a memory a human wrote.
var terminalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^Collecting\s+\S+`),
	regexp.MustCompile(`(?m)^\s*Downloading\s+\S+`),
	regexp.MustCompile(`(?m)^\s*Installing collected packages:`),
	regexp.MustCompile(`(?i)\bnpm warn\b`),
	regexp.MustCompile(`(?m)^\s*drwx`),
	regexp.MustCompile(`(?m)^-rw-`),
	regexp.MustCompile(`(?m)^total \d+$`),
	regexp.MustCompile(`(?m)^\s*\[notice\]`),
	regexp.MustCompile(`(?i)\bgo: downloading\b`),
	regexp.MustCompile(`(?i)\bpip install\b.*\bSuccessfully installed\b`),
}

// stackTracePatterns mark a Python or Go traceback.
var stackTracePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^Traceback \(most recent call last\):`),
	regexp.MustCompile(`(?m)^\s+File "[^"]+", line \d+, in `),
	regexp.MustCompile(`(?m)^goroutine \d+ \[`),
	regexp.MustCompile(`(?m)^panic: `),
	regexp.MustCompile(`(?m)^\s+0x[0-9a-fA-F]+\s+\+0x[0-9a-fA-F]+`),
}

// noisePatterns is the catch-all for known noise shapes that are not terminal
// output or a stack trace: heartbeat pings, cron lines, debug markers.
var noisePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^\s*(heartbeat|ping|pong)\s*$`),
	regexp.MustCompile(`(?i)^\s*\[?debug\]?\s*$`),
	regexp.MustCompile(`(?i)^\s*cron\b.*\b(run|job|started|finished)\b`),
	regexp.MustCompile(`(?i)^\s*\{"level":\s*"(debug|trace)"`),
	regexp.MustCompile(`(?i)^\s*DEBUG\s+\S+`),
	regexp.MustCompile(`(?i)^\s*INFO\s+\S+\s+\d{4}-\d{2}-\d{2}`),
	regexp.MustCompile(`(?i)^\s*WARN(ING)?\s+\S+\s+\d{4}-\d{2}-\d{2}`),
	regexp.MustCompile(`(?i)^\s*ERROR\s+\S+\s+\d{4}-\d{2}-\d{2}`),
}

// trivialKeywords are one-word acknowledgements that carry no information.
// Matched case-insensitively as the whole content (after trimming).
var trivialKeywords = map[string]struct{}{
	"ok": {}, "oke": {}, "okay": {}, "oké": {}, "k": {}, "kk": {},
	"ja": {}, "nee": {}, "yes": {}, "yep": {}, "yeah": {}, "yup": {},
	"no": {}, "nope": {}, "done": {}, "klaar": {}, "gelukt": {},
	"top": {}, "prima": {}, "goed": {}, "fijn": {}, "mooi": {},
	"thanks": {}, "thank": {}, "thx": {}, "dank": {}, "dankje": {},
	"bedankt": {}, "dankuwel": {}, "dankjewel": {}, "cheers": {},
	"ping": {}, "pong": {}, "heartbeat": {},
}

// valueKeywords clamp the noise score down: a memory that contains one of
// these is likely a real preference, decision or instruction, and a coarse
// pattern must not sweep it up. The clamp is skipped when a secret was
// detected — a secret outranks usefulness.
var valueKeywords = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(prefer|voorkeur|always|altijd|never|nooit)\b`),
	regexp.MustCompile(`(?i)\b(decision|beslissing|decided|besloten|choose|kies)\b`),
	regexp.MustCompile(`(?i)\b(convention|conventie|rule|regel|policy|beleid)\b`),
	regexp.MustCompile(`(?i)\b(project|projekt|product|feature)\b`),
	regexp.MustCompile(`(?i)\b(architecture|architectuur|design|ontwerp)\b`),
	regexp.MustCompile(`(?i)\b(remember|onthoud|let op|noteer)\b`),
	regexp.MustCompile(`(?i)\b(important|belangrijk|crucial|critical|kritiek)\b`),
	regexp.MustCompile(`(?i)\b(goal|doel|target|doelstelling)\b`),
	regexp.MustCompile(`(?i)\b(lesson|les|learned|geleerd)\b`),
	regexp.MustCompile(`(?i)\b(be careful|pas op|attention|aandacht)\b`),
}

// Score thresholds. The score is not additive: each rule raises it to at least
// its own value, so one strong signal is enough and several weak ones do not
// compound into a false positive.
const (
	ScoreEmpty         = 1.0
	ScoreSecret        = 0.9
	ScoreTerminal      = 0.85
	ScoreStackTrace    = 0.85
	ScoreNoisePattern  = 0.8
	ScoreTrivial       = 0.7
	ScoreNoisySource   = 0.7
	ScoreLikelyDump    = 0.65
	ScoreLowImportance = 0.5
	// ScoreValueClamp is the ceiling a value keyword imposes on the score.
	ScoreValueClamp = 0.3
)

// ScoreContent computes the noise score of one memory. importance is the
// caller-assigned importance (0..1, 0.5 neutral); source is the origin tag
// (e.g. "cli", "heartbeat", "cron"). Both lower the score when they indicate
// a noisy origin.
func ScoreContent(content string, importance float64, source string) Score {
	var reasons []string
	score := 0.0
	secretLabels := detectSecrets(content)

	raise := func(v float64, reason string) {
		if v > score {
			score = v
		}
		reasons = append(reasons, reason)
	}

	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return Score{Value: ScoreEmpty, Reasons: []string{"empty_content"}}
	}

	if len(secretLabels) > 0 {
		for _, l := range secretLabels {
			raise(ScoreSecret, "secret_detected:"+l)
		}
	}

	for _, re := range terminalPatterns {
		if re.MatchString(content) {
			raise(ScoreTerminal, "terminal_output")
			break
		}
	}

	for _, re := range stackTracePatterns {
		if re.MatchString(content) {
			raise(ScoreStackTrace, "stack_trace")
			break
		}
	}

	for _, re := range noisePatterns {
		if re.MatchString(content) {
			raise(ScoreNoisePattern, "noise_pattern_match")
			break
		}
	}

	// Trivial keyword: the whole content is one acknowledgement word under
	// 15 characters.
	if len([]rune(trimmed)) < 15 {
		if _, ok := trivialKeywords[strings.ToLower(trimmed)]; ok {
			raise(ScoreTrivial, "trivial_keyword")
		}
	}

	// Noisy source: heartbeat, cron, debug or terminal origins are noise by
	// construction.
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "heartbeat", "cron", "debug", "terminal", "ping":
		raise(ScoreNoisySource, "noisy_source:"+strings.ToLower(strings.TrimSpace(source)))
	}

	// Likely dump: 30+ lines, 1000+ chars, few sentence breaks.
	if isLikelyDump(content) {
		raise(ScoreLikelyDump, "likely_dump")
	}

	// Low importance: the caller itself rated this as not mattering.
	if importance < 0.2 {
		raise(ScoreLowImportance, "low_importance")
	}

	// Value keyword clamp: a memory that says "prefer", "decision",
	// "architecture" etc. is probably worth keeping. Skipped when a secret
	// was detected — a secret outranks usefulness.
	if len(secretLabels) == 0 && score > ScoreValueClamp {
		for _, re := range valueKeywords {
			if re.MatchString(content) {
				if score > ScoreValueClamp {
					score = ScoreValueClamp
				}
				reasons = append(reasons, "value_keyword_clamp")
				break
			}
		}
	}

	if score > 1.0 {
		score = 1.0
	}
	if score < 0 {
		score = 0
	}
	return Score{Value: score, Reasons: reasons, SecretLabels: secretLabels}
}

// detectSecrets returns the labels of every secret pattern that matches. The
// matched values are never returned — only the labels, so the audit log can
// say "a github_token was found" without recording the token.
func detectSecrets(content string) []string {
	seen := make(map[string]struct{})
	var labels []string
	for _, sp := range secretPatterns {
		if sp.re.MatchString(content) {
			if _, ok := seen[sp.label]; !ok {
				seen[sp.label] = struct{}{}
				labels = append(labels, sp.label)
			}
		}
	}
	return labels
}

// isLikelyDump reports whether content looks like an unpunctuated dump: many
// lines, long, and few sentence-ending punctuation marks. Newlines do not
// count as sentence breaks — a log dump has one entry per line but no
// sentences.
func isLikelyDump(content string) bool {
	lines := strings.Split(content, "\n")
	if len(lines) < 30 {
		return false
	}
	runes := []rune(content)
	if len(runes) < 1000 {
		return false
	}
	sentenceBreaks := 0
	for i, r := range runes {
		if r == '.' || r == '!' || r == '?' {
			if i+1 >= len(runes) || runes[i+1] == ' ' || runes[i+1] == '\n' || runes[i+1] == '\t' {
				sentenceBreaks++
			}
		}
	}
	// Fewer than one sentence break per 200 chars in a 30+ line, 1000+ char
	// block is a dump, not prose.
	return sentenceBreaks < len(runes)/200
}

// Redact replaces every secret match with a placeholder carrying its label, so
// the output can be stored or logged without leaking the value.
func Redact(content string) string {
	return redactSecrets(content)
}

// redactSecrets replaces every secret match with a placeholder carrying its
// label, so a preview or log line can show that a secret was present without
// showing the secret.
func redactSecrets(content string) string {
	out := content
	for _, sp := range secretPatterns {
		out = sp.re.ReplaceAllString(out, "[REDACTED:"+sp.label+"]")
	}
	return out
}
