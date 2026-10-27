package brain

// Integration tests for the hygiene auditor and cleaner. Skipped unless
// CORTEXIQ_TEST_DSN is set, same as the rest of the integration suite.

import (
	"context"
	"testing"
)

func findCandidate(cs []HygieneCandidate, table string, id int64) *HygieneCandidate {
	for i := range cs {
		if cs[i].Table == table && cs[i].ID == id {
			return &cs[i]
		}
	}
	return nil
}

// The whole point of the audit is that a secret is reported even when its
// noise score is low, and that a dry run writes nothing.
func TestIntegrationHygieneAuditReportsSecretWithoutWriting(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "deploy key AKIAIOSFODNN7EXAMPLE hoort bij de bucket", nil)

	cs, err := e.b.Audit(e.ctx, HygieneAuditOptions{
		Namespaces: []string{e.ns},
		MinScore:   0,
		Tables:     []string{"facts"},
	})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	c := findCandidate(cs, "facts", factID)
	if c == nil {
		t.Fatalf("Audit returned %d candidates; want the fact with a secret", len(cs))
	}
	if len(c.SecretLabels) == 0 {
		t.Fatalf("candidate %+v has no secret labels; a secret must always be reported", c)
	}
	if c.Suggested == "" {
		t.Fatalf("candidate %+v has no suggested action", c)
	}

	res, err := e.b.Clean(e.ctx, cs, HygieneCleanOptions{Action: "archive"})
	if err != nil {
		t.Fatalf("Clean dry run: %v", err)
	}
	if !res.DryRun || res.Counts["would_archive"] != len(cs) {
		t.Fatalf("dry run = %+v; want dry_run with would_archive = %d", res, len(cs))
	}

	var archived bool
	var importance float64
	if err := e.b.pool.QueryRow(e.ctx, `SELECT hygiene_archived, importance FROM facts WHERE id = $1`, factID).Scan(&archived, &importance); err != nil {
		t.Fatal(err)
	}
	if archived || importance == 0 {
		t.Fatalf("dry run changed the fact (archived=%v importance=%v)", archived, importance)
	}

	st, err := e.b.HygieneStatus(e.ctx, []string{e.ns}, 10)
	if err != nil {
		t.Fatalf("HygieneStatus: %v", err)
	}
	if st.AuditLogCount != 0 {
		t.Fatalf("audit log has %d rows after a dry run; want 0", st.AuditLogCount)
	}
}

