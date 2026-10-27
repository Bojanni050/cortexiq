package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

// tagsCmd prints the tag vocabulary with counts plus how much of the store is
// tagged at all, optionally limited to a period. The two belong together: the
// vocabulary can only show tags that exist, so on its own it cannot tell an
// empty namespace from an untagged one.
func tagsCmd(ctx context.Context, cmd *cli.Command) error {
	since, err := parseAsOf(cmd.String("since"))
	if err != nil {
		return fmt.Errorf("invalid since: %w", err)
	}
	bc := getBootstrap(cmd)
	namespaces := cmd.StringSlice("namespaces")
	tags, err := bc.Brain.ListTags(ctx, namespaces, since, cmd.Int("limit"))
	if err != nil {
		return err
	}
	coverage, err := bc.Brain.TagCoverage(ctx, namespaces)
	if err != nil {
		return err
	}
	return printJSON(map[string]any{"tags": tags, "coverage": coverage})
}
