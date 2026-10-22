// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"errors"
	"testing"
	"time"
)

// TestMentalModelCreateStoresGrounding checks that a new model records both the
// memories it was built from and the snapshot it read them at.
func TestMentalModelCreateStoresGrounding(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "the kafka cluster runs three brokers", nil)

	mm, err := e.b.CreateMentalModel(e.ctx, e.nsID, CreateMentalModelOptions{
		Name:        "Kafka",
		SourceQuery: "What do we know about the kafka cluster?",
	})
	if err != nil {
		t.Fatalf("CreateMentalModel: %v", err)
	}
	if mm.LastMemorySeenAt == nil {
		t.Fatal("last_memory_seen_at should be stamped")
	}
	if !equalIDs(mm.BasedOn.Facts, []int64{factID}) {
		t.Fatalf("based_on facts = %v, want [%d]", mm.BasedOn.Facts, factID)
	}
	if mm.LastRefreshFailedAt != nil {
		t.Fatalf("a fresh model should not be marked failed: %v", mm.LastRefreshFailedAt)
	}

	got, err := e.b.GetMentalModel(e.ctx, mm.ID)
	if err != nil {
		t.Fatalf("GetMentalModel: %v", err)
	}
	if got.IsStale {
		t.Fatalf("model should be fresh right after creation (reason %q)", got.StaleReason)
	}
}

// TestMentalModelCreateRefusesUngrounded checks that an empty namespace cannot
// produce a mental model that claims to know something.
func TestMentalModelCreateRefusesUngrounded(t *testing.T) {
	e := newEnv(t, nil)

	_, err := e.b.CreateMentalModel(e.ctx, e.nsID, CreateMentalModelOptions{
		Name:        "Empty",
		SourceQuery: "What do we know about anything?",
	})
	if !errors.Is(err, ErrMentalModelNotGrounded) {
		t.Fatalf("CreateMentalModel error = %v, want ErrMentalModelNotGrounded", err)
	}
}

// TestMentalModelStaleAfterNewMemory checks the watermark: a fact written after
// the snapshot makes the model stale, and the read explains why.
func TestMentalModelStaleAfterNewMemory(t *testing.T) {
	e := newEnv(t, nil)
	e.insertFact(t, "the kafka cluster runs three brokers", nil)

	mm, err := e.b.CreateMentalModel(e.ctx, e.nsID, CreateMentalModelOptions{
		Name:        "Kafka",
		SourceQuery: "What do we know about the kafka cluster?",
	})
	if err != nil {
		t.Fatalf("CreateMentalModel: %v", err)
	}

	// The watermark has microsecond precision; make the next write strictly later.
	time.Sleep(10 * time.Millisecond)
	e.insertFact(t, "the kafka cluster was upgraded to version 4", nil)

	got, err := e.b.GetMentalModel(e.ctx, mm.ID)
	if err != nil {
		t.Fatalf("GetMentalModel: %v", err)
	}
	if !got.IsStale {
		t.Fatal("model should be stale after a new fact")
	}
	if got.StaleReason != "new_memories" {
		t.Fatalf("stale reason = %q, want new_memories", got.StaleReason)
	}
	if got.LatestMemoryAt == nil {
		t.Fatal("latest_memory_at should be reported")
	}
}

// TestMentalModelStaleAfterDeletedGrounding checks the signal no watermark can
// raise: the memories a summary cites are gone, but nothing new was written.
func TestMentalModelStaleAfterDeletedGrounding(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "the kafka cluster runs three brokers", nil)

	mm, err := e.b.CreateMentalModel(e.ctx, e.nsID, CreateMentalModelOptions{
		Name:        "Kafka",
		SourceQuery: "What do we know about the kafka cluster?",
	})
	if err != nil {
		t.Fatalf("CreateMentalModel: %v", err)
	}

	if _, err := e.b.pool.Exec(e.ctx, `DELETE FROM facts WHERE id = $1`, factID); err != nil {
		t.Fatalf("delete fact: %v", err)
	}

	got, err := e.b.GetMentalModel(e.ctx, mm.ID)
	if err != nil {
		t.Fatalf("GetMentalModel: %v", err)
	}
	if !got.IsStale || got.StaleReason != "retracted_grounding" {
		t.Fatalf("stale = %v, reason = %q; want stale with retracted_grounding", got.IsStale, got.StaleReason)
	}
}

