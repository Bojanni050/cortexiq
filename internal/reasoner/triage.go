// Modifications copyright (c) 2026 Bojan van der Heide
package reasoner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openai "github.com/openai/openai-go"
)

// TriageVerdict is the cheap decision made before the extractor reads a cluster
// of episodes. It answers one question only: is there anything here that a
// memory should hold?
type TriageVerdict string

const (
	// TriageDurable means the text carries a claim, decision, state change,
	// number, name or outcome — something worth extracting into facts.
	TriageDurable TriageVerdict = "durable"
	// TriageChatter means the text has no information of its own: a greeting,
	// thanks, an acknowledgement, a status ping, or thinking out loud.
	TriageChatter TriageVerdict = "chatter"
)

// TriageResult carries the verdict and the model's reason, so a skipped episode
// can be explained later instead of vanishing silently.
type TriageResult struct {
	Verdict TriageVerdict `json:"verdict"`
	Reason  string        `json:"reason"`
}

const triageSystem = `You sort text from an assistant's working session before it is stored as memory.
You receive one or more pieces of text. Decide whether they contain anything a
long-term memory should keep: a fact, a decision, a state change, a preference,
a number, a name, or the outcome of an action.

Answer "chatter" when the text holds no information of its own: greetings, thanks,
acknowledgements ("ok", "done", "that worked"), status pings, restatements of the
request, or thinking out loud with no conclusion.

Answer "durable" otherwise. When in doubt, answer "durable": skipping a real
memory is worse than storing a dull one.

Output ONLY JSON: {"verdict":"durable|chatter","reason":"one short sentence"}`

// Triage classifies a cluster of episodes before the extractor runs. It exists
// because extraction is the expensive call in consolidation: asked to find facts
// in a session of acknowledgements, it will sometimes invent them. A cheaper
// model can refuse that work first.
//
// The call fails open: an unparseable answer, a missing verdict or a provider
// error all resolve to TriageDurable, so a flaky triage model can only cost
// money, never memories.
func (o *OpenAI) Triage(ctx context.Context, texts []string) (*TriageResult, error) {
	var b strings.Builder
	for i, t := range texts {
		fmt.Fprintf(&b, "--- %d ---\n%s\n", i+1, strings.TrimSpace(t))
	}

	out, err := o.chat(ctx, "triage", []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(triageSystem),
		openai.UserMessage(b.String()),
	})
	if err != nil {
		return nil, err
	}

	var res TriageResult
	if err := json.Unmarshal([]byte(extractJSON(out)), &res); err != nil {
		return &TriageResult{Verdict: TriageDurable, Reason: "triage answer was not parseable"}, nil
	}
	switch res.Verdict {
	case TriageChatter:
		if strings.TrimSpace(res.Reason) == "" {
			res.Reason = "no information worth keeping"
		}
		return &res, nil
	case TriageDurable:
		return &res, nil
	default:
		return &TriageResult{Verdict: TriageDurable, Reason: fmt.Sprintf("unrecognised verdict %q", res.Verdict)}, nil
	}
}
