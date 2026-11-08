// Modifications copyright (c) 2026 Bojan van der Heide
package brain

import (
	"errors"
	"strings"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/Bojanni050/cortexiq/internal/reasoner"
)

// A goal may be closed from its own namespace or any descendant, so a session in
// /projects/x can close a goal filed at /projects. Anything else is out of reach.
func TestNamespaceCovers(t *testing.T) {
	cases := []struct {
		ns, slug string
		want     bool
	}{
		{"/", "/projects/x", true},
		{"/projects", "/projects", true},
		{"/projects", "/projects/x", true},
		{"/projects", "/projects-x", false},
		{"/projects/x", "/projects", false},
		{"/projects/x", "/projects/y", false},
	}
	for _, c := range cases {
		if got := namespaceCovers(c.ns, c.slug); got != c.want {
			t.Errorf("namespaceCovers(%q, %q) = %v, want %v", c.ns, c.slug, got, c.want)
		}
	}
}

// The consolidator's inference belongs in the log, not in the notes: notes is
// text the user wrote, and a suggestion appended to it can be neither queried nor
// taken back. A repeat run over the same facts must not add a row for a
// conclusion the log already holds.
func TestIntegrationConsolidationLogsGoalProgressWithoutTouchingNotes(t *testing.T) {
	e := newEnv(t, nil)

	goal, err := e.b.CreateGoal(e.ctx, e.nsID, "ship tags_any", nil, 1)
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	fact := e.insertFact(t, "tags_any is merged and released", nil)

	e.r.goalAssess = []*reasoner.GoalProgressAssessment{{
		GoalID: goal.ID, Assessment: "suggested_complete",
		Note: "the release is out", Confidence: 0.9,
	}}

	cp := &models.ConsolidationProgress{}
	annotated, suggested, _, errs := e.b.consolidateGoalProgress(e.ctx, e.nsID, cp)
	if len(errs) != 0 {
		t.Fatalf("consolidateGoalProgress errors: %v", errs)
	}
	if annotated != 1 || suggested != 1 {
		t.Errorf("counters = annotated %d, suggested %d; want 1 and 1", annotated, suggested)
	}

	events, err := e.b.ListGoalEvents(e.ctx, []string{e.ns}, goal.ID, Pagination{Limit: 10})
	if err != nil {
		t.Fatalf("ListGoalEvents: %v", err)
	}
	// The log is newest first and the creation event is still there.
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (created + suggested_complete)", len(events))
	}
	ev := events[0]
	if ev.Event != GoalEventSuggestedComplete {
		t.Errorf("newest event = %q, want %q", ev.Event, GoalEventSuggestedComplete)
	}
	if ev.Note == nil || *ev.Note != "the release is out" {
		t.Errorf("note = %v, want the model's reasoning", ev.Note)
	}
	if ev.Confidence == nil || *ev.Confidence != 0.9 {
		t.Errorf("confidence = %v, want 0.9", ev.Confidence)
	}
	if ev.FactID == nil || *ev.FactID != fact {
		t.Errorf("fact_id = %v, want %d (the fact the inference rested on)", ev.FactID, fact)
	}
	if ev.Actor == "" {
		t.Error("event has no actor")
	}
	if ev.GoalContent != "ship tags_any" {
		t.Errorf("goal_content = %q, want the joined content", ev.GoalContent)
	}

	g, err := e.b.GetGoal(e.ctx, goal.ID)
	if err != nil {
		t.Fatalf("GetGoal: %v", err)
	}
	if strings.Contains(g.Notes, "SUGGESTED COMPLETE") || strings.Contains(g.Notes, "[PROGRESS]") {
		t.Errorf("notes still carries the inference: %q", g.Notes)
	}
	if g.Status != "active" {
		t.Errorf("consolidation changed the status to %q; it may only suggest", g.Status)
	}
	if g.LatestEvent == nil || g.LatestEvent.Event != GoalEventSuggestedComplete {
		t.Errorf("latest_event = %+v, want the suggested completion", g.LatestEvent)
	}

	// A run that fails after this stage never saves its checkpoint, so the next
	// one reads the same facts again. That is not a new observation.
	cp.LastGoalProgressFactID = 0
	annotated, suggested, _, errs = e.b.consolidateGoalProgress(e.ctx, e.nsID, cp)
	if len(errs) != 0 {
		t.Fatalf("repeat consolidateGoalProgress errors: %v", errs)
	}
	if annotated != 0 || suggested != 0 {
		t.Errorf("repeat run counted annotated %d, suggested %d; want 0 and 0", annotated, suggested)
	}
	again, err := e.b.ListGoalEvents(e.ctx, []string{e.ns}, goal.ID, Pagination{Limit: 10})
	if err != nil {
		t.Fatalf("ListGoalEvents after repeat: %v", err)
	}
	if len(again) != len(events) {
		t.Errorf("repeat run added %d rows for the same conclusion", len(again)-len(events))
	}
}

