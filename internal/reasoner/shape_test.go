// Modifications copyright (c) 2026 Bojan van der Heide
package reasoner

import (
	"context"
	"strings"
	"testing"
)

func shapeCandidates() []ShapeCandidate {
	return []ShapeCandidate{
		{
			RelationType: "uses_transport",
			Target:       "http-first transport",
			Sources:      []string{"alpha", "beta", "gamma"},
			Samples:      []string{"alpha reaches its server over the http-first transport"},
		},
		{
			RelationType: "runs_on",
			Target:       "strato vps",
			Sources:      []string{"delta", "epsilon", "zeta"},
		},
	}
}

func TestShapePromptAsksToPhraseNotToFind(t *testing.T) {
	p := shapePrompt(shapeCandidates())

	for _, want := range []string{
		"Candidate 1:",
		"relation: uses_transport",
		"target: http-first transport",
		"sources (3): alpha, beta, gamma",
		"source statement: alpha reaches its server over the http-first transport",
		"Every candidate gets exactly one object",
		"empty statement with coherence 0.0 rejects",
		"The statement must stand on its own, as a claim about the world",
		`no "these sources", no "the listed items", no count`,
		"not wrapped in an object",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("shapePrompt is missing %q", want)
		}
	}
}

// The sweep exists because a model asked to find patterns writes summaries. Its
// system prompt must therefore keep search out and allow refusal.
func TestShapeSystemPromptKeepsSearchOut(t *testing.T) {
	for _, want := range []string{
		"The grouping is\nevidence that something recurs",
		"Never add context, purpose, intent",
		"Reject when the sources are one thing under different names",
		"Reject by returning an empty statement with coherence 0.0",
	} {
		if !strings.Contains(shapeSystemPrompt, want) {
			t.Errorf("shapeSystemPrompt is missing %q", want)
		}
	}
}

// Production accepted "the archive list column has a minimum width of 240px" and
// "released on 2026-10-08": a value target is one subject described many times,
// not a behaviour several things share.
func TestShapeSystemPromptRejectsValueTargets(t *testing.T) {
	for _, want := range []string{
		"A target that is a value or a name rather than a behaviour",
		"Grouping on it collects facts about one subject, so reject it",
	} {
		if !strings.Contains(shapeSystemPrompt, want) {
			t.Errorf("shapeSystemPrompt is missing %q", want)
		}
	}
}

// Production stored "These sources are involved in configuring a Claude Desktop
// integration…". The statement has to be about the world, not about the input.
func TestShapeSystemPromptForbidsDescribingTheEvidence(t *testing.T) {
	for _, want := range []string{
		"Never write about the evidence",
		`no "these sources", no "the listed items", no`,
	} {
		if !strings.Contains(shapeSystemPrompt, want) {
			t.Errorf("shapeSystemPrompt is missing %q", want)
		}
	}
}

// A model with one candidate answers with the element itself. Losing that answer
// threw away four judged candidates per run, so the object is wrapped.
func TestReasonShapesAcceptsASingleObject(t *testing.T) {
	srv, _ := triageServer(t, `{"candidate": 1, "statement": "every client uses the http-first transport", "coherence": 1.0}`)
	r := newTriage(t, srv.URL)

	got, err := r.ReasonShapes(context.Background(), shapeCandidates()[:1])
	if err != nil {
		t.Fatalf("ReasonShapes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want one result, got %d", len(got))
	}
	if got[0].Statement != "every client uses the http-first transport" || got[0].Coherence != 1.0 {
		t.Fatalf("a bare object was not read as one element: %+v", got[0])
	}
}

func TestReasonShapesMapsAnswersOntoCandidates(t *testing.T) {
	srv, _ := triageServer(t, `[
		{"candidate": 2, "statement": "every service is deployed on the same VPS", "coherence": 0.9},
		{"candidate": 1, "statement": "", "coherence": 0.0}
	]`)
	r := newTriage(t, srv.URL)

	got, err := r.ReasonShapes(context.Background(), shapeCandidates())
	if err != nil {
		t.Fatalf("ReasonShapes: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want one statement per candidate, got %d", len(got))
	}
	if got[0].Statement != "" || got[0].Coherence != 0 {
		t.Errorf("candidate 1 was rejected by the model, got %+v", got[0])
	}
	if got[1].Statement != "every service is deployed on the same VPS" || got[1].Coherence != 0.9 {
		t.Errorf("candidate 2 not mapped: %+v", got[1])
	}
}

// A candidate the model silently skips is a rejection, not a reason to fail the
// whole run: the caller records it and moves on.
func TestReasonShapesTreatsMissingCandidateAsRejection(t *testing.T) {
	srv, _ := triageServer(t, `[{"candidate": 1, "statement": "every client uses the http-first transport", "coherence": 1.0}]`)
	r := newTriage(t, srv.URL)

	got, err := r.ReasonShapes(context.Background(), shapeCandidates())
	if err != nil {
		t.Fatalf("ReasonShapes: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want two results, got %d", len(got))
	}
	if got[1].Coherence != 0 || got[1].Statement != "" {
		t.Errorf("missing candidate should be a rejection, got %+v", got[1])
	}
}

// Naming a candidate that was never asked about means the answer is not about
// this prompt, so retrying and finally failing is correct.
func TestReasonShapesFailsOnUnknownCandidate(t *testing.T) {
	srv, _ := triageServer(t, `[{"candidate": 9, "statement": "something", "coherence": 1.0}]`)
	r := newTriage(t, srv.URL)

	if _, err := r.ReasonShapes(context.Background(), shapeCandidates()); err == nil {
		t.Fatal("want an error for an answer that names an unknown candidate")
	}
}
