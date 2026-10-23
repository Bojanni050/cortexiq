package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

// tagsCmd prints the tag vocabulary with counts, optionally limited to a period.
func tagsCmd(ctx context.Context, cmd *cli.Command) error {
	since, err := parseAsOf(cmd.String("since"))
	if err != nil {
		return fmt.Errorf("invalid since: %w", err)
	}
	bc := getBootstrap(cmd)
	tags, err := bc.Brain.ListTags(ctx, cmd.StringSlice("namespaces"), since, cmd.Int("limit"))
	if err != nil {
		return err
	}
	return printJSON(tags)
}
