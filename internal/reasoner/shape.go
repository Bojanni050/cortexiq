// Modifications copyright (c) 2026 Bojan van der Heide
package reasoner

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go"
)

// shapeSystemPrompt is deliberately narrow. The grouping that produced each
// candidate is evidence of recurrence, so the model is not asked to find
// anything: it decides whether the sources really share the behaviour and, if
// they do, states it in one sentence. Keeping search out of the model is the
// whole point of the sweep — a model asked to find patterns in a pile of facts
// writes a summary, which is what filled one namespace with paraphrases.
const shapeSystemPrompt = `You are a pattern editor.

Each item you receive was produced by grouping relationships: one relation type
pointing at the same target from several distinct sources. The grouping is
evidence that something recurs, not proof of a regularity. Your job is to judge
the candidate and, if it holds, state it — not to describe the item.

A regularity is a behaviour: several different things do the same thing, or the
same thing happens under the same condition.

Rules:
- Use only the sources and statements given. Never add context, purpose, intent
  or design that they do not carry.
- A target that is a value or a name rather than a behaviour — a number, a date,
  a path, a file, a component, a function or a constant — is not a regularity.
  Grouping on it collects facts about one subject, so reject it.
- Reject when the sources are one thing under different names, when the relation
  type only records containment of a single object, or when the sources do not
  actually share the behaviour.
- Do not describe the grouping itself ("eleven services use X"). State what they
  do, in a form someone could act on.
- Never write about the evidence: no "these sources", no "the listed items", no
  count. A statement that cannot stand on its own is a rejection.
- Reject by returning an empty statement with coherence 0.0.`

// shapePrompt is kept separate from the call so its rules can be tested.
func shapePrompt(candidates []ShapeCandidate) string {
	var b strings.Builder
	b.WriteString("Below are candidate regularities, each grouped from relationships.\n")

	for i, c := range candidates {
		fmt.Fprintf(&b, "\nCandidate %d:\n  relation: %s\n  target: %s\n  sources (%d): %s\n",
			i+1, c.RelationType, c.Target, len(c.Sources), strings.Join(c.Sources, ", "))
		for _, s := range c.Samples {
			fmt.Fprintf(&b, "  source statement: %s\n", s)
		}
	}

	b.WriteString(`
Output ONLY a JSON array with one object per candidate, in the same order:
[{"candidate": 1, "statement": "string", "coherence": 0.0}]

Rules:
- candidate is the number above. Every candidate gets exactly one object.
- coherence is the share of the listed sources that fit the statement without an
  exception: 1.0 when all of them do, lower when one or two do not, 0.0 when the
  statement would not be true of them.
- The statement must stand on its own, as a claim about the world. Refer to
  nothing in this prompt: no "these sources", no "the listed items", no count.
- An empty statement with coherence 0.0 rejects the candidate.
- Answer with the array itself, not wrapped in an object.`)

	return b.String()
}

// shapeAnswer mirrors one element of the model's answer.
type shapeAnswer struct {
	Candidate int      `json:"candidate"`
	Statement string   `json:"statement"`
	Coherence *float32 `json:"coherence"`
}

// ReasonShapes asks the model to phrase or reject candidate regularities that
// were already found by grouping relationships. Every candidate gets a result:
// a missing or unusable answer counts as a rejection, so the caller can record
// it and move on instead of retrying the same candidate forever.
func (o *OpenAI) ReasonShapes(ctx context.Context, candidates []ShapeCandidate) ([]*ShapeStatement, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(shapeSystemPrompt),
		openai.UserMessage(shapePrompt(candidates)),
	}

	var raw string
	var result []*ShapeStatement

	for attempt := 0; attempt < 2; attempt++ {
		output, err := o.chat(ctx, "shapes", msgs)
		if err != nil {
			return nil, err
		}
		raw = unwrapTopLevelArray(extractJSON(output), "shapes", "candidate")

		var answers []shapeAnswer
		if err := decodeModelJSON(raw, &answers); err != nil {
			msgs = append(msgs, openai.SystemMessage(retryWarning))
			continue
		}

		out := make([]*ShapeStatement, len(candidates))
		usable := true
		for _, a := range answers {
			// The model answers with the number it was given, not an offset.
			if a.Candidate < 1 || a.Candidate > len(candidates) {
				usable = false
				break
			}

			coherence := float32(0.5)
			if a.Coherence != nil {
				switch c := *a.Coherence; {
				case c <= 0:
					coherence = 0 // explicit rejection
				case c <= 1:
					coherence = c
				default:
					coherence = 0.5
				}
			}
			statement := strings.TrimSpace(a.Statement)
			if statement == "" {
				coherence = 0
			}
			out[a.Candidate-1] = &ShapeStatement{Statement: statement, Coherence: coherence}
		}
		if !usable {
			msgs = append(msgs, openai.SystemMessage(retryWarning))
			continue
		}

		for i := range out {
			if out[i] == nil {
				out[i] = &ShapeStatement{Coherence: 0}
			}
		}
		result = out
		break
	}

	if result == nil {
		return nil, fmt.Errorf("parse json: unreadable answer to %d shape candidates: %.160s", len(candidates), raw)
	}
	return result, nil
}
