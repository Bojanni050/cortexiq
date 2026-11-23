// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Bojanni050/cortexiq/internal/bootstrap"
	"github.com/Bojanni050/cortexiq/internal/brain"
	"github.com/Bojanni050/cortexiq/internal/schedule"
	"github.com/urfave/cli/v3"
)

func consolidateRunCmd(ctx context.Context, cmd *cli.Command) error {
	namespaces := cmd.StringSlice("namespaces")
	tags := cmd.StringSlice("tag")
	dryRun := cmd.Bool("dry-run")

	bc := getBootstrap(cmd)

	if dryRun {
		return printJSON(map[string]any{
			"namespaces": namespaces,
			"tags":       tags,
			"status":     "dry-run requested",
		})
	}

	if len(namespaces) == 0 {
		namespaces = []string{"/"}
	}

	ids, err := bc.Brain.ResolveNamespaceIDs(ctx, namespaces)
	if err != nil {
		return fmt.Errorf("resolve namespaces: %w", err)
	}

	var results []brain.ConsolidationResult
	for _, id := range ids {
		result, err := bc.Brain.ConsolidateByIDWithTags(ctx, id, tags)
		if err != nil {
			log.Printf("Consolidation failed for namespace ID %d: %v", id, err)
			continue
		}
		results = append(results, result)
		bc.Logger.Info("consolidation completed", "result", result)
	}

	return printJSON(results)
}

func consolidateServeCmd(ctx context.Context, cmd *cli.Command) error {
	interval := cmd.Duration("interval")
	namespaces := cmd.StringSlice("namespaces")

	bc := getBootstrap(cmd)

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Printf("Starting consolidation service")
	return runSchedule(ctx, bc, interval, namespaces)
}

// runSchedule is the background consolidator shared by `serve` and
// `mcp serve --with-consolidation`. The mode and interval come from the
// settings table (the dashboard writes them there); the flag only seeds the
// first start and is the fallback when the table cannot be read.
func runSchedule(ctx context.Context, bc *bootstrap.Context, interval time.Duration, namespaces []string) error {
	if len(namespaces) == 0 {
		namespaces = []string{"/"}
	}

	// The flag supplies the initial value without ever overwriting a choice
	// made in the dashboard.
	if err := bc.Brain.SeedSchedule(ctx, schedule.ConfigFromInterval(interval)); err != nil {
		log.Printf("Consolidation: cannot seed schedule: %v", err)
	}
	cfg, err := bc.Brain.ScheduleConfig(ctx)
	if err != nil {
		log.Printf("Consolidation: cannot read schedule, using interval %s: %v", interval, err)
		cfg = schedule.ConfigFromInterval(interval)
	}

	// Writes made by a sibling process (the dashboard in the split deployment,
	// or a stdio server) only reach this one through the database, so this is
	// also the only process that listens for their wake-ups. Without it a write
	// from the dashboard waited for the next tick — two hours on a slow
	// schedule.
	go bc.Brain.WatchEpisodeSignals(ctx)

	// The merges that need no judgement run here too: a merge is a write, and
	// this is the process that owns background writes. It keeps its own clock,
	// because a consolidation tick means "read new facts" and this pass is not
	// reacting to episodes.
	if bc.Config.CleanupSweep {
		go sweepCleanup(ctx, bc, namespaces, bc.Config.CleanupSweepInterval)
	} else {
		log.Printf("Clean-up sweep: off; the dashboard merges only when asked")
	}

	sched := &schedule.Scheduler{
		Initial: cfg,
		Consolidate: func(ctx context.Context, nsID int64) error {
			// A model chosen in the dashboard reaches this process only through the
			// database, so read it before every run. A failure keeps the models
			// already running: one unreadable row should not stop consolidation.
			if changed, err := bc.ReloadModelBindings(ctx); err != nil {
				log.Printf("Consolidation: cannot reload model bindings, keeping the current models: %v", err)
			} else if changed {
				log.Printf("Consolidation: model bindings changed in the database, now using the new models")
			}
			result, err := bc.Brain.ConsolidateByID(ctx, nsID)
			if err != nil {
				return err
			}
			b, _ := json.Marshal(result)
			log.Printf("Consolidation completed for %s: %s", result.Namespace, string(b))
			return nil
		},
		Namespaces: func(ctx context.Context) ([]int64, error) {
			return bc.Brain.ResolveNamespaceIDs(ctx, namespaces)
		},
		Config:   bc.Brain.ScheduleConfig,
		Episodes: bc.Brain.EpisodeSignal,
		Drain:    bc.Brain.DrainEpisodeNamespaces,
		Changes:  bc.Brain.ScheduleChanged,
		Now:      bc.Brain.RunNow,
		// A run that failed on a batch leaves work waiting behind it. Retrying
		// two minutes later beats waiting for the next tick, which on a slow
		// schedule is a day.
		Pending:    bc.Brain.NamespacesWithPendingWork,
		RetryDelay: 2 * time.Minute,
		Logf:       log.Printf,
	}
	return sched.Run(ctx)
}

// sweepCleanup folds the duplicate spellings that need no judgement, on its own
// interval. The first pass waits a few minutes: a restart should not put a merge
// in front of the work the process was restarted for.
func sweepCleanup(ctx context.Context, bc *bootstrap.Context, namespaces []string, every time.Duration) {
	if every <= 0 {
		every = 6 * time.Hour
	}
	t := time.NewTimer(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		rep, err := bc.Brain.SweepCleanup(ctx, namespaces)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			log.Printf("Clean-up sweep: cannot read the duplicate groups: %v", err)
		default:
			log.Printf("Clean-up sweep: %s", sweepLine(rep))
		}
		t.Reset(every)
	}
}

// sweepLine words one pass: what changed, and why the rest did not.
func sweepLine(rep brain.SweepReport) string {
	if rep.Decidable == 0 {
		return "nothing to settle"
	}
	line := fmt.Sprintf("%d merged of %d groups that needed no judgement; %d facts and %d relations rewritten",
		rep.Merged, rep.Decidable, rep.Facts, rep.Relations)
	if rep.Failed > 0 {
		line += fmt.Sprintf("; %d refused, first: %s", rep.Failed, rep.FirstError)
	}
	return line
}
