// Modifications copyright (c) 2026 Bojan van der Heide
// Package reasoner synthesizes structured reasoning over text.
package reasoner

import (
	"context"

	"github.com/Bojanni050/cortexiq/internal/models"
)

// StructuredFact represents an extracted fact with entity, property, and value.
type StructuredFact struct {
	Entity   string
	Property string
	Value    string
	Summary  string
}

// StructuredRelationship represents an extracted relationship between two entities.
type StructuredRelationship struct {
	FromEntity   string
	RelationType string
	// CanonicalType is the relation type mapped onto RelationVocabulary, or "" when
	// there is no confident mapping (see CanonicalRelationType).
	CanonicalType string
	ToEntity      string
	Confidence    float32
}

// StructuredPattern represents an abstract pattern derived from facts and relationships.
type StructuredPattern struct {
	Content        string
	CoherenceScore float32
	SourceFactIDs  []int64
	SourceRelIDs   []int64
}

// ContradictionClassification is the result of classifying a fact pair.
type ContradictionClassification string

const (
	ClassificationReplacement   ContradictionClassification = "replacement"
	ClassificationContradiction ContradictionClassification = "contradiction"
	ClassificationCompatible    ContradictionClassification = "compatible"
)

// ContradictionResult is the LLM output for classifying two facts about the same entity+property.
type ContradictionResult struct {
	Classification ContradictionClassification
	Confidence     float32
	Explanation    string
}

// StructuredCausalLink represents an extracted cause-effect relationship between two facts.
type StructuredCausalLink struct {
	CauseFactID  int64
	EffectFactID int64
	Confidence   float32
}

// GoalProgressAssessment is the LLM output for one goal against a batch of facts.
type GoalProgressAssessment struct {
	GoalID     int64
	Assessment string // "progress", "suggested_complete", "contradicted", "irrelevant"
	Note       string
	Confidence float32
}

// FailurePatternResult covers repetition detection and pattern extraction.
type FailurePatternResult struct {
	Type        string // "repetition" or "pattern"
	FailureID   int64  // For repetition: the original failure ID
	Evidence    string // For repetition: what evidence suggests the repeat
	PatternFact string // For pattern: the extracted higher-order fact content
	Confidence  float32
}

// HypothesisEvidenceResult is the LLM output for one hypothesis against a batch of facts.
type HypothesisEvidenceResult struct {
	HypothesisID  int64
	Verdict       string // "supports", "weakens", "contradicts", "irrelevant"
	Confidence    float32
	Reasoning     string
	NewConfidence float32
}

// CounterHypothesis is the opposing belief a critic model formulates for an
// existing hypothesis: what would be true instead, and how to tell the two
// apart. The plan matters more than the claim — a counter you cannot
// distinguish from the original is not a test, it is a second opinion.
type CounterHypothesis struct {
	Content          string
	VerificationPlan string
	Confidence       float32
	Reasoning        string
}

// ProposedHypothesis is a belief the consolidator thinks the stored facts imply
// but do not state. It is a proposal, not a conclusion: nothing about it is
// trusted until the critic has argued against it and evidence has been weighed.
type ProposedHypothesis struct {
	Content          string
	VerificationPlan string
	Confidence       float32
	Reasoning        string
	SourceFactIDs    []int64
}

// ChatMessage is one message in a chat conversation.
type ChatMessage struct {
	Role    string // "system", "user", or "assistant"
	Content string
}

// ShapeCandidate is one regularity candidate for ReasonShapes: the same relation
// type pointing at the same target from several distinct sources. It is found by
// grouping relationships, not by reading a window of consecutive memories, which
// is why it can catch recurrence that spreads over weeks.
type ShapeCandidate struct {
	RelationType string
	Target       string
	Sources      []string // distinct from_entity values: the proof of recurrence
	Samples      []string // a few statements behind the edges, for vocabulary
}

// ShapeStatement is the model's answer for one candidate. An empty Statement, or
// a zero Coherence, is a rejection: the grouping was an artifact of extraction
// rather than a behaviour the sources share.
type ShapeStatement struct {
	Statement string
	Coherence float32 // 0.0 = rejected, 1.0 = every source fits without exception
}

