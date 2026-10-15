// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Bojanni050/cortexiq/internal/config"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/urfave/cli/v3"
)

func EnvCmd(ctx context.Context, cmd *cli.Command) error {
	// Legacy STASH_* names are folded into CORTEXIQ_* so the listing is complete.
	config.ApplyLegacyEnv()

	// Collect all CORTEXIQ_* environment variables
	vars := getAllCortexIQEnvVars()

	if len(vars) == 0 {
		fmt.Println("No CORTEXIQ_* environment variables found.")
		return nil
	}

	// Create and render table
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleLight)
	t.AppendHeader(table.Row{"Environment Variable", "Value"})

	sensitiveSuffixes := []string{"KEY", "SECRET", "PASSWORD", "TOKEN", "DSN"}

	for _, env := range vars {
		masked := env[1]
		for _, suffix := range sensitiveSuffixes {
			if strings.HasSuffix(env[0], suffix) && len(masked) > 4 {
				masked = masked[:4] + strings.Repeat("*", len(masked)-4)
				break
			}
		}
		t.AppendRow(table.Row{env[0], masked})
	}

	t.Render()
	return nil
}

func getAllCortexIQEnvVars() [][2]string {
	vars := [][2]string{}

	// Get all env vars and filter for CORTEXIQ_ prefix
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "CORTEXIQ_") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				vars = append(vars, [2]string{parts[0], parts[1]})
			}
		}
	}

	// Include CORTEXIQCONFIG (not CORTEXIQ_ prefixed but relevant)
	if configFile := os.Getenv(config.ConfigEnvVar); configFile != "" {
		vars = append(vars, [2]string{config.ConfigEnvVar, configFile})
	}

	// Sort alphabetically for consistent output
	sort.Slice(vars, func(i, j int) bool {
		return vars[i][0] < vars[j][0]
	})

	return vars
}
