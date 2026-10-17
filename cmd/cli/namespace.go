// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"context"
	"fmt"

	"github.com/Bojanni050/cortexiq/internal/brain"
	"github.com/urfave/cli/v3"
)

func namespaceCreateCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("namespace slug is required")
	}
	slug := args.First()
	name := cmd.String("name")
	if name == "" {
		name = slug
	}
	description := cmd.String("description")

	bc := getBootstrap(cmd)
	id, err := bc.Brain.CreateNamespace(ctx, slug, name, description)
	if err != nil {
		return err
	}
	return printJSON(map[string]any{"id": id, "slug": slug})
}

func namespaceRenameCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() < 2 {
		return fmt.Errorf("usage: cortexiq namespace rename <from> <to>")
	}

	bc := getBootstrap(cmd)
	res, err := bc.Brain.RenameNamespace(ctx, args.Get(0), args.Get(1))
	if err != nil {
		return err
	}
	return printJSON(res)
}

func namespaceListCmd(ctx context.Context, cmd *cli.Command) error {
	patterns := cmd.StringSlice("namespaces")
	page := brain.Pagination{
		Offset: cmd.Int("offset"),
		Limit:  cmd.Int("limit"),
	}
	bc := getBootstrap(cmd)
	namespaces, err := bc.Brain.ListNamespaces(ctx, patterns, page)
	if err != nil {
		return err
	}
	return printJSON(namespaces)
}
