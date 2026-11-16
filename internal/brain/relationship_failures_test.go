// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import "testing"

// The relationship stage needs a count of failed runs per batch to stop an
// unreadable answer from blocking a namespace forever. A zero Brain must work:
// the map is created on first use, and a test environment never calls New.
func TestRelationshipFailuresCountPerBatch(t *testing.T) {
	var b Brain
	if got := b.noteRelationshipFailure(1, 10); got != 1 {
		t.Fatalf("first failure = %d, want 1", got)
	}
	if got := b.noteRelationshipFailure(1, 10); got != 2 {
		t.Fatalf("second failure = %d, want 2", got)
	}
	if got := b.noteRelationshipFailure(1, 20); got != 1 {
		t.Fatalf("another batch must count on its own, got %d", got)
	}
	if got := b.noteRelationshipFailure(2, 10); got != 1 {
		t.Fatalf("another namespace must count on its own, got %d", got)
	}

	b.clearRelationshipFailure(1, 10)
	if got := b.noteRelationshipFailure(1, 10); got != 1 {
		t.Fatalf("after a successful batch the count restarts, got %d", got)
	}
}

func TestPruneRelationshipFailuresKeepsOnlyTheLeadingBatch(t *testing.T) {
	var b Brain
	b.noteRelationshipFailure(1, 10)
	b.noteRelationshipFailure(1, 20)
	b.noteRelationshipFailure(2, 30)

	b.pruneRelationshipFailures(1, 20)

	if got := b.noteRelationshipFailure(1, 10); got != 1 {
		t.Errorf("a pruned batch must start from zero, got %d", got)
	}
	if got := b.noteRelationshipFailure(1, 20); got != 2 {
		t.Errorf("the leading batch keeps its count, got %d", got)
	}
	if got := b.noteRelationshipFailure(2, 30); got != 2 {
		t.Errorf("another namespace keeps its count, got %d", got)
	}
}
