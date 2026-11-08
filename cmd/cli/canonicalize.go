// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

// canonicalizeRelationshipsCmd folds the entity names of a namespace onto their
// canonical spelling and drops the edges whose entity is only a description. It is
// a command and not a migration because the rule is a heuristic: run it with
// --dry-run first, read the mapping, and only then let it touch the evidence.
func canonicalizeRelationshipsCmd(ctx context.Context, cmd *cli.Command) error {
	namespaces := cmd.StringSlice("namespaces")
	dryRun := cmd.Bool("dry-run")
	if len(namespaces) == 0 {
		namespaces = []string{"/"}
	}

	bc := getBootstrap(cmd)

	ids, err := bc.Brain.ResolveNamespaceIDs(ctx, namespaces)
	if err != nil {
		return fmt.Errorf("resolve namespaces: %w", err)
	}

	report, err := bc.Brain.CanonicalizeEntities(ctx, ids, dryRun)
	if err != nil {
		return err
	}
	return printJSON(report)
}