// remember with complete_goal is the session-end trigger: the memory is written
// and the goal moves with it, and the log says which memory closed it.
func TestIntegrationRememberCompletesGoal(t *testing.T) {
	e := newEnv(t, nil)

	goal, err := e.b.CreateGoal(e.ctx, e.nsID, "ship the goal log", nil, 1)
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}

	out, err := e.b.RememberWithOutcome(e.ctx, e.ns, "the goal log is merged and released", RememberOptions{
		CompleteGoal: &goal.ID,
	})
	if err != nil {
		t.Fatalf("RememberWithOutcome: %v", err)
	}

	g, err := e.b.GetGoal(e.ctx, goal.ID)
	if err != nil {
		t.Fatalf("GetGoal: %v", err)
	}
	if g.Status != "completed" {
		t.Errorf("status = %q, want completed", g.Status)
	}
	if g.CompletedAt == nil {
		t.Error("completed_at is empty on a completed goal")
	}
	if g.LatestEvent == nil || g.LatestEvent.Event != GoalEventCompleted {
		t.Fatalf("latest_event = %+v, want a completion", g.LatestEvent)
	}
	if g.LatestEvent.EpisodeID == nil || *g.LatestEvent.EpisodeID != out.ID {
		t.Errorf("episode_id = %v, want %d (the memory that closed it)", g.LatestEvent.EpisodeID, out.ID)
	}

	// A closed goal cannot be closed twice, and the refusal comes before the
	// write: no second episode, no second event.
	if _, err := e.b.RememberWithOutcome(e.ctx, e.ns, "the goal log is merged again", RememberOptions{
		CompleteGoal: &goal.ID,
	}); !errors.Is(err, ErrGoalNotActive) {
		t.Errorf("second completion error = %v, want ErrGoalNotActive", err)
	}
	var episodes int
	if err := e.b.pool.QueryRow(e.ctx,
		`SELECT COUNT(*) FROM episodes WHERE namespace_id = $1 AND deleted_at IS NULL`, e.nsID,
	).Scan(&episodes); err != nil {
		t.Fatalf("count episodes: %v", err)
	}
	if episodes != 1 {
		t.Errorf("episodes = %d, want 1: the refused write must store nothing", episodes)
	}

	// Another namespace cannot close this goal: the id is not a capability.
	other := e.ns + "-other"
	if _, err := e.b.CreateNamespace(e.ctx, other, other, ""); err != nil {
		t.Fatalf("CreateNamespace(%q): %v", other, err)
	}
	otherGoal, err := e.b.CreateGoal(e.ctx, e.nsID, "ship the goal log too", nil, 1)
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	if _, err := e.b.RememberWithOutcome(e.ctx, other, "a memory far away", RememberOptions{
		CompleteGoal: &otherGoal.ID,
	}); err == nil {
		t.Error("a write in an unrelated namespace closed a goal")
	}
}
