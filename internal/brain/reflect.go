// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/Bojanni050/cortexiq/internal/reasoner"
)

// ReflectResult is the output of a reflect operation: an answer synthesized
// from recalled memories, plus the memories that supported it.
type ReflectResult struct {
	Answer           string          `json:"answer"`
	Sources          []ReflectSource `json:"sources"`
	RecallCount      int             `json:"recall_count"`
	MentalModelCount int             `json:"mental_model_count"`
	// Hypotheses are the open beliefs the answer was warned about. They are
	// reported separately from Sources because they are not evidence: they are
	// the reason an answer may carry a caveat.
	Hypotheses      []ReflectHypothesis `json:"hypotheses,omitempty"`
	HypothesisCount int                 `json:"hypothesis_count"`
	DurationMs      int64               `json:"duration_ms"`
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
	// Tags narrows the grounding recall to memories carrying every listed tag
	// (AND, not OR), the same way RecallOptions.Tags does. Mental models keep
	// their own filter (MentalModelTags); this only applies to recalled memories.
	Tags []string
	// MMR overrides the server MMR lambda for the grounding recall. nil keeps
	// the server config; 0 disables MMR; 0..1 balances relevance vs diversity.
	MMR *float64
	// SkipMentalModels disables the mental model lookup in reflect. By default
	// reflect searches mental models first (they are curated summaries) and
	// includes them in the synthesis prompt.
	SkipMentalModels bool
	// MentalModelTags filters mental model search to models with any of these tags.
	MentalModelTags []string
	// Instructions are appended to the system prompt. The default prompt is
	// written for answering a question; a mental model needs document rules.
	Instructions string
	// SkipHypotheses keeps open hypotheses out of the synthesis. Curated
	// summaries and benchmark runs want settled knowledge only: a hypothesis is
	// by definition not something to write down as truth.
	SkipHypotheses bool
}

