package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Bojanni050/cortexiq/internal/brain"
	"github.com/urfave/cli/v3"
)

// dedupeCmd merges duplicate facts. It only reports unless --apply is given.
func dedupeCmd(ctx context.Context, cmd *cli.Command) error {
	namespaces := cmd.StringSlice("namespaces")
	if len(namespaces) == 0 {
		namespaces = []string{"/"}
	}

	bc := getBootstrap(cmd)
	threshold := float32(cmd.Float("threshold"))
	if threshold == 0 {
		threshold = float32(bc.Config.ConsolidationDedupThreshold)
	}
	var review float32
	if cmd.Bool("review") {
		review = float32(bc.Config.ConsolidationDedupReviewThreshold)
	}

	res, err := bc.Brain.DedupeFacts(ctx, brain.DedupeOptions{
		Namespaces:      namespaces,
		Threshold:       threshold,
		ReviewThreshold: review,
		MaxReviews:      cmd.Int("max-reviews"),
		Apply:           cmd.Bool("apply"),
	})
	if err != nil {
		return err
	}
	if err := printJSON(res); err != nil {
		return err
	}
	if res.Applied {
		fmt.Fprintf(os.Stderr, "merged %d duplicate facts in %d groups\n", res.FactsMerged, len(res.Groups))
	} else {
		fmt.Fprintf(os.Stderr, "dry run: would merge %d duplicate facts in %d groups; use --apply to write\n", res.FactsMerged, len(res.Groups))
	}
	return nil
}
