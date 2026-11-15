// Modifications copyright (c) 2026 Bojan van der Heide
package llmreg

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Bojanni050/cortexiq/internal/db"
)

// The manual price table is written from the dashboard and read on every refresh,
// so two of its rules need a real database: the case-folded unique key, and the
// check that refuses a row that would price a call at nothing.
// Skipped unless CORTEXIQ_TEST_DSN is set, like the other integration tests.
func TestIntegrationManualPrices(t *testing.T) {
	dsn := os.Getenv("CORTEXIQ_TEST_DSN")
	if dsn == "" {
		t.Skip("CORTEXIQ_TEST_DSN not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dsn, "fake", 32)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer pool.Close()

	store := NewStore(pool)
	model := fmt.Sprintf("test/manual-%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = store.DeleteManualPrice(context.Background(), model) })

	saved, err := store.SetManualPrice(ctx, model, 0.5, 1.5)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.Model != model || saved.InputPerMTok != 0.5 || saved.OutputPerMTok != 1.5 {
		t.Fatalf("saved = %+v", saved)
	}

	// A second spelling that only differs in case replaces the row instead of
	// adding one: the cost lookup folds case, so two rows would fight.
	if _, err := store.SetManualPrice(ctx, upperFirst(model), 2, 3); err != nil {
		t.Fatalf("save with different case: %v", err)
	}
	list, err := store.ManualPrices(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	rows := 0
	for _, p := range list {
		if p.Model == model || p.Model == upperFirst(model) {
			rows++
			if p.InputPerMTok != 2 || p.OutputPerMTok != 3 {
				t.Errorf("price = %+v, want the second save", p)
			}
		}
	}
	if rows != 1 {
		t.Fatalf("%d rows for the same model, want 1", rows)
	}

	// Two zero rates would look like a free model.
	if _, err := store.SetManualPrice(ctx, model, 0, 0); err == nil {
		t.Error("a row with two zero rates was accepted")
	}

	if err := store.DeleteManualPrice(ctx, upperFirst(model)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// Removing a model that is gone is not an error: the dashboard's remove
	// button must be safe to press twice.
	if err := store.DeleteManualPrice(ctx, model); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

// upperFirst changes only the case of the model name's first letter, so the two
// spellings must collide on the folded key.
func upperFirst(s string) string {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}
