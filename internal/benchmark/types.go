// Modifications copyright (c) 2026 Bojan van der Heide
// Package benchmark runs standardized memory benchmarks against CortexIQ.
//
// It is inspired by Hindsight's LongMemEval integration but adapted to CortexIQ's
// architecture: sessions are stored as episodes, questions are answered with
// reflect, and answers are judged by an LLM or by exact match.
package benchmark

import (
	"context"
	"strings"
	"time"
)

// Session is one conversation turn-group to ingest into memory. It mirrors the
// LongMemEval haystack_sessions format: a list of chat turns with a date.
type Session struct {
	// ID is a unique identifier for this session within the item.
	ID string `json:"id"`
	// Date is when this session happened (RFC3339 or YYYY-MM-DD).
	Date string `json:"date"`
	// Turns are the conversation turns. Each turn is {"role": "user|assistant", "content": "..."}.
	Turns []Turn `json:"turns"`
}

// Turn is one message in a session.
type Turn struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content string `json:"content"` // the message text
}

// Question is one benchmark question with its expected answer.
type Question struct {
	// ID is a unique identifier for this question.
	ID string `json:"id"`
	// Category classifies the question (e.g. "single-session", "multi-session", "temporal").
	Category string `json:"category"`
	// Question is the natural language question to answer.
	Question string `json:"question"`
	// Answer is the gold-standard answer.
	Answer string `json:"answer"`
	// QuestionDate is when the question was asked (for temporal questions).
	QuestionDate string `json:"question_date,omitempty"`
}

// Item is one benchmark item: a set of sessions to ingest, then questions to answer.
// This mirrors the LongMemEval format: haystack_sessions + haystack_dates + qa_pairs.
type Item struct {
	// ID is a unique identifier for this item.
	ID string `json:"question_id"` // LongMemEval uses "question_id" for the item ID
	// Sessions are the conversation sessions to ingest.
	Sessions []Session `json:"haystack_sessions"`
	// Questions are the questions to answer after ingesting the sessions.
	Questions []Question `json:"qa_pairs"`
}

// Dataset is a collection of benchmark items loaded from a JSON file.
type Dataset struct {
	// Name is the benchmark name (e.g. "longmemeval", "locomo").
	Name string `json:"name"`
	// Items are the benchmark items.
	Items []Item `json:"items"`
}

// BenchmarkResult is the outcome of one question.
type BenchmarkResult struct {
	// QuestionID is the question identifier.
	QuestionID string `json:"question_id"`
	// Category is the question category.
	Category string `json:"category"`
	// Question is the question text.
	Question string `json:"question"`
	// ExpectedAnswer is the gold-standard answer.
	ExpectedAnswer string `json:"expected_answer"`
	// GeneratedAnswer is what CortexIQ produced.
	GeneratedAnswer string `json:"generated_answer"`
	// Sources are the memories that grounded the answer.
	Sources []string `json:"sources,omitempty"`
	// JudgeScore is the LLM judge score (0.0 to 1.0), or -1 if not judged.
	JudgeScore float64 `json:"judge_score"`
	// JudgeReasoning explains the judge's score.
	JudgeReasoning string `json:"judge_reasoning,omitempty"`
	// ExactMatch is true when the generated answer exactly matches the expected.
	ExactMatch bool `json:"exact_match"`
	// DurationMs is how long the question took (reflect + judge).
	DurationMs int64 `json:"duration_ms"`
	// Error is set when the question failed.
	Error string `json:"error,omitempty"`
}

// ItemResult is the outcome of one benchmark item (all its questions).
type ItemResult struct {
	// ItemID is the item identifier.
	ItemID string `json:"item_id"`
	// Questions are the per-question results.
	Questions []BenchmarkResult `json:"questions"`
	// IngestedEpisodes is how many episodes were stored for this item.
	IngestedEpisodes int `json:"ingested_episodes"`
	// DurationMs is the total time for this item (ingestion + all questions).
	DurationMs int64 `json:"duration_ms"`
}

// BenchmarkReport is the full benchmark report.
type BenchmarkReport struct {
	// Name is the benchmark name.
	Name string `json:"name"`
	// StartedAt is when the benchmark started.
	StartedAt time.Time `json:"started_at"`
	// DurationMs is the total benchmark duration.
	DurationMs int64 `json:"duration_ms"`
	// Items is the number of items processed.
	Items int `json:"items"`
	// Questions is the total number of questions.
	Questions int `json:"questions"`
	// Results are the per-item results.
	Results []ItemResult `json:"results"`
	// Summary contains aggregate metrics.
	Summary Summary `json:"summary"`
	// Config records the benchmark configuration.
	Config Config `json:"config"`
}

// Summary contains aggregate benchmark metrics.
type Summary struct {
	// Questions is the total number of questions evaluated.
	Questions int `json:"questions"`
	// Accuracy is the fraction of questions with judge score >= 0.5 (or exact match).
	Accuracy float64 `json:"accuracy"`
	// AverageJudgeScore is the mean judge score across all questions.
	AverageJudgeScore float64 `json:"average_judge_score"`
	// ExactMatchRate is the fraction of exact matches.
	ExactMatchRate float64 `json:"exact_match_rate"`
	// AverageLatencyMs is the mean time per question.
	AverageLatencyMs float64 `json:"average_latency_ms"`
	// TotalEpisodes is the total number of episodes ingested.
	TotalEpisodes int `json:"total_episodes"`
	// Errors is the number of questions that failed.
	Errors int `json:"errors"`
	// ByCategory breaks down accuracy by question category.
	ByCategory map[string]CategoryStats `json:"by_category"`
}

// CategoryStats are the metrics for one question category.
type CategoryStats struct {
	Questions int     `json:"questions"`
	Accuracy  float64 `json:"accuracy"`
	AvgScore  float64 `json:"avg_score"`
}

// Config records the benchmark configuration for reproducibility.
type Config struct {
	// Namespace is the namespace prefix used for benchmark data.
	Namespace string `json:"namespace"`
	// Limit is the recall limit used for reflect.
	Limit int `json:"limit"`
	// JudgeModel is the model used for judging (empty = exact match only).
	JudgeModel string `json:"judge_model,omitempty"`
	// MaxItems limits how many items to process (0 = all).
	MaxItems int `json:"max_items"`
	// Keep keeps the benchmark namespace after the run.
	Keep bool `json:"keep"`
}

// Runner runs a benchmark against a CortexIQ brain.
type Runner interface {
	// Run executes the benchmark and returns the report.
	Run(ctx context.Context, dataset *Dataset, cfg Config) (*BenchmarkReport, error)
}

// Judge evaluates a generated answer against the expected answer.
type Judge interface {
	// Judge scores the answer. Returns a score 0.0-1.0 and a reasoning string.
	Judge(ctx context.Context, question, expected, generated string) (score float64, reasoning string, err error)
}

// ExactMatchJudge is a judge that only checks for exact string match.
type ExactMatchJudge struct{}

// Judge returns 1.0 for exact match (case-insensitive, trimmed), 0.0 otherwise.
func (ExactMatchJudge) Judge(_ context.Context, _, expected, generated string) (float64, string, error) {
	match := strings.EqualFold(strings.TrimSpace(expected), strings.TrimSpace(generated))
	if match {
		return 1.0, "exact match", nil
	}
	return 0.0, "no exact match", nil
}
