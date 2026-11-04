// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pgvector/pgvector-go"
)

// TestMentalModelCreateStoresGrounding checks that a new model records both the
// memories it was built from and the snapshot it read them at.
func TestMentalModelCreateStoresGrounding(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "the kafka cluster runs three brokers", nil)

	mm, err := e.b.CreateMentalModel(e.ctx, CreateMentalModelOptions{
		NamespaceID: &e.nsID,
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

	_, err := e.b.CreateMentalModel(e.ctx, CreateMentalModelOptions{
		NamespaceID: &e.nsID,
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

	mm, err := e.b.CreateMentalModel(e.ctx, CreateMentalModelOptions{
		NamespaceID: &e.nsID,
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

	mm, err := e.b.CreateMentalModel(e.ctx, CreateMentalModelOptions{
		NamespaceID: &e.nsID,
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

	mm, err := e.b.CreateMentalModel(e.ctx, CreateMentalModelOptions{
		NamespaceID: &e.nsID,
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

	mm, err := e.b.CreateMentalModel(e.ctx, CreateMentalModelOptions{
		NamespaceID: &e.nsID,
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

// TestMentalModelStaleWithoutGrounding checks the upgrade path: a model from
// before based_on existed has content nobody can account for, so it is treated
// as stale once instead of being trusted forever.
func TestMentalModelStaleWithoutGrounding(t *testing.T) {
	e := newEnv(t, nil)
	e.insertFact(t, "the kafka cluster runs three brokers", nil)

	mm, err := e.b.CreateMentalModel(e.ctx, CreateMentalModelOptions{
		NamespaceID: &e.nsID,
		Name:        "Kafka",
		SourceQuery: "What do we know about the kafka cluster?",
	})
	if err != nil {
		t.Fatalf("CreateMentalModel: %v", err)
	}

	// Simulate a row written by the previous version: watermark present,
	// grounding absent.
	if _, err := e.b.pool.Exec(e.ctx,
		`UPDATE mental_models SET based_on = '{}'::jsonb WHERE id = $1`, mm.ID); err != nil {
		t.Fatalf("clear based_on: %v", err)
	}

	got, err := e.b.GetMentalModel(e.ctx, mm.ID)
	if err != nil {
		t.Fatalf("GetMentalModel: %v", err)
	}
	if !got.IsStale || got.StaleReason != "no_grounding" {
		t.Fatalf("stale = %v, reason = %q; want stale with no_grounding", got.IsStale, got.StaleReason)
	}
}

// TestRefreshStaleMentalModels checks the consolidation sweep end to end: nothing
// to do on a fresh model, one refresh after a new memory.
func TestRefreshStaleMentalModels(t *testing.T) {
	e := newEnv(t, nil)
	e.insertFact(t, "the kafka cluster runs three brokers", nil)

	mm, err := e.b.CreateMentalModel(e.ctx, CreateMentalModelOptions{
		NamespaceID: &e.nsID,
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
	// The sweep rate-limits itself to one refresh per hour; backdate the model so
	// this run is due.
	if _, err := e.b.pool.Exec(e.ctx,
		`UPDATE mental_models SET last_refreshed_at = now() - interval '2 hours' WHERE id = $1`, mm.ID); err != nil {
		t.Fatalf("backdate model: %v", err)
	}

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

// insertFactIn writes a fact in a namespace other than the test's own.
func (e *testEnv) insertFactIn(t *testing.T, nsID int64, content string) int64 {
	t.Helper()
	vec, _ := fakeEmbedder{}.Embed(e.ctx, content)
	var id int64
	err := e.b.pool.QueryRow(e.ctx,
		`INSERT INTO facts (namespace_id, content, embedding, embedding_model, confidence, valid_from)
		 VALUES ($1, $2, $3, 'fake', 0.8, now()) RETURNING id`,
		nsID, content, pgvector.NewVector(vec),
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert fact in namespace: %v", err)
	}
	return id
}

// TestGlobalMentalModelReadsAllNamespaces checks that a model without a
// namespace is built from memories elsewhere, is visible everywhere, and goes
// stale when anything changes.
func TestGlobalMentalModelReadsAllNamespaces(t *testing.T) {
	e := newEnv(t, nil)
	other := e.ns + "-other"
	otherID, err := e.b.CreateNamespace(e.ctx, other, "other", "")
	if err != nil {
		t.Fatalf("CreateNamespace(%q): %v", other, err)
	}
	otherFact := e.insertFactIn(t, otherID, "zebraqq migration uses kafkazz brokers")

	mm, err := e.b.CreateMentalModel(e.ctx, CreateMentalModelOptions{
		Name:        "Kafka",
		SourceQuery: "What do we know about zebraqq?",
	})
	if err != nil {
		t.Fatalf("CreateMentalModel: %v", err)
	}
	t.Cleanup(func() {
		_, _ = e.b.pool.Exec(context.Background(), `DELETE FROM mental_models WHERE id = $1`, mm.ID)
	})
	if mm.NamespaceID != nil {
		t.Fatalf("a model created without a namespace should be global, got namespace %d", *mm.NamespaceID)
	}
	if !containsID(mm.BasedOn.Facts, otherFact) {
		t.Fatalf("based_on facts = %v, want it to include [%d] from the sibling namespace", mm.BasedOn.Facts, otherFact)
	}

	// A reflect scoped to the empty namespace still finds the global model,
	// alongside any global models an earlier test left behind.
	results, err := e.b.SearchMentalModels(e.ctx, []string{e.ns}, "zebraqq", 20, nil)
	if err != nil {
		t.Fatalf("SearchMentalModels: %v", err)
	}
	found := false
	for _, r := range results {
		if r.ID == mm.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("global model %d not visible from %s", mm.ID, e.ns)
	}

	// A fact in yet another namespace makes it stale.
	time.Sleep(10 * time.Millisecond)
	e.insertFact(t, "a local zebraqq broker was added", nil)
	got, err := e.b.GetMentalModel(e.ctx, mm.ID)
	if err != nil {
		t.Fatalf("GetMentalModel: %v", err)
	}
	if !got.IsStale || got.StaleReason != "new_memories" {
		t.Fatalf("stale = %v, reason = %q; want new_memories", got.IsStale, got.StaleReason)
	}
}

// TestScopedMentalModelStaysInItsNamespace checks the other half: a model filed
// in a namespace is not visible elsewhere and ignores memories outside it.
func TestScopedMentalModelStaysInItsNamespace(t *testing.T) {
	e := newEnv(t, nil)
	e.insertFact(t, "the kafka cluster runs three brokers", nil)

	mm, err := e.b.CreateMentalModel(e.ctx, CreateMentalModelOptions{
		NamespaceID: &e.nsID,
		Name:        "Kafka",
		SourceQuery: "What do we know about the kafka cluster?",
	})
	if err != nil {
		t.Fatalf("CreateMentalModel: %v", err)
	}
	if mm.NamespaceID == nil || *mm.NamespaceID != e.nsID {
		t.Fatal("scoped model should keep its namespace")
	}

	other := e.ns + "-other"
	otherID, err := e.b.CreateNamespace(e.ctx, other, "other", "")
	if err != nil {
		t.Fatalf("CreateNamespace(%q): %v", other, err)
	}

	results, err := e.b.SearchMentalModels(e.ctx, []string{other}, "kafka cluster", 20, nil)
	if err != nil {
		t.Fatalf("SearchMentalModels: %v", err)
	}
	for _, r := range results {
		if r.ID == mm.ID {
			t.Fatalf("scoped model %d leaked into %s", mm.ID, other)
		}
	}

	time.Sleep(10 * time.Millisecond)
	e.insertFactIn(t, otherID, "a fact in the sibling namespace")
	got, err := e.b.GetMentalModel(e.ctx, mm.ID)
	if err != nil {
		t.Fatalf("GetMentalModel: %v", err)
	}
	if got.IsStale {
		t.Fatalf("a memory outside the model's namespace made it stale (%q)", got.StaleReason)
	}
}

// containsID reports whether id is in ids.
func containsID(ids []int64, id int64) bool {
	for _, got := range ids {
		if got == id {
			return true
		}
	}
	return false
}

// TestMentalModelQueuedGeneration checks the async split: the insert returns an
// empty row immediately, and RefreshMentalModel fills it.
func TestMentalModelQueuedGeneration(t *testing.T) {
	e := newEnv(t, nil)
	e.insertFact(t, "the kafka cluster runs three brokers", nil)

	m, err := e.b.CreateMentalModelQueued(e.ctx, CreateMentalModelOptions{
		Name:        "Kafka",
		SourceQuery: "What do we know about the kafka cluster?",
	})
	if err != nil {
		t.Fatalf("CreateMentalModelQueued: %v", err)
	}
	if m.Content != "" || m.LastRefreshedAt != nil || m.LastMemorySeenAt != nil {
		t.Fatalf("queued model should be empty and unrefreshed: %+v", m)
	}

	got, err := e.b.GetMentalModel(e.ctx, m.ID)
	if err != nil {
		t.Fatalf("GetMentalModel: %v", err)
	}
	if !got.IsStale || got.StaleReason != "never_refreshed" {
		t.Fatalf("queued model should read as never_refreshed, got %v/%q", got.IsStale, got.StaleReason)
	}

	updated, err := e.b.RefreshMentalModel(e.ctx, m.ID)
	if err != nil {
		t.Fatalf("RefreshMentalModel: %v", err)
	}
	if updated.Content == "" {
		t.Fatal("refresh should fill the queued model")
	}
	if updated.LastRefreshedAt == nil || updated.LastMemorySeenAt == nil {
		t.Fatal("refresh should stamp both clocks")
	}
}

// TestQueuedMentalModelRecoversOnSweep checks the crash path: a queued row whose
// background generation never ran is picked up by the consolidation sweep.
func TestQueuedMentalModelRecoversOnSweep(t *testing.T) {
	e := newEnv(t, nil)
	e.insertFact(t, "the kafka cluster runs three brokers", nil)

	m, err := e.b.CreateMentalModelQueued(e.ctx, CreateMentalModelOptions{
		NamespaceID: &e.nsID,
		Name:        "Kafka",
		SourceQuery: "What do we know about the kafka cluster?",
	})
	if err != nil {
		t.Fatalf("CreateMentalModelQueued: %v", err)
	}

	summary, err := e.b.refreshStaleMentalModels(e.ctx, e.nsID)
	if err != nil {
		t.Fatalf("refreshStaleMentalModels: %v", err)
	}
	if summary.Refreshed != 1 {
		t.Fatalf("queued model should be swept, got %+v", summary)
	}
	got, err := e.b.GetMentalModel(e.ctx, m.ID)
	if err != nil {
		t.Fatalf("GetMentalModel: %v", err)
	}
	if got.Content == "" || got.LastRefreshedAt == nil {
		t.Fatal("sweep should have generated the queued model")
	}
}

// TestUpdateMentalModelMarksSourceStale checks the edit path: a rename keeps
// the content valid, a new source query marks it stale, and a refresh records
// the query it answered and clears the mark.
func TestUpdateMentalModelMarksSourceStale(t *testing.T) {
	e := newEnv(t, nil)
	e.insertFact(t, "the kafka cluster runs three brokers", nil)

	mm, err := e.b.CreateMentalModel(e.ctx, CreateMentalModelOptions{
		NamespaceID: &e.nsID,
		Name:        "Kafka",
		SourceQuery: "What do we know about the kafka cluster?",
	})
	if err != nil {
		t.Fatalf("CreateMentalModel: %v", err)
	}
	before := mm.Content

	renamed, err := e.b.UpdateMentalModel(e.ctx, mm.ID, UpdateMentalModelOptions{
		Name:        "Kafka cluster",
		SourceQuery: mm.SourceQuery,
		Tags:        mm.Tags,
		MaxTokens:   mm.MaxTokens,
	})
	if err != nil {
		t.Fatalf("UpdateMentalModel rename: %v", err)
	}
	if renamed.Name != "Kafka cluster" {
		t.Fatalf("name = %q, want renamed", renamed.Name)
	}
	got, err := e.b.GetMentalModel(e.ctx, mm.ID)
	if err != nil {
		t.Fatalf("GetMentalModel: %v", err)
	}
	if got.IsStale {
		t.Fatalf("a rename should not invalidate the content (reason %q)", got.StaleReason)
	}

	edited, err := e.b.UpdateMentalModel(e.ctx, mm.ID, UpdateMentalModelOptions{
		Name:        renamed.Name,
		SourceQuery: "What do we know about the brokers?",
		Tags:        renamed.Tags,
		MaxTokens:   renamed.MaxTokens,
	})
	if err != nil {
		t.Fatalf("UpdateMentalModel query: %v", err)
	}
	if edited.Content != before {
		t.Fatal("editing the query should leave the stored content alone")
	}
	got, err = e.b.GetMentalModel(e.ctx, mm.ID)
	if err != nil {
		t.Fatalf("GetMentalModel: %v", err)
	}
	if !got.IsStale || got.StaleReason != "source_changed" {
		t.Fatalf("edited query should be stale with source_changed, got %v/%q", got.IsStale, got.StaleReason)
	}

	if _, err := e.b.RefreshMentalModel(e.ctx, mm.ID); err != nil {
		t.Fatalf("RefreshMentalModel: %v", err)
	}
	got, err = e.b.GetMentalModel(e.ctx, mm.ID)
	if err != nil {
		t.Fatalf("GetMentalModel: %v", err)
	}
	if got.IsStale {
		t.Fatalf("refresh should clear source_changed (reason %q)", got.StaleReason)
	}
	if got.LastRefreshedSourceQuery != got.SourceQuery {
		t.Fatalf("refresh should record the query it answered: %q vs %q", got.LastRefreshedSourceQuery, got.SourceQuery)
	}
}
