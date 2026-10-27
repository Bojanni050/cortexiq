// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Bojanni050/cortexiq/internal/brain"
	"github.com/urfave/cli/v3"
)

// hygieneAuditCmd scans memories for noise. It is read-only: nothing is
// written, not even to the audit log.
func hygieneAuditCmd(ctx context.Context, cmd *cli.Command) error {
	opts := brain.HygieneAuditOptions{
		Namespaces: cmd.StringSlice("namespaces"),
		MinScore:   cmd.Float("min-score"),
		Limit:      cmd.Int("limit"),
		Offset:     cmd.Int("offset"),
		Tables:     cmd.StringSlice("tables"),
	}

	bc := getBootstrap(cmd)
	candidates, err := bc.Brain.Audit(ctx, opts)
	if err != nil {
		return err
	}

	return printJSON(map[string]any{
		"candidates": candidates,
		"count":      len(candidates),
		"read_only":  true,
	})
}

// hygieneCleanCmd acts on candidates. It reads a JSON candidate list (as
// produced by `hygiene audit --json > candidates.json`) and applies one action.
// Without --confirm it is a dry run: nothing is written.
func hygieneCleanCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("candidates file is required (or pass - for stdin); output of 'hygiene audit'")
	}
	path := args.First()

	var raw []byte
	var err error
	if path == "-" {
		raw, err = readStdin()
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return fmt.Errorf("read candidates: %w", err)
	}

	// Accept either the full audit output ({"candidates": [...]}) or a bare
	// array of candidates.
	var wrapped struct {
		Candidates []brain.HygieneCandidate `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Candidates != nil {
		// ok
	} else {
		wrapped.Candidates = nil
		if err := json.Unmarshal(raw, &wrapped.Candidates); err != nil {
			return fmt.Errorf("parse candidates: %w (expected the JSON output of 'hygiene audit')", err)
		}
	}

	action := cmd.String("action")
	switch strings.ToLower(action) {
	case "archive", "flag", "keep":
	default:
		return fmt.Errorf("unknown action %q: use archive, flag or keep", action)
	}

	bc := getBootstrap(cmd)
	result, err := bc.Brain.Clean(ctx, wrapped.Candidates, brain.HygieneCleanOptions{
		Action:  action,
		Confirm: cmd.Bool("confirm"),
		Actor:   cmd.String("actor"),
	})
	if err != nil {
		return err
	}

	out := map[string]any{
		"action":     result.Action,
		"dry_run":    result.DryRun,
		"counts":     result.Counts,
		"candidates": result.Candidates,
	}
	if result.DryRun {
		out["message"] = "dry run: nothing was written; re-run with --confirm to apply"
	} else {
		out["message"] = fmt.Sprintf("%s applied to %d candidates", result.Action, len(result.Candidates))
	}
	return printJSON(out)
}

// hygieneStatusCmd summarises the audit log and current archive/flag counts.
func hygieneStatusCmd(ctx context.Context, cmd *cli.Command) error {
	bc := getBootstrap(cmd)
	status, err := bc.Brain.HygieneStatus(ctx, cmd.StringSlice("namespaces"), cmd.Int("limit"))
	if err != nil {
		return err
	}
	return printJSON(status)
}

// hygieneRestoreCmd reverses archives: it puts the original importance back
// and clears the archive markers.
func hygieneRestoreCmd(ctx context.Context, cmd *cli.Command) error {
	bc := getBootstrap(cmd)
	restored, err := bc.Brain.Restore(ctx, cmd.StringSlice("namespaces"), cmd.Int("limit"))
	if err != nil {
		return err
	}
	return printJSON(map[string]any{
		"restored": restored,
		"message":  fmt.Sprintf("restored %d archived memories", restored),
	})
}

// hygieneFlags returns the shared flag set for the hygiene subcommands.
func hygieneAuditFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to scan (each includes descendants; default all)"},
		&cli.FloatFlag{Name: "min-score", Value: 0.3, Usage: "Lowest noise score to report (0..1); secrets always surface"},
		&cli.IntFlag{Name: "limit", Value: 200, Usage: "Max candidates"},
		&cli.IntFlag{Name: "offset", Value: 0, Usage: "Skip this many candidates"},
		&cli.StringSliceFlag{Name: "tables", Usage: "Tables to scan: episodes, facts (default: both)"},
	}
}

func readStdin() ([]byte, error) {
	return io.ReadAll(os.Stdin)
}