// Reasoner synthesizes structured reasoning over text input.
type Reasoner interface {
	// Chat performs a free-form chat completion. Unlike the structured Reason*
	// methods, this returns natural language: it is what reflect uses to
	// synthesize an answer from recalled memories. The messages follow the
	// standard chat format (system/user/assistant roles).
	Chat(ctx context.Context, messages []ChatMessage) (string, error)

	// ReasonStructured synthesizes the facts that the given texts state. Each distinct
	// claim or topic becomes its own fact, so a note about two things yields two facts
	// instead of losing one. It returns no facts when the texts hold nothing durable.
	// subject names what the texts are about (the namespace name, never free text); it
	// is used only to name the subject in the summaries, so every fact is
	// understandable on its own. It may be empty.
	ReasonStructured(ctx context.Context, texts []string, subject string) ([]*StructuredFact, error)

	// ReasonSameFact reports whether candidate adds nothing to existing: a rewording
	// or translation of the same information is "same"; a different or extra value,
	// date, number, name or detail is not.
	ReasonSameFact(ctx context.Context, existing, candidate string) (bool, error)

	// ReasonRelationshipsBatch extracts relationships for several facts in one call.
	// The result maps fact ID -> relationships; facts without any are absent.
	// Every relationship is grounded in the content of the fact it is attributed to.
	ReasonRelationshipsBatch(ctx context.Context, facts []models.Fact) (map[int64][]*StructuredRelationship, error)

	// ReasonPatterns takes facts and relationships and extracts abstract patterns.
	ReasonPatterns(ctx context.Context, facts []models.Fact, relationships []models.Relationship) ([]*StructuredPattern, error)

	// ReasonShapes phrases or rejects regularities the database already found.
	// Each candidate is a relation type pointing at one target from several
	// distinct sources; the grouping is the evidence, so the model only has to
	// say whether those sources genuinely share the behaviour and, if so, to
	// state it. A zero Coherence means "not a recurrence". The result has one
	// entry per candidate, in the same order.
	ReasonShapes(ctx context.Context, candidates []ShapeCandidate) ([]*ShapeStatement, error)

	// ReasonContradiction classifies whether a new fact replaces, contradicts, or is compatible with an old fact.
	ReasonContradiction(ctx context.Context, entity, property, oldValue, newValue string) (*ContradictionResult, error)

	// ReasonCausalLinks takes a batch of facts and extracts cause-effect pairs.
	ReasonCausalLinks(ctx context.Context, facts []models.Fact) ([]*StructuredCausalLink, error)

	// ReasonGoalProgress assesses whether recent facts indicate progress, completion, or contradiction of active goals.
	ReasonGoalProgress(ctx context.Context, goals []models.Goal, facts []models.Fact) ([]*GoalProgressAssessment, error)

	// ReasonFailurePatterns detects whether recent evidence repeats past failures, and extracts higher-order failure patterns.
	ReasonFailurePatterns(ctx context.Context, failures []models.Failure, evidence []string) ([]*FailurePatternResult, error)

	// ReasonHypothesisEvidence assesses whether new evidence supports, weakens, or contradicts open hypotheses.
	ReasonHypothesisEvidence(ctx context.Context, hypotheses []models.Hypothesis, facts []models.Fact) ([]*HypothesisEvidenceResult, error)

	// ProposeCounterHypothesis formulates the strongest belief that would be
	// true instead of the given one, together with a test that separates them.
	// It is deliberately adversarial: agreeing with the input is a failure.
	ProposeCounterHypothesis(ctx context.Context, h models.Hypothesis, evidence []models.Fact) (*CounterHypothesis, error)

	// ProposeHypotheses looks for a testable belief implied by the facts that
	// none of the existing hypotheses already covers. It is allowed to return
	// nothing: most batches of facts contain no real uncertainty, and a made-up
	// belief is worse than an empty answer because it will be tested as if a
	// human had asserted it.
	ProposeHypotheses(ctx context.Context, facts []models.Fact, existing []string, max int) ([]*ProposedHypothesis, error)
}
