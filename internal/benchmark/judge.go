// Modifications copyright (c) 2026 Bojan van der Heide
package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Bojanni050/cortexiq/internal/reasoner"
)

// LLMJudge uses an LLM to score how well a generated answer matches the
// expected answer. It is more nuanced than exact match: it understands
// paraphrases, partial answers, and missing information.
type LLMJudge struct {
	reasoner reasoner.Reasoner
}

// NewLLMJudge creates a judge that uses the given reasoner to score answers.
func NewLLMJudge(r reasoner.Reasoner) *LLMJudge {
	return &LLMJudge{reasoner: r}
}

// judgeSystemPrompt instructs the LLM to act as a strict evaluator.
const judgeSystemPrompt = `You are a strict answer evaluator. You compare a generated answer against the expected (gold) answer and score how correct the generated answer is.

Scoring rules:
- 1.0: The generated answer is fully correct and contains all key information from the expected answer.
- 0.75: The generated answer is mostly correct but misses a minor detail or is slightly imprecise.
- 0.5: The generated answer is partially correct — it has some right information but misses key points.
- 0.25: The generated answer is mostly wrong — it has a grain of truth but is largely incorrect.
- 0.0: The generated answer is completely wrong or says it doesn't know when the expected answer has the information.

You must respond with ONLY a JSON object:
{"score": <0.0-1.0>, "reasoning": "<one sentence explaining the score>"}

No explanation outside the JSON. No markdown. Just the JSON object.`

// judgeResponse is the expected JSON output from the LLM.
type judgeResponse struct {
	Score     float64 `json:"score"`
	Reasoning string  `json:"reasoning"`
}

// Judge scores a generated answer against the expected answer using the LLM.
func (j *LLMJudge) Judge(ctx context.Context, question, expected, generated string) (float64, string, error) {
	if strings.TrimSpace(generated) == "" {
		return 0.0, "empty answer", nil
	}

	prompt := fmt.Sprintf(`Question: %s

Expected answer: %s

Generated answer: %s

Score the generated answer.`, question, expected, generated)

	messages := []reasoner.ChatMessage{
		{Role: "system", Content: judgeSystemPrompt},
		{Role: "user", Content: prompt},
	}

	resp, err := j.reasoner.Chat(ctx, messages)
	if err != nil {
		return 0, "", fmt.Errorf("judge LLM call: %w", err)
	}

	// Parse the JSON response, tolerating markdown code blocks.
	score, reasoning, err := parseJudgeResponse(resp)
	if err != nil {
		// If parsing fails, fall back to a keyword heuristic.
		return fallbackScore(expected, generated), "fallback heuristic (LLM response unparseable)", nil
	}

	// Clamp score to [0, 1].
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score, reasoning, nil
}

// parseJudgeResponse extracts the score and reasoning from the LLM output.
// It handles raw JSON, markdown code blocks, and leading/trailing text.
func parseJudgeResponse(resp string) (float64, string, error) {
	// Strip markdown code blocks if present.
	text := strings.TrimSpace(resp)
	if strings.HasPrefix(text, "```") {
		// Remove ```json or ``` prefix and trailing ```
		if idx := strings.Index(text, "\n"); idx >= 0 {
			text = text[idx+1:]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
		text = strings.TrimSpace(text)
	}

	// Try to find a JSON object in the text.
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return 0, "", fmt.Errorf("no JSON object found in response")
	}
	jsonText := text[start : end+1]

	var jr judgeResponse
	if err := json.Unmarshal([]byte(jsonText), &jr); err != nil {
		return 0, "", fmt.Errorf("invalid JSON: %w", err)
	}
	return jr.Score, jr.Reasoning, nil
}

// fallbackScore uses a simple keyword overlap heuristic when the LLM response
// cannot be parsed. It is less accurate but always returns a number.
func fallbackScore(expected, generated string) float64 {
	expectedWords := wordSet(expected)
	generatedWords := wordSet(generated)
	if len(expectedWords) == 0 {
		return 0
	}
	matches := 0
	for w := range generatedWords {
		if expectedWords[w] {
			matches++
		}
	}
	// Jaccard-like score: intersection over union, scaled to be a bit generous.
	union := len(expectedWords) + len(generatedWords) - matches
	if union == 0 {
		return 0
	}
	return float64(matches) / float64(union)
}

// wordSet returns the set of lowercase alphanumeric words in a text.
func wordSet(text string) map[string]bool {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	set := make(map[string]bool, len(words))
	for _, w := range words {
		if len(w) > 2 { // skip short words like "a", "is", "to"
			set[w] = true
		}
	}
	return set
}

// ParseScore is a helper that extracts a numeric score from a string.
// It handles "0.85", "85%", "score: 0.9", etc.
func ParseScore(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	s = strings.TrimPrefix(s, "score:")
	s = strings.TrimSpace(s)
	return strconv.ParseFloat(s, 64)
}
