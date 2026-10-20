// Modifications copyright (c) 2026 Bojan van der Heide
package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Bojanni050/cortexiq/internal/brain"
)

// CortexIQRunner runs a benchmark against a CortexIQ brain. It ingests each
// item's sessions as episodes, answers each question with reflect, and judges
// the answer with the configured judge.
type CortexIQRunner struct {
	brain *brain.Brain
	judge Judge
}

// NewRunner creates a benchmark runner for the given brain and judge.
func NewRunner(b *brain.Brain, judge Judge) *CortexIQRunner {
	if judge == nil {
		judge = ExactMatchJudge{}
	}
	return &CortexIQRunner{brain: b, judge: judge}
}

// Run executes the benchmark: for each item, ingest sessions, answer questions,
// judge answers, and aggregate the results into a report.
func (r *CortexIQRunner) Run(ctx context.Context, dataset *Dataset, cfg Config) (*BenchmarkReport, error) {
	start := time.Now()
	cfg.Namespace = normalizeNamespace(cfg.Namespace)
	if cfg.Limit <= 0 {
		cfg.Limit = 10
	}

	items := dataset.Items
	if cfg.MaxItems > 0 && cfg.MaxItems < len(items) {
		items = items[:cfg.MaxItems]
	}

	report := &BenchmarkReport{
		Name:      dataset.Name,
		StartedAt: start,
		Items:     len(items),
		Questions: countQuestions(items),
		Config:    cfg,
		Results:   make([]ItemResult, 0, len(items)),
	}

	for _, item := range items {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		itemResult, err := r.runItem(ctx, item, cfg)
		if err != nil {
			return nil, fmt.Errorf("item %s: %w", item.ID, err)
		}
		report.Results = append(report.Results, itemResult)
	}

	report.DurationMs = time.Since(start).Milliseconds()
	report.Summary = summarizeResults(report.Results)

	// Clean up the benchmark namespace unless Keep is set. Only delete the
	// benchmark prefix, never anything else: a bug here must not touch real
	// memories. The DeleteNamespace cascade removes all child namespaces and
	// their data.
	if !cfg.Keep {
		_ = r.brain.DeleteNamespace(ctx, cfg.Namespace)
	}

	return report, nil
}

// runItem processes one benchmark item: ingest sessions, answer questions.
func (r *CortexIQRunner) runItem(ctx context.Context, item Item, cfg Config) (ItemResult, error) {
	start := time.Now()
	itemNS := cfg.Namespace + "/" + sanitizeSlug(item.ID)

	// Create the item's namespace.
	if _, err := r.brain.CreateNamespace(ctx, itemNS, item.ID, "Benchmark item "+item.ID); err != nil {
		// Namespace may already exist from a previous run; that's fine.
		if !strings.Contains(err.Error(), "already exists") {
			return ItemResult{}, fmt.Errorf("create namespace: %w", err)
		}
	}

	result := ItemResult{ItemID: item.ID}

	// Ingest each session as an episode.
	for _, session := range item.Sessions {
		content := formatSession(session)
		occurredAt := parseSessionDate(session.Date)
		if _, err := r.brain.Remember(ctx, itemNS, content, occurredAt); err != nil {
			// Social closers and empty content are skipped; that's expected.
			if !strings.Contains(err.Error(), "social") && !strings.Contains(err.Error(), "empty") {
				return ItemResult{}, fmt.Errorf("ingest session %s: %w", session.ID, err)
			}
			continue
		}
		result.IngestedEpisodes++
	}

	// Run consolidation so episodes become facts.
	if _, err := r.brain.Consolidate(ctx, itemNS); err != nil {
		return ItemResult{}, fmt.Errorf("consolidate: %w", err)
	}

	// Answer each question with reflect.
	for _, q := range item.Questions {
		qResult := r.answerQuestion(ctx, itemNS, q, cfg.Limit)
		result.Questions = append(result.Questions, qResult)
	}

	result.DurationMs = time.Since(start).Milliseconds()
	return result, nil
}

// answerQuestion answers one question with reflect and judges the answer.
func (r *CortexIQRunner) answerQuestion(ctx context.Context, namespace string, q Question, limit int) BenchmarkResult {
	start := time.Now()
	result := BenchmarkResult{
		QuestionID:     q.ID,
		Category:       q.Category,
		Question:       q.Question,
		ExpectedAnswer: q.Answer,
		JudgeScore:     -1,
	}

	// Parse question date for temporal questions.
	var asOf *time.Time
	if q.QuestionDate != "" {
		if t, err := time.Parse(time.RFC3339, q.QuestionDate); err == nil {
			asOf = &t
		} else if t, err := time.Parse("2006-01-02", q.QuestionDate); err == nil {
			asOf = &t
		}
	}

	reflectResult, err := r.brain.Reflect(ctx, q.Question, brain.ReflectOptions{
		Namespaces: []string{namespace},
		Limit:      limit,
		AsOf:       asOf,
	})
	if err != nil {
		result.Error = err.Error()
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}

	result.GeneratedAnswer = reflectResult.Answer
	for _, src := range reflectResult.Sources {
		result.Sources = append(result.Sources, src.Content)
	}

	// Judge the answer.
	score, reasoning, err := r.judge.Judge(ctx, q.Question, q.Answer, reflectResult.Answer)
	if err != nil {
		result.Error = fmt.Sprintf("judge: %v", err)
	} else {
		result.JudgeScore = score
		result.JudgeReasoning = reasoning
	}

	// Also check exact match.
	exactJudge := ExactMatchJudge{}
	exactScore, _, _ := exactJudge.Judge(ctx, q.Question, q.Answer, reflectResult.Answer)
	result.ExactMatch = exactScore == 1.0

	result.DurationMs = time.Since(start).Milliseconds()
	return result
}

