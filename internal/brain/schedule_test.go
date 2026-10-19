package brain

import (
	"testing"
	"time"

	"github.com/Bojanni050/cortexiq/internal/schedule"
)

func clearScheduleSetting(t *testing.T, e *testEnv) {
	t.Helper()
	if _, err := e.b.pool.Exec(e.ctx, `DELETE FROM settings WHERE key LIKE 'schedule.%'`); err != nil {
		t.Fatalf("clear settings: %v", err)
	}
	t.Cleanup(func() {
		_, _ = e.b.pool.Exec(e.ctx, `DELETE FROM settings WHERE key LIKE 'schedule.%'`)
	})
}

func TestIntegrationScheduleSettingRoundTrip(t *testing.T) {
	e := newEnv(t, nil)
	clearScheduleSetting(t, e)

	// Nothing stored yet: the old behaviour, interval every five minutes.
	cfg, err := e.b.ScheduleConfig(e.ctx)
	if err != nil {
		t.Fatalf("ScheduleConfig: %v", err)
	}
	if cfg != schedule.DefaultConfig() {
		t.Fatalf("empty setting = %+v, want %+v", cfg, schedule.DefaultConfig())
	}

	want := schedule.Config{Mode: schedule.ModeEvents, IntervalSeconds: 30}
	if err := e.b.SetScheduleConfig(e.ctx, want); err != nil {
		t.Fatalf("SetScheduleConfig: %v", err)
	}
	select {
	case <-e.b.ScheduleChanged():
	default:
		t.Fatal("storing a schedule did not wake the consolidator")
	}

	// A first start must not overwrite a choice already made in the dashboard.
	if err := e.b.SeedSchedule(e.ctx, schedule.DefaultConfig()); err != nil {
		t.Fatalf("SeedSchedule: %v", err)
	}
	if got, _ := e.b.ScheduleConfig(e.ctx); got != want {
		t.Fatalf("seed overwrote the stored schedule: %+v", got)
	}

	// Invalid values are refused before they reach the table.
	if err := e.b.SetScheduleConfig(e.ctx, schedule.Config{Mode: "sometimes", IntervalSeconds: 300}); err == nil {
		t.Fatal("SetScheduleConfig accepted an unknown mode")
	}
	if err := e.b.SetScheduleConfig(e.ctx, schedule.Config{Mode: schedule.ModeInterval, IntervalSeconds: 90}); err == nil {
		t.Fatal("SetScheduleConfig accepted an interval that is not offered")
	}
	if got, _ := e.b.ScheduleConfig(e.ctx); got != want {
		t.Fatalf("a rejected save changed the schedule: %+v", got)
	}
}

func TestIntegrationRememberSignalsTheConsolidator(t *testing.T) {
	e := newEnv(t, nil)

	for i := 0; i < 2; i++ {
		if _, err := e.b.Remember(e.ctx, e.ns, "the scheduler should hear about this", nil); err != nil {
			t.Fatalf("Remember: %v", err)
		}
	}
	select {
	case <-e.b.EpisodeSignal():
	default:
		t.Fatal("Remember did not signal the consolidator")
	}

	// Two episodes in one namespace are one run.
	ids := e.b.DrainEpisodeNamespaces()
	if len(ids) != 1 || ids[0] != e.nsID {
		t.Fatalf("drained %v, want exactly namespace %d", ids, e.nsID)
	}
	if again := e.b.DrainEpisodeNamespaces(); again != nil {
		t.Fatalf("drain did not clear the set: %v", again)
	}

	// Evaluation writes exist to measure recall; consolidating them mid-run
	// would turn the measurement into a training run.
	evalNS := "/eval/never-" + time.Now().Format("150405000000")
	if _, err := e.b.CreateNamespace(e.ctx, evalNS, evalNS, ""); err != nil {
		t.Fatalf("CreateNamespace: %v", err)
	}
	if _, err := e.b.Remember(e.ctx, evalNS, "an evaluation episode", nil); err != nil {
		t.Fatalf("Remember(eval): %v", err)
	}
	if ids := e.b.DrainEpisodeNamespaces(); ids != nil {
		t.Fatalf("an /eval/ write signalled consolidation: %v", ids)
	}
}