// TestMentalModelRefreshKeepsContentWhenUngrounded checks the guard: a refresh
// that finds no memories must not replace a good summary.
func TestMentalModelRefreshKeepsContentWhenUngrounded(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "the kafka cluster runs three brokers", nil)

	mm, err := e.b.CreateMentalModel(e.ctx, e.nsID, CreateMentalModelOptions{
		Name:        "Kafka",
		SourceQuery: "What do we know about the kafka cluster?",
	})
	if err != nil {
		t.Fatalf("CreateMentalModel: %v", err)
	}
	before := mm.Content
	beforeRefreshed := mm.LastRefreshedAt

	if _, err := e.b.pool.Exec(e.ctx, `DELETE FROM facts WHERE id = $1`, factID); err != nil {
		t.Fatalf("delete fact: %v", err)
	}

	_, err = e.b.RefreshMentalModel(e.ctx, mm.ID)
	if !errors.Is(err, ErrMentalModelNotGrounded) {
		t.Fatalf("RefreshMentalModel error = %v, want ErrMentalModelNotGrounded", err)
	}

	got, err := e.b.GetMentalModel(e.ctx, mm.ID)
	if err != nil {
		t.Fatalf("GetMentalModel: %v", err)
	}
	if got.Content != before {
		t.Fatalf("content changed after an ungrounded refresh:\n got %q\nwant %q", got.Content, before)
	}
	if got.LastRefreshedAt == nil || !got.LastRefreshedAt.Equal(*beforeRefreshed) {
		t.Fatal("last_refreshed_at should not move when the refresh is refused")
	}
	if got.LastRefreshFailedAt == nil {
		t.Fatal("last_refresh_failed_at should be stamped")
	}
	if got.LastRefreshError == "" {
		t.Fatal("last_refresh_error should explain the refusal")
	}

	// The automatic sweep must not retry a model that just failed.
	summary, err := e.b.refreshStaleMentalModels(e.ctx, e.nsID)
	if err != nil {
		t.Fatalf("refreshStaleMentalModels: %v", err)
	}
	if summary.Refreshed != 0 || summary.Failed != 0 {
		t.Fatalf("paused model should be skipped, got %+v", summary)
	}
}

// TestMentalModelRefreshClearsFailure checks the recovery path: a successful
// explicit refresh updates the watermark and clears the failure mark.
func TestMentalModelRefreshClearsFailure(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "the kafka cluster runs three brokers", nil)

	mm, err := e.b.CreateMentalModel(e.ctx, e.nsID, CreateMentalModelOptions{
		Name:        "Kafka",
		SourceQuery: "What do we know about the kafka cluster?",
	})
	if err != nil {
		t.Fatalf("CreateMentalModel: %v", err)
	}

	if _, err := e.b.pool.Exec(e.ctx, `DELETE FROM facts WHERE id = $1`, factID); err != nil {
		t.Fatalf("delete fact: %v", err)
	}
	if _, err := e.b.RefreshMentalModel(e.ctx, mm.ID); !errors.Is(err, ErrMentalModelNotGrounded) {
		t.Fatalf("expected the guard to refuse, got %v", err)
	}

	newFactID := e.insertFact(t, "the kafka cluster now has five brokers", nil)
	updated, err := e.b.RefreshMentalModel(e.ctx, mm.ID)
	if err != nil {
		t.Fatalf("RefreshMentalModel after new fact: %v", err)
	}
	if updated.LastRefreshFailedAt != nil || updated.LastRefreshError != "" {
		t.Fatalf("failure mark not cleared: %+v %q", updated.LastRefreshFailedAt, updated.LastRefreshError)
	}
	if !equalIDs(updated.BasedOn.Facts, []int64{newFactID}) {
		t.Fatalf("based_on facts = %v, want [%d]", updated.BasedOn.Facts, newFactID)
	}
	if updated.LastMemorySeenAt == nil || !updated.LastMemorySeenAt.After(*mm.LastMemorySeenAt) {
		t.Fatal("watermark should advance after a successful refresh")
	}

	got, err := e.b.GetMentalModel(e.ctx, mm.ID)
	if err != nil {
		t.Fatalf("GetMentalModel: %v", err)
	}
	if got.IsStale {
		t.Fatalf("model should be fresh after refresh (reason %q)", got.StaleReason)
	}
}

// TestRefreshStaleMentalModels checks the consolidation sweep end to end: nothing
// to do on a fresh model, one refresh after a new memory.
func TestRefreshStaleMentalModels(t *testing.T) {
	e := newEnv(t, nil)
	e.insertFact(t, "the kafka cluster runs three brokers", nil)

	mm, err := e.b.CreateMentalModel(e.ctx, e.nsID, CreateMentalModelOptions{
		Name:        "Kafka",
		SourceQuery: "What do we know about the kafka cluster?",
	})
	if err != nil {
		t.Fatalf("CreateMentalModel: %v", err)
	}

	summary, err := e.b.refreshStaleMentalModels(e.ctx, e.nsID)
	if err != nil {
		t.Fatalf("refreshStaleMentalModels: %v", err)
	}
	if summary.Refreshed != 0 {
		t.Fatalf("fresh model should be skipped, got %+v", summary)
	}

	time.Sleep(10 * time.Millisecond)
	e.insertFact(t, "the kafka cluster was upgraded to version 4", nil)

	summary, err = e.b.refreshStaleMentalModels(e.ctx, e.nsID)
	if err != nil {
		t.Fatalf("refreshStaleMentalModels: %v", err)
	}
	if summary.Refreshed != 1 || summary.Failed != 0 {
		t.Fatalf("expected one refresh, got %+v", summary)
	}

	got, err := e.b.GetMentalModel(e.ctx, mm.ID)
	if err != nil {
		t.Fatalf("GetMentalModel: %v", err)
	}
	if got.IsStale {
		t.Fatalf("model should be fresh after the sweep (reason %q)", got.StaleReason)
	}
}
