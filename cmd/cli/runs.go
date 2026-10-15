package main

import (
	"context"

	"github.com/Bojanni050/cortexiq/internal/brain"
	"github.com/urfave/cli/v3"
)

// runsCmd prints recent consolidation runs as JSON.
func runsCmd(ctx context.Context, cmd *cli.Command) error {
	bc := getBootstrap(cmd)
	page := brain.Pagination{
		Offset: cmd.Int("offset"),
		Limit:  cmd.Int("limit"),
	}
	runs, err := bc.Brain.ListConsolidationRuns(ctx, cmd.StringSlice("namespaces"), page)
	if err != nil {
		return err
	}
	return printJSON(runs)
}
