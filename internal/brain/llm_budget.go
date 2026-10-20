package brain

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/Bojanni050/cortexiq/internal/reasoner"
)

type llmBudgetKey struct{}

type llmBudget struct {
	remaining atomic.Int64
	limit     int
}

// withLLMBudget attaches a per-consolidation reasoner-call budget to ctx.
// A zero or negative limit means unlimited.
func withLLMBudget(ctx context.Context, limit int) context.Context {
	if limit <= 0 {
		return ctx
	}
	b := &llmBudget{limit: limit}
	b.remaining.Store(int64(limit))
	return context.WithValue(ctx, llmBudgetKey{}, b)
}

func consumeLLMCall(ctx context.Context) error {
	b, _ := ctx.Value(llmBudgetKey{}).(*llmBudget)
	if b == nil {
		return nil
	}
	for {
		remaining := b.remaining.Load()
		if remaining <= 0 {
			return fmt.Errorf("reasoner call budget exhausted (limit %d)", b.limit)
		}
		if b.remaining.CompareAndSwap(remaining, remaining-1) {
			return nil
		}
	}
}

func llmBudgetExhausted(ctx context.Context) bool {
	b, _ := ctx.Value(llmBudgetKey{}).(*llmBudget)
	return b != nil && b.remaining.Load() <= 0
}

// budgetReasoner enforces the per-run limit without changing the Reasoner API.
// Calls outside a consolidation context remain unlimited.
type budgetReasoner struct{ inner reasoner.Reasoner }

func (r budgetReasoner) Chat(ctx context.Context, messages []reasoner.ChatMessage) (string, error) {
	if err := consumeLLMCall(ctx); err != nil {
		return "", err
	}
	return r.inner.Chat(ctx, messages)
}

func (r budgetReasoner) ReasonStructured(ctx context.Context, texts []string, subject string) ([]*reasoner.StructuredFact, error) {
	if err := consumeLLMCall(ctx); err != nil {
		return nil, err
	}
	return r.inner.ReasonStructured(ctx, texts, subject)
}

func (r budgetReasoner) ReasonSameFact(ctx context.Context, existing, candidate string) (bool, error) {
	if err := consumeLLMCall(ctx); err != nil {
		return false, err
	}
	return r.inner.ReasonSameFact(ctx, existing, candidate)
}

func (r budgetReasoner) ReasonRelationshipsBatch(ctx context.Context, facts []models.Fact) (map[int64][]*reasoner.StructuredRelationship, error) {
	if err := consumeLLMCall(ctx); err != nil {
		return nil, err
	}
	return r.inner.ReasonRelationshipsBatch(ctx, facts)
}

func (r budgetReasoner) ReasonPatterns(ctx context.Context, facts []models.Fact, relationships []models.Relationship) ([]*reasoner.StructuredPattern, error) {
	if err := consumeLLMCall(ctx); err != nil {
		return nil, err
	}
	return r.inner.ReasonPatterns(ctx, facts, relationships)
}

func (r budgetReasoner) ReasonContradiction(ctx context.Context, entity, property, oldValue, newValue string) (*reasoner.ContradictionResult, error) {
	if err := consumeLLMCall(ctx); err != nil {
		return nil, err
	}
	return r.inner.ReasonContradiction(ctx, entity, property, oldValue, newValue)
}

func (r budgetReasoner) ReasonCausalLinks(ctx context.Context, facts []models.Fact) ([]*reasoner.StructuredCausalLink, error) {
	if err := consumeLLMCall(ctx); err != nil {
		return nil, err
	}
	return r.inner.ReasonCausalLinks(ctx, facts)
}

func (r budgetReasoner) ReasonGoalProgress(ctx context.Context, goals []models.Goal, facts []models.Fact) ([]*reasoner.GoalProgressAssessment, error) {
	if err := consumeLLMCall(ctx); err != nil {
		return nil, err
	}
	return r.inner.ReasonGoalProgress(ctx, goals, facts)
}

func (r budgetReasoner) ReasonFailurePatterns(ctx context.Context, failures []models.Failure, evidence []string) ([]*reasoner.FailurePatternResult, error) {
	if err := consumeLLMCall(ctx); err != nil {
		return nil, err
	}
	return r.inner.ReasonFailurePatterns(ctx, failures, evidence)
}

func (r budgetReasoner) ReasonHypothesisEvidence(ctx context.Context, hypotheses []models.Hypothesis, facts []models.Fact) ([]*reasoner.HypothesisEvidenceResult, error) {
	if err := consumeLLMCall(ctx); err != nil {
		return nil, err
	}
	return r.inner.ReasonHypothesisEvidence(ctx, hypotheses, facts)
}
