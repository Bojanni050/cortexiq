// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

// namespaceConfigGetCmd shows the effective configuration for a namespace:
// the global defaults merged with the overrides of the namespace and its
// ancestors (nearest ancestor wins), plus the namespace's own overrides.
func namespaceConfigGetCmd(ctx context.Context, cmd *cli.Command) error {
	slug := cmd.Args().First()
	if slug == "" {
		return fmt.Errorf("namespace slug is required")
	}
	bc := getBootstrap(cmd)
	cfg, err := bc.Brain.GetNamespaceConfig(ctx, slug)
	if err != nil {
		return err
	}
	return printJSON(cfg)
}

// namespaceConfigSetCmd replaces the namespace's own overrides. Key/value
// pairs are given as key=value; numbers and booleans are parsed, everything
// else is stored as a string.
func namespaceConfigSetCmd(ctx context.Context, cmd *cli.Command) error {
	slug := cmd.Args().First()
	if slug == "" {
		return fmt.Errorf("namespace slug is required")
	}
	overrides := map[string]any{}
	for _, pair := range cmd.Args().Slice()[1:] {
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			return fmt.Errorf("override must be key=value, got %q", pair)
		}
		overrides[k] = parseConfigValue(v)
	}
	bc := getBootstrap(cmd)
	if err := bc.Brain.SetNamespaceConfig(ctx, slug, overrides); err != nil {
		return err
	}
	cfg, err := bc.Brain.GetNamespaceConfig(ctx, slug)
	if err != nil {
		return err
	}
	return printJSON(cfg)
}

// namespaceConfigUnsetCmd removes one override key, or all of the namespace's
// own overrides when no key is given.
func namespaceConfigUnsetCmd(ctx context.Context, cmd *cli.Command) error {
	slug := cmd.Args().First()
	if slug == "" {
		return fmt.Errorf("namespace slug is required")
	}
	bc := getBootstrap(cmd)
	if key := cmd.Args().Get(1); key != "" {
		if err := bc.Brain.DeleteNamespaceConfigKey(ctx, slug, key); err != nil {
			return err
		}
	} else {
		if err := bc.Brain.DeleteNamespaceConfig(ctx, slug); err != nil {
			return err
		}
	}
	cfg, err := bc.Brain.GetNamespaceConfig(ctx, slug)
	if err != nil {
		return err
	}
	return printJSON(cfg)
}

// parseConfigValue parses a CLI override value: true/false become booleans,
// integers and floats become numbers, everything else stays a string.
func parseConfigValue(s string) any {
	switch strings.ToLower(s) {
	case "true":
		return true
	case "false":
		return false
	}
	var i int64
	if _, err := fmt.Sscanf(s, "%d", &i); err == nil && fmt.Sprintf("%d", i) == s {
		return float64(i)
	}
	var f float64
	if err := json.Unmarshal([]byte(s), &f); err == nil {
		return f
	}
	return s
}