// reflectSystemPrompt is the system prompt for the reflect synthesis. It tells
// the LLM to answer only from the provided memories, cite what it used, and
// say so when the memories do not answer the question.
const reflectSystemPrompt = `You are a reflection agent that answers questions by reasoning over retrieved memories.

You may receive two kinds of input:
- MENTAL MODELS: curated, high-quality summaries about specific topics. These are the most reliable knowledge — prefer them over raw memories when they cover the question.
- MEMORIES: raw facts and episodes. Use these to fill gaps the mental models leave.

You may also receive OPEN HYPOTHESES: beliefs the system holds but has not verified. They are included only when they bear on the question.
- Never present a hypothesis as an established fact. If your answer rests on one, say so plainly and name what would settle it.
- A hypothesis with a counter-hypothesis means the system holds the opposing belief too. If both are listed, say the matter is contested instead of picking a side.
- A belief backed by facts from a single conversation is only as strong as that conversation; say so when it matters.

Rules:
- Answer ONLY from the input provided below. Never use outside knowledge.
- If the input does not contain the answer, say so clearly. Do not guess.
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

	var mentalModels []MentalModelSearchResult
	if !opts.SkipMentalModels {
		mmLimit := limit / 2
		if mmLimit < 1 {
			mmLimit = 1
		}
		if mmLimit > 5 {
			mmLimit = 5
		}
		mentalModels, _ = b.SearchMentalModels(ctx, namespaces, query, mmLimit, opts.MentalModelTags)
	}

	recallOpts := RecallOptions{
		AsOf:      opts.AsOf,
		From:      opts.From,
		To:        opts.To,
		KnownFrom: opts.KnownFrom,
		KnownTo:   opts.KnownTo,
		Types:     opts.Types,
		Tags:      opts.Tags,
		MMR:       opts.MMR,
	}
	results, err := b.RecallWith(ctx, namespaces, query, limit, recallOpts)
	if err != nil {
		return nil, fmt.Errorf("reflect recall: %w", err)
	}

	sources := make([]ReflectSource, 0, len(results)+len(mentalModels))
	for _, mm := range mentalModels {
		sources = append(sources, ReflectSource{
			Type:    "mental_model",
			ID:      mm.ID,
			Content: mm.Content,
			Score:   mm.Relevance,
		})
	}
	for _, r := range results {
		sources = append(sources, ReflectSource{
			Type:    r.Type,
			ID:      r.ID,
			Content: r.Content,
			Score:   r.Score,
		})
	}

	memoriesText := formatMemoriesForReflect(results)
	mmText := formatMentalModelsForReflect(mentalModels)

	// Open beliefs are matched against the same question, but after recall, so
	// they can be found by the facts they rest on as well as by their wording.
	// A failure here must not cost the answer: reflect still works without them.
	var hyps []models.Hypothesis
	if !opts.SkipHypotheses {
		nsIDs, err := b.resolveNamespaceIDs(ctx, namespaces)
		if err == nil {
			hyp, err := b.matchHypotheses(ctx, nsIDs, query, recalledFactIDs(results), 0)
			if err != nil {
				log.Printf("reflect: matching hypotheses failed, answering without them: %v", err)
			} else {
				hyps = hyp
			}
		}
	}
	hypText := formatHypothesesForReflect(hyps)

	var sb strings.Builder
	if mmText != "" {
		sb.WriteString("## Mental Models (curated summaries)\n\n")
		sb.WriteString(mmText)
		sb.WriteString("\n")
	}
	if hypText != "" {
		sb.WriteString("## Open Hypotheses (unverified)\n\n")
		sb.WriteString(hypText)
		sb.WriteString("\n")
	}
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
		{Role: "system", Content: reflectSystemPrompt + instructionsSuffix(opts.Instructions)},
		{Role: "user", Content: sb.String()},
	}

	answer, err := b.reasoner.Chat(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("reflect synthesize: %w", err)
	}

	return &ReflectResult{
		Answer:           answer,
		Sources:          sources,
		RecallCount:      len(results),
		MentalModelCount: len(mentalModels),
		Hypotheses:       toReflectHypotheses(hyps),
		HypothesisCount:  len(hyps),
		DurationMs:       time.Since(start).Milliseconds(),
	}, nil
}

// recalledFactIDs is the fact half of the recall results: the signal that a
// question is about the same thing a belief stands on.
func recalledFactIDs(results []RecallResult) []int64 {
	var ids []int64
	for _, r := range results {
		if r.Type == "fact" {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

func toReflectHypotheses(hyps []models.Hypothesis) []ReflectHypothesis {
	if len(hyps) == 0 {
		return nil
	}
	out := make([]ReflectHypothesis, 0, len(hyps))
	for _, h := range hyps {
		out = append(out, ReflectHypothesis{
			ID:              h.ID,
			NamespaceID:     h.NamespaceID,
			Content:         h.Content,
			Status:          h.Status,
			Confidence:      h.Confidence,
			SupportFacts:    h.SupportFacts,
			SupportEpisodes: h.SupportEpisodes,
			CounterOf:       h.CounterOf,
		})
	}
	return out
}

// formatHypothesesForReflect renders open beliefs for the synthesis prompt, with
// the two things that decide their worth: how well they are backed, and whether
// the system already holds the opposite.
func formatHypothesesForReflect(hyps []models.Hypothesis) string {
	if len(hyps) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, h := range hyps {
		meta := fmt.Sprintf("%s, confidence %.2f", h.Status, h.Confidence)
		switch {
		case h.SupportFacts == 0:
			meta += ", no supporting facts yet"
		case h.SupportEpisodes <= 1:
			meta += fmt.Sprintf(", %d fact(s) from a single conversation", h.SupportFacts)
		default:
			meta += fmt.Sprintf(", %d facts from %d conversations", h.SupportFacts, h.SupportEpisodes)
		}
		if h.CounterOf != nil {
			meta += fmt.Sprintf(", counter to hypothesis #%d", *h.CounterOf)
		}
		fmt.Fprintf(&sb, "%d. [%s] %s\n", i+1, meta, h.Content)
		if h.VerificationPlan != "" {
			fmt.Fprintf(&sb, "   would be settled by: %s\n", h.VerificationPlan)
		}
	}
	return sb.String()
}

// instructionsSuffix renders caller instructions as an extra system-prompt
// paragraph, or "" when there are none.
func instructionsSuffix(instructions string) string {
	if strings.TrimSpace(instructions) == "" {
		return ""
	}
	return "\n\n" + instructions
}

// formatMentalModelsForReflect renders curated mental models for the synthesis
// prompt. They appear before raw memories because they are higher quality.
func formatMentalModelsForReflect(models []MentalModelSearchResult) string {
	if len(models) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, mm := range models {
		stale := ""
		if mm.IsStale {
			stale = " [STALE"
			if mm.StaleReason != "" {
				stale += ": " + mm.StaleReason
			}
			stale += " - may need refresh]"
		}
		fmt.Fprintf(&sb, "%d. [%s]%s\n%s\n", i+1, mm.Name, stale, mm.Content)
	}
	return sb.String()
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
