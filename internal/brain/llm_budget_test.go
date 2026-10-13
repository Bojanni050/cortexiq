package brain

import (
	"context"
	"testing"
)

func TestLLMBudget(t *testing.T) {
	ctx := withLLMBudget(context.Background(), 2)
	if err := consumeLLMCall(ctx); err != nil {
		t.Fatal(err)
	}
	if err := consumeLLMCall(ctx); err != nil {
		t.Fatal(err)
	}
	if err := consumeLLMCall(ctx); err == nil {
		t.Fatal("expected the third call to exceed the budget")
	}
	if !llmBudgetExhausted(ctx) {
		t.Fatal("expected the budget to be exhausted")
	}
}

func TestLLMBudgetUnlimited(t *testing.T) {
	ctx := withLLMBudget(context.Background(), 0)
	for i := 0; i < 100; i++ {
		if err := consumeLLMCall(ctx); err != nil {
			t.Fatalf("unlimited budget rejected call %d: %v", i, err)
		}
	}
	if llmBudgetExhausted(ctx) {
		t.Fatal("unlimited budget should not be exhausted")
	}
}
