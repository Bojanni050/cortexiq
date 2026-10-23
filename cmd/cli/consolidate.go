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

	sched := &schedule.Scheduler{
		Initial: cfg,
		Consolidate: func(ctx context.Context, nsID int64) error {
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
		Logf:     log.Printf,
	}
	return sched.Run(ctx)
}
