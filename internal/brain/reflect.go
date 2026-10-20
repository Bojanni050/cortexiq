// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Bojanni050/cortexiq/internal/reasoner"
)

// ReflectResult is the output of a reflect operation: an answer synthesized
// from recalled memories, plus the memories that supported it.
type ReflectResult struct {
	Answer     string         `json:"answer"`
	Sources    []ReflectSource `json:"sources"`
	RecallCount int           `json:"recall_count"`
	DurationMs int64          `json:"duration_ms"`
}

// ReflectSource is one memory that contributed to the answer.
type ReflectSource struct {
	Type    string  `json:"type"` // "fact", "episode", "failure", or "goal"
	ID      int64   `json:"id"`
	Content string  `json:"content"`
	Score   float32 `json:"score"`
}

// ReflectOptions configures a reflect call.
type ReflectOptions struct {
	// Namespaces limits the recall to these paths (each includes descendants).
	// Empty means all namespaces.
	Namespaces []string
	// Limit is how many memories are recalled to ground the answer.
	Limit int
	// MaxTokens caps the answer length the LLM may produce. 0 means no cap.
	MaxTokens int
	// Context is additional context the caller wants included (e.g. the current
	// conversation or task description).
	Context string
	// AsOf answers "what did we know at that moment"; see RecallAsOf.
	AsOf *time.Time
	// From and To bound the valid-time window; see RecallOptions.
	From *time.Time
	To   *time.Time
	// KnownFrom and KnownTo bound the knowledge-time window; see RecallOptions.
	KnownFrom *time.Time
	KnownTo   *time.Time
	// Types limits which memory types are recalled. Empty means facts and episodes.
	Types []string
}

// reflectSystemPrompt is the system prompt for the reflect synthesis. It tells
// the LLM to answer only from the provided memories, cite what it used, and
// say so when the memories do not answer the question.
const reflectSystemPrompt = `You are a reflection agent that answers questions by reasoning over retrieved memories.

Rules:
- Answer ONLY from the memories provided below. Never use outside knowledge.
- If the memories do not contain the answer, say so clearly. Do not guess.
- Be concise and direct. Use the same language as the question.
- When memories conflict, note the conflict and say which is more recent or better supported.
- Do not mention that you are an AI or that you used a memory system.
- Format your answer in Markdown when structure helps (lists, tables, bold).`

// Reflect answers a question by recalling relevant memories and synthesizing
// an answer from them with the LLM. It is the natural-language counterpart to
// Recall: where Recall returns ranked memories, Reflect returns a grounded answer.
//
// The recall grounds the answer: every claim should trace back to a recalled
// memory. When nothing relevant is found, the LLM is told to say so rather than
// invent an answer.
func (b *Brain) Reflect(ctx context.Context, query string, opts ReflectOptions) (*ReflectResult, error) {
	start := time.Now()

	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("brain: reflect query cannot be empty")
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}

	namespaces := opts.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{"/"}
	}

	recallOpts := RecallOptions{
		AsOf:      opts.AsOf,
		From:      opts.From,
		To:        opts.To,
		KnownFrom: opts.KnownFrom,
		KnownTo:   opts.KnownTo,
		Types:     opts.Types,
	}
	results, err := b.RecallWith(ctx, namespaces, query, limit, recallOpts)
	if err != nil {
		return nil, fmt.Errorf("reflect recall: %w", err)
	}

	sources := make([]ReflectSource, 0, len(results))
	for _, r := range results {
		sources = append(sources, ReflectSource{
			Type:    r.Type,
			ID:      r.ID,
			Content: r.Content,
			Score:   r.Score,
		})
	}

	memoriesText := formatMemoriesForReflect(results)

	var sb strings.Builder
	sb.WriteString("## Memories\n\n")
	if memoriesText == "" {
		sb.WriteString("(no relevant memories found)\n")
	} else {
		sb.WriteString(memoriesText)
		sb.WriteString("\n")
	}

	if opts.Context != "" {
		sb.WriteString("## Additional Context\n\n")
		sb.WriteString(opts.Context)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Question\n\n")
	sb.WriteString(query)
	sb.WriteString("\n")

	messages := []reasoner.ChatMessage{
		{Role: "system", Content: reflectSystemPrompt},
		{Role: "user", Content: sb.String()},
	}

	answer, err := b.reasoner.Chat(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("reflect synthesize: %w", err)
	}

	return &ReflectResult{
		Answer:      answer,
		Sources:     sources,
		RecallCount: len(results),
		DurationMs:  time.Since(start).Milliseconds(),
	}, nil
}

// formatMemoriesForReflect renders recalled memories as a numbered list for
// the synthesis prompt. Facts and episodes are labelled by type so the LLM can
// tell consolidated truths from raw observations.
func formatMemoriesForReflect(results []RecallResult) string {
	if len(results) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, r := range results {
		label := r.Type
		if r.Type == "fact" && r.Confidence > 0 {
			label = fmt.Sprintf("fact (confidence %.2f)", r.Confidence)
		}
		fmt.Fprintf(&sb, "%d. [%s] %s\n", i+1, label, r.Content)
		if r.OccurredAt != "" {
			fmt.Fprintf(&sb, "   (occurred: %s)\n", r.OccurredAt)
		}
		if r.ValidFrom != "" {
			fmt.Fprintf(&sb, "   (valid from: %s)\n", r.ValidFrom)
		}
	}
	return sb.String()
}
