// Copyright (c) 2026 Bojan van der Heide
package brain

import (
	"slices"
	"testing"
	"time"

	"github.com/Bojanni050/cortexiq/internal/models"
)

func ptr[T any](v T) *T { return &v }

func TestMentalModelSettingsApply(t *testing.T) {
	base := defaultMentalModelSettings()

	got, err := base.apply(MentalModelSettingsPatch{
		AutoRefresh:               ptr(false),
		MinRefreshIntervalSeconds: ptr(0),
		SourceTags:                &[]string{"Bug Fix", "bug-fix"},
		SourceTypes:               &[]string{"Goal", "fact", "fact", ""},
		SourceLimit:               ptr(40),
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got.AutoRefresh || got.MinRefreshIntervalSeconds == nil || *got.MinRefreshIntervalSeconds != 0 {
		t.Errorf("refresh settings = %v / %v", got.AutoRefresh, got.MinRefreshIntervalSeconds)
	}
	if !slices.Equal(got.SourceTags, []string{"bug-fix"}) || !slices.Equal(got.SourceTypes, []string{"fact", "goal"}) || got.SourceLimit != 40 {
		t.Errorf("source settings = %v %v %d", got.SourceTags, got.SourceTypes, got.SourceLimit)
	}

	// An empty patch changes nothing; the restore values go back to the default.
	same, err := got.apply(MentalModelSettingsPatch{})
	if err != nil || same.SourceLimit != 40 || same.AutoRefresh {
		t.Errorf("empty patch changed the settings: %+v (%v)", same, err)
	}
	reset, err := got.apply(MentalModelSettingsPatch{
		MinRefreshIntervalSeconds: ptr(-1),
		SourceTags:                &[]string{},
		SourceTypes:               &[]string{},
		SourceLimit:               ptr(0),
	})
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if reset.MinRefreshIntervalSeconds != nil || reset.SourceTags == nil || len(reset.SourceTags) != 0 ||
		reset.SourceTypes == nil || len(reset.SourceTypes) != 0 || reset.SourceLimit != defaultMentalModelSourceLimit {
		t.Errorf("reset = %+v", reset)
	}

	for name, bad := range map[string]MentalModelSettingsPatch{
		"unknown type":      {SourceTypes: &[]string{"observation"}},
		"limit too high":    {SourceLimit: ptr(maxMentalModelSourceLimit + 1)},
		"interval too long": {MinRefreshIntervalSeconds: ptr(maxMentalModelRefreshIntervalSeconds + 1)},
	} {
		if _, err := base.apply(bad); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestMentalModelReflectOptionsSkipPatternsUnderAFilter(t *testing.T) {
	plain := defaultMentalModelSettings().reflectOptions([]string{"/"}, 2048)
	if plain.SkipPatterns || plain.Limit != 20 || plain.MaxTokens != 2048 {
		t.Errorf("default options = %+v", plain)
	}
	filtered := defaultMentalModelSettings()
	filtered.SourceTags = []string{"decision"}
	if !filtered.reflectOptions([]string{"/"}, 0).SkipPatterns {
		t.Error("a source filter must keep unfiltered patterns out")
	}
}

func TestAutomaticRefreshDueUsesTheModelInterval(t *testing.T) {
	now := time.Now()
	m := models.MentalModel{LastRefreshedAt: ptr(now.Add(-10 * time.Minute))}
	if automaticRefreshDue(m, now) {
		t.Error("ten minutes is inside the default hour")
	}
	m.MinRefreshIntervalSeconds = ptr(300)
	if !automaticRefreshDue(m, now) {
		t.Error("ten minutes is past a five minute interval")
	}
}

func TestIntegrationMentalModelSettingsAndHistory(t *testing.T) {
	e := newEnv(t, nil)
	factID := e.insertFact(t, "the kafka cluster runs three brokers", nil)
	e.r.chatAnswer = "first version"

	mm, err := e.b.CreateMentalModel(e.ctx, CreateMentalModelOptions{
		NamespaceID: &e.nsID,
		Name:        "Kafka",
		SourceQuery: "What do we know about the kafka cluster?",
		Settings: MentalModelSettingsPatch{
			AutoRefresh: ptr(false),
			SourceTypes: &[]string{"fact"},
			SourceLimit: ptr(5),
		},
	})
	if err != nil {
		t.Fatalf("CreateMentalModel: %v", err)
	}
	if mm.AutoRefresh || mm.SourceLimit != 5 || !slices.Equal(mm.SourceTypes, []string{"fact"}) {
		t.Fatalf("settings not stored: %+v", mm)
	}

	// Switched off: a stale model that is long overdue stays as it is.
	time.Sleep(10 * time.Millisecond)
	e.insertFact(t, "the kafka cluster was upgraded to version 4", nil)
	if _, err := e.b.pool.Exec(e.ctx,
		`UPDATE mental_models SET last_refreshed_at = now() - interval '2 hours' WHERE id = $1`, mm.ID); err != nil {
		t.Fatalf("backdate model: %v", err)
	}
	summary, err := e.b.refreshStaleMentalModels(e.ctx, e.nsID)
	if err != nil || summary.Refreshed != 0 {
		t.Fatalf("auto_refresh=false must be skipped, got %+v (%v)", summary, err)
	}

	// An explicit refresh still works and keeps the replaced content.
	e.r.chatAnswer = "second version"
	if _, err := e.b.RefreshMentalModel(e.ctx, mm.ID); err != nil {
		t.Fatalf("RefreshMentalModel: %v", err)
	}
	versions, err := e.b.ListMentalModelVersions(e.ctx, mm.ID)
	if err != nil {
		t.Fatalf("ListMentalModelVersions: %v", err)
	}
	if len(versions) != 1 || versions[0].Content != "first version" || versions[0].ReplacedBy != "refresh" {
		t.Fatalf("versions after refresh = %+v", versions)
	}

	// A changed source setting makes the content stale; a rename does not.
	updated, err := e.b.UpdateMentalModel(e.ctx, mm.ID, UpdateMentalModelOptions{
		Name: "Kafka", SourceQuery: mm.SourceQuery, MaxTokens: mm.MaxTokens,
		Settings: MentalModelSettingsPatch{SourceLimit: ptr(8)},
	})
	if err != nil {
		t.Fatalf("UpdateMentalModel: %v", err)
	}
	if updated.AutoRefresh || !slices.Equal(updated.SourceTypes, []string{"fact"}) {
		t.Fatalf("update reset settings it did not set: %+v", updated)
	}
	got, err := e.b.GetMentalModel(e.ctx, mm.ID)
	if err != nil || got.StaleReason != "source_changed" {
		t.Fatalf("stale reason = %q (%v), want source_changed", got.StaleReason, err)
	}

	cleared, err := e.b.ClearMentalModel(e.ctx, mm.ID)
	if err != nil {
		t.Fatalf("ClearMentalModel: %v", err)
	}
	if cleared.Content != "" || cleared.LastRefreshedAt == nil {
		t.Fatalf("cleared model = %+v", cleared)
	}
	versions, err = e.b.ListMentalModelVersions(e.ctx, mm.ID)
	if err != nil || len(versions) != 2 || versions[0].ReplacedBy != "clear" {
		t.Fatalf("versions after clear = %+v (%v)", versions, err)
	}

	// A purge must not leave the erased memory readable in the history.
	if _, err := e.b.PurgeFact(e.ctx, factID, PurgeOptions{Reason: "test"}); err != nil {
		t.Fatalf("PurgeFact: %v", err)
	}
	versions, err = e.b.ListMentalModelVersions(e.ctx, mm.ID)
	if err != nil || len(versions) != 0 {
		t.Fatalf("versions after purge = %+v (%v), want none", versions, err)
	}
}