// Archive must be reversible: importance goes to 0, the original is kept, and
// Restore brings it back exactly.
func TestIntegrationHygieneArchiveAndRestore(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "password=hunter2 staat in het oude script", nil)

	cs, err := e.b.Audit(e.ctx, HygieneAuditOptions{
		Namespaces: []string{e.ns},
		MinScore:   0,
		Tables:     []string{"facts"},
	})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	c := findCandidate(cs, "facts", factID)
	if c == nil {
		t.Fatalf("Audit did not return fact %d", factID)
	}

	res, err := e.b.Clean(e.ctx, []HygieneCandidate{*c}, HygieneCleanOptions{Action: "archive", Confirm: true, Actor: "test"})
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if res.DryRun || res.Counts["archived"] != 1 {
		t.Fatalf("Clean = %+v; want one archived row", res)
	}

	var archived bool
	var importance float64
	var original *float64
	var reason string
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT hygiene_archived, importance, hygiene_original_importance, hygiene_archive_reason FROM facts WHERE id = $1`, factID,
	).Scan(&archived, &importance, &original, &reason); err != nil {
		t.Fatal(err)
	}
	if !archived || importance != 0 {
		t.Fatalf("after archive: archived=%v importance=%v; want archived with importance 0", archived, importance)
	}
	if original == nil || *original == 0 {
		t.Fatalf("original importance = %v; want the pre-archive value", original)
	}
	if reason == "" {
		t.Fatal("archive reason is empty; restore and review lose the why")
	}

	st, err := e.b.HygieneStatus(e.ctx, []string{e.ns}, 10)
	if err != nil {
		t.Fatalf("HygieneStatus: %v", err)
	}
	if st.AuditLogCount < 1 || st.ByAction["archived"] < 1 {
		t.Fatalf("status = %+v; want an archive entry in the audit log", st)
	}
	if st.ArchivedFacts < 1 {
		t.Fatalf("status archived facts = %d; want at least one", st.ArchivedFacts)
	}
	if len(st.RecentEntries) == 0 || st.RecentEntries[0].TargetID != factID || st.RecentEntries[0].Action != "archived" {
		t.Fatalf("recent entries = %+v; want the archived fact as the newest entry", st.RecentEntries)
	}

	n, err := e.b.Restore(e.ctx, []string{e.ns}, 10)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if n < 1 {
		t.Fatalf("Restore restored %d rows; want at least one", n)
	}
	var restoredImp float64
	if err := e.b.pool.QueryRow(e.ctx, `SELECT importance FROM facts WHERE id = $1`, factID).Scan(&restoredImp); err != nil {
		t.Fatal(err)
	}
	if restoredImp != *original {
		t.Fatalf("restored importance = %v; want %v", restoredImp, *original)
	}
	var stillArchived bool
	if err := e.b.pool.QueryRow(e.ctx, `SELECT hygiene_archived FROM facts WHERE id = $1`, factID).Scan(&stillArchived); err != nil {
		t.Fatal(err)
	}
	if stillArchived {
		t.Fatal("restored fact is still archived")
	}
}

// Flag marks for review without touching the memory itself.
func TestIntegrationHygieneFlagLeavesContentAndImportance(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "log dump: GET /health 200 GET /health 200 GET /health 200", nil)

	cs, err := e.b.Audit(e.ctx, HygieneAuditOptions{Namespaces: []string{e.ns}, MinScore: 0, Tables: []string{"facts"}})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	c := findCandidate(cs, "facts", factID)
	if c == nil {
		t.Fatalf("Audit did not return fact %d", factID)
	}

	if _, err := e.b.Clean(e.ctx, []HygieneCandidate{*c}, HygieneCleanOptions{Action: "flag", Confirm: true, Actor: "test"}); err != nil {
		t.Fatalf("Clean flag: %v", err)
	}

	var flagged bool
	var importance float64
	var reason string
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT hygiene_flagged, importance, COALESCE(hygiene_flag_reason, '') FROM facts WHERE id = $1`, factID,
	).Scan(&flagged, &importance, &reason); err != nil {
		t.Fatal(err)
	}
	if !flagged || reason == "" {
		t.Fatalf("flagged=%v reason=%q; want a flag with a reason", flagged, reason)
	}
	if importance == 0 {
		t.Fatalf("flag cleared importance; flagging must not archive")
	}

	st, err := e.b.HygieneStatus(e.ctx, []string{e.ns}, 10)
	if err != nil {
		t.Fatalf("HygieneStatus: %v", err)
	}
	if st.FlaggedFacts < 1 {
		t.Fatalf("status flagged facts = %d; want at least one", st.FlaggedFacts)
	}
}

func TestHygieneAuditRejectsUnknownTable(t *testing.T) {
	var b *Brain
	if _, err := b.Audit(context.Background(), HygieneAuditOptions{Tables: []string{"secrets"}}); err == nil {
		t.Fatal("Audit accepted an unknown table")
	}
}

func TestHygieneCleanRejectsUnknownAction(t *testing.T) {
	var b *Brain
	if _, err := b.Clean(context.Background(), nil, HygieneCleanOptions{Action: "delete"}); err == nil {
		t.Fatal("Clean accepted an unknown action")
	}
}

func TestSortCandidatesByScore(t *testing.T) {
	cs := []HygieneCandidate{
		{Table: "facts", ID: 2, NoiseScore: 0.4},
		{Table: "episodes", ID: 9, NoiseScore: 0.9},
		{Table: "facts", ID: 1, NoiseScore: 0.4},
	}
	sortCandidatesByScore(cs)
	if cs[0].ID != 9 || cs[1].ID != 2 || cs[2].ID != 1 {
		t.Fatalf("order = %+v; want worst score first and a stable tie order", cs)
	}
}