// formatSession renders a session as a single episode text. Each turn is on its
// own line, prefixed with the role, so the extractor sees a conversation.
func formatSession(s Session) string {
	var sb strings.Builder
	if s.Date != "" {
		fmt.Fprintf(&sb, "[Conversation on %s]\n", s.Date)
	}
	for _, turn := range s.Turns {
		role := strings.ToUpper(turn.Role[:1]) + turn.Role[1:]
		fmt.Fprintf(&sb, "%s: %s\n", role, turn.Content)
	}
	return strings.TrimSpace(sb.String())
}

// parseSessionDate parses a session date, returning nil for empty/invalid dates.
func parseSessionDate(date string) *time.Time {
	if date == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, date); err == nil {
			return &t
		}
	}
	return nil
}

// summarizeResults aggregates per-question results into summary metrics.
func summarizeResults(results []ItemResult) Summary {
	summary := Summary{
		ByCategory:    map[string]CategoryStats{},
		TotalEpisodes: 0,
	}

	var totalScore float64
	var totalLatency float64
	var correctCount int
	var exactCount int
	var questionCount int

	catStats := map[string]*CategoryStats{}

	for _, item := range results {
		summary.TotalEpisodes += item.IngestedEpisodes
		for _, q := range item.Questions {
			questionCount++
			totalLatency += float64(q.DurationMs)

			// Use judge score if available, otherwise exact match. Errored
			// questions count as score 0 but still appear in category stats.
			score := q.JudgeScore
			if q.Error != "" {
				summary.Errors++
				score = 0
			} else if score < 0 {
				if q.ExactMatch {
					score = 1.0
				} else {
					score = 0.0
				}
			}
			totalScore += score
			if score >= 0.5 {
				correctCount++
			}
			if q.ExactMatch {
				exactCount++
			}

			// Per-category stats.
			cat := q.Category
			if cat == "" {
				cat = "uncategorized"
			}
			if catStats[cat] == nil {
				catStats[cat] = &CategoryStats{}
				summary.ByCategory[cat] = CategoryStats{}
			}
			cs := catStats[cat]
			cs.Questions++
			cs.AvgScore += score
			if score >= 0.5 {
				cs.Accuracy++
			}
		}
	}

	summary.Questions = questionCount
	if questionCount > 0 {
		summary.Accuracy = float64(correctCount) / float64(questionCount)
		summary.AverageJudgeScore = totalScore / float64(questionCount)
		summary.ExactMatchRate = float64(exactCount) / float64(questionCount)
		summary.AverageLatencyMs = totalLatency / float64(questionCount)
	}

	// Finalize category stats.
	for cat, cs := range catStats {
		if cs.Questions > 0 {
			summary.ByCategory[cat] = CategoryStats{
				Questions: cs.Questions,
				Accuracy:  cs.Accuracy / float64(cs.Questions),
				AvgScore:  cs.AvgScore / float64(cs.Questions),
			}
		}
	}

	return summary
}

// normalizeNamespace ensures the namespace starts with / and has no trailing /.
func normalizeNamespace(ns string) string {
	if ns == "" {
		return "/benchmark"
	}
	if !strings.HasPrefix(ns, "/") {
		ns = "/" + ns
	}
	ns = strings.TrimSuffix(ns, "/")
	if ns == "" {
		return "/benchmark"
	}
	return ns
}

// sanitizeSlug converts an item ID into a valid namespace slug segment.
func sanitizeSlug(id string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(id) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '-' || r == '_':
			sb.WriteRune(r)
		default:
			sb.WriteRune('-')
		}
	}
	slug := sb.String()
	if slug == "" {
		slug = "item"
	}
	// Namespace segments must start with alphanumeric.
	if slug[0] == '-' {
		slug = "i" + slug
	}
	return slug
}

// countQuestions counts the total questions across items.
func countQuestions(items []Item) int {
	n := 0
	for _, item := range items {
		n += len(item.Questions)
	}
	return n
}

// MarshalJSON serializes a report to indented JSON.
func MarshalJSON(report *BenchmarkReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}
