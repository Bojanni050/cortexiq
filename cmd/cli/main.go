// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/Bojanni050/cortexiq/internal/bootstrap"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "cortexiq",
		Usage: "CortexIQ - Persistent Memory for AI",
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			if _, ok := cmd.Root().Metadata["bootstrapCtx"]; ok {
				return ctx, nil
			}
			bc, err := bootstrap.New(ctx)
			if err != nil {
				return ctx, err
			}
			cmd.Metadata["bootstrapCtx"] = bc
			return ctx, nil
		},
		After: func(ctx context.Context, cmd *cli.Command) error {
			if bc, ok := cmd.Metadata["bootstrapCtx"].(*bootstrap.Context); ok {
				return bc.Close()
			}
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:  "serve",
				Usage: "Start all services (HTTP, MCP, consolidation)",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "http-host", Value: "0.0.0.0", Usage: "HTTP server host"},
					&cli.StringFlag{Name: "http-port", Value: "9090", Usage: "HTTP server port (metrics, health)"},
					&cli.StringFlag{Name: "mcp-host", Value: "0.0.0.0", Usage: "MCP SSE server host"},
					&cli.StringFlag{Name: "mcp-port", Value: "8080", Usage: "MCP SSE server port"},
					&cli.DurationFlag{Name: "consolidate-interval", Value: 5 * time.Minute, Usage: "Consolidation interval"},
					&cli.StringSliceFlag{Name: "consolidate-namespaces", Usage: "Namespaces to consolidate (default: all)"},
				},
				Action: serveAllCmd,
			},
			{
				Name:  "http",
				Usage: "HTTP server commands",
				Commands: []*cli.Command{
					{
						Name:   "serve",
						Usage:  "Start HTTP server (metrics, health)",
						Action: serveHTTP,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "host", Value: "0.0.0.0"},
							&cli.StringFlag{Name: "port", Value: "9090"},
						},
					},
				},
			},
			{
				Name:   "env",
				Usage:  "Show environment variables and configuration",
				Action: EnvCmd,
			},
			{
				Name:  "namespace",
				Usage: "Manage namespaces",
				Commands: []*cli.Command{
					{
						Name:   "create",
						Usage:  "Create a namespace",
						Action: namespaceCreateCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "name", Usage: "Display name"},
							&cli.StringFlag{Name: "description", Usage: "Description"},
						},
					},
					{
						Name:      "rename",
						Usage:     "Rename a namespace and its descendants",
						ArgsUsage: "<from> <to>",
						Action:    namespaceRenameCmd,
					},
					{
						Name:   "list",
						Usage:  "List namespaces",
						Action: namespaceListCmd,
						Flags: []cli.Flag{
							&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to list (each includes descendants)"},
							&cli.IntFlag{Name: "limit", Value: 100, Usage: "Max results"},
							&cli.IntFlag{Name: "offset", Value: 0, Usage: "Result offset"},
						},
					},
				},
			},
			{
				Name:    "remember",
				Aliases: []string{"add"},
				Usage:   "Store a memory",
				Action:  rememberCmd,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "namespace", Aliases: []string{"n"}, Value: "/", Usage: "Namespace path"},
					&cli.StringFlag{Name: "occurred-at", Usage: "When it occurred (RFC3339)"},
					&cli.FloatFlag{Name: "importance", Value: 0.5, Usage: "How much it matters, 0..1 (0.5 = neutral)"},
				},
			},
			{
				Name:    "recall",
				Aliases: []string{"search"},
				Usage:   "Search for relevant memories",
				Action:  recallCmd,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to search (each includes descendants)"},
					&cli.IntFlag{Name: "limit", Value: 10, Usage: "Max results"},
					&cli.StringFlag{Name: "as-of", Usage: "Recall what was known at this time (RFC3339 or YYYY-MM-DD)"},
				},
			},
			{
				Name:   "forget",
				Usage:  "Soft-delete a matching episode",
				Action: forgetCmd,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to search (each includes descendants)"},
				},
			},
			{
				Name:  "purge",
				Usage: "Hard-delete by ID",
				Commands: []*cli.Command{
					{
						Name:   "episode",
						Usage:  "Hard-delete an episode by ID",
						Action: purgeEpisodeCmd,
					},
					{
						Name:   "fact",
						Usage:  "Hard-delete a fact by ID",
						Action: purgeFactCmd,
					},
				},
			},
			{
				Name:   "facts",
				Usage:  "Query facts",
				Action: factsListCmd,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to query (each includes descendants)"},
					&cli.StringFlag{Name: "since", Usage: "Since timestamp (RFC3339)"},
					&cli.StringFlag{Name: "until", Usage: "Until timestamp (RFC3339)"},
					&cli.StringFlag{Name: "as-of", Usage: "Only facts valid at this time (RFC3339 or YYYY-MM-DD)"},
					&cli.IntFlag{Name: "limit", Value: 100, Usage: "Max results"},
					&cli.IntFlag{Name: "offset", Value: 0, Usage: "Result offset"},
				},
			},
			{
				Name:   "runs",
				Usage:  "Show recent consolidation runs (what each run changed)",
				Action: runsCmd,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to query (each includes descendants; default all)"},
					&cli.IntFlag{Name: "limit", Value: 50, Usage: "Max results"},
					&cli.IntFlag{Name: "offset", Value: 0, Usage: "Result offset"},
				},
			},
			{
				Name:   "export",
				Usage:  "Export episodes and facts as JSON Lines (embeddings are not included)",
				Action: exportCmd,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to export (each includes descendants; default /)"},
					&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Usage: "Output file (default: stdout)"},
					&cli.StringSliceFlag{Name: "types", Usage: "episode and/or fact (default: both)"},
				},
			},
			{
				Name:      "import",
				Usage:     "Import a file written by 'export'; embeddings are recomputed with the configured model",
				ArgsUsage: "<file|->",
				Action:    importCmd,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{Name: "types", Usage: "episode and/or fact (default: both)"},
					&cli.BoolFlag{Name: "dry-run", Aliases: []string{"d"}, Usage: "Validate and count without writing"},
				},
			},
			{
				Name:   "dedupe",
				Usage:  "Merge duplicate facts (dry-run unless --apply)",
				Action: dedupeCmd,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to clean (each includes descendants; default /)"},
					&cli.FloatFlag{Name: "threshold", Usage: "Merge facts at or above this cosine similarity (default: CORTEXIQ_CONSOLIDATION_DEDUP_THRESHOLD)"},
					&cli.BoolFlag{Name: "review", Usage: "Let the LLM review pairs between the review threshold and --threshold"},
					&cli.IntFlag{Name: "max-reviews", Value: 100, Usage: "Maximum LLM review calls"},
					&cli.BoolFlag{Name: "apply", Usage: "Write the merge (without it nothing is changed)"},
				},
			},
			{
				Name:  "eval",
				Usage: "Score quality against labelled calibration sets (nothing is written)",
				Commands: []*cli.Command{
					{
						Name:   "recall",
						Usage:  "Hit rate and MRR of recall over a labelled corpus and question set, in a temporary namespace",
						Action: evalRecallCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "corpus", Value: "eval/recall_corpus.jsonl", Usage: "JSON Lines memories to store as episodes"},
							&cli.StringFlag{Name: "queries", Value: "eval/recall_queries.jsonl", Usage: "JSON Lines questions with the expected memory ids"},
							&cli.IntFlag{Name: "limit", Value: 10, Usage: "Results per question (k)"},
							&cli.BoolFlag{Name: "keep", Usage: "Keep the temporary /eval/ namespace afterwards"},
						},
					},
					{
						Name:   "dedup",
						Usage:  "Cosine distribution and threshold sweep over labelled fact pairs",
						Action: evalDedupCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "file", Aliases: []string{"f"}, Value: "eval/dedup_pairs.jsonl", Usage: "JSON Lines pair set"},
							&cli.FloatFlag{Name: "threshold", Usage: "Auto-merge threshold to score against (default: CORTEXIQ_CONSOLIDATION_DEDUP_THRESHOLD)"},
							&cli.FloatFlag{Name: "review-threshold", Usage: "LLM review threshold to score against (default: CORTEXIQ_CONSOLIDATION_DEDUP_REVIEW_THRESHOLD)"},
							&cli.BoolFlag{Name: "llm", Usage: "Also ask the reasoner about the pairs in the review band (one call each)"},
						},
					},
				},
			},
			{
				Name:  "consolidate",
				Usage: "Consolidate memories",
				Commands: []*cli.Command{
					{
						Name:   "run",
						Usage:  "Run consolidation once",
						Action: consolidateRunCmd,
						Flags: []cli.Flag{
							&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to consolidate (each includes descendants)"},
							&cli.BoolFlag{Name: "dry-run", Aliases: []string{"d"}, Usage: "Show what would be consolidated"},
						},
					},
					{
						Name:   "serve",
						Usage:  "Run consolidation as a background service",
						Action: consolidateServeCmd,
						Flags: []cli.Flag{
							&cli.DurationFlag{Name: "interval", Aliases: []string{"i"}, Value: 5 * time.Minute, Usage: "Interval between runs"},
							&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespaces to consolidate"},
						},
					},
				},
			},
			{
				Name:  "context",
				Usage: "Manage working context",
				Commands: []*cli.Command{
					{
						Name:   "set",
						Usage:  "Set working context focus",
						Action: contextSetCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "namespace", Aliases: []string{"n"}, Value: "/", Usage: "Namespace path"},
							&cli.DurationFlag{Name: "expires", Value: 1 * time.Hour, Usage: "Context TTL"},
						},
					},
					{
						Name:   "show",
						Usage:  "Show current context",
						Action: contextShowCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "namespace", Aliases: []string{"n"}, Value: "/", Usage: "Namespace path"},
						},
					},
				},
			},
			{
				Name:  "contradictions",
				Usage: "Manage contradictions",
				Commands: []*cli.Command{
					{
						Name:   "list",
						Usage:  "List unresolved contradictions",
						Action: contradictionsListCmd,
						Flags: []cli.Flag{
							&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to query (each includes descendants)"},
							&cli.IntFlag{Name: "limit", Value: 100, Usage: "Max results"},
							&cli.IntFlag{Name: "offset", Value: 0, Usage: "Result offset"},
						},
					},
					{
						Name:   "resolve",
						Usage:  "Resolve a contradiction by ID",
						Action: contradictionResolveCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "resolution", Value: "resolved", Usage: "Resolution note"},
						},
					},
				},
			},
			{
				Name:  "causal",
				Usage: "Manage causal links between facts",
				Commands: []*cli.Command{
					{
						Name:   "list",
						Usage:  "List causal links",
						Action: causalListCmd,
						Flags: []cli.Flag{
							&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to query (each includes descendants)"},
							&cli.IntFlag{Name: "limit", Value: 100, Usage: "Max results"},
							&cli.IntFlag{Name: "offset", Value: 0, Usage: "Result offset"},
						},
					},
					{
						Name:   "create",
						Usage:  "Create a causal link between two facts",
						Action: causalCreateCmd,
						Flags: []cli.Flag{
							&cli.IntFlag{Name: "cause-id", Usage: "ID of the cause fact"},
							&cli.IntFlag{Name: "effect-id", Usage: "ID of the effect fact"},
							&cli.FloatFlag{Name: "confidence", Value: 0.8, Usage: "Confidence score (0-1)"},
							&cli.StringFlag{Name: "namespace", Aliases: []string{"n"}, Value: "/", Usage: "Namespace path"},
						},
					},
					{
						Name:   "trace",
						Usage:  "Trace causal chain from a fact",
						Action: causalTraceCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "direction", Value: "forward", Usage: "Trace direction: forward or backward"},
							&cli.IntFlag{Name: "depth", Value: 10, Usage: "Max traversal depth"},
						},
					},
					{
						Name:   "delete",
						Usage:  "Delete a causal link by ID",
						Action: causalDeleteCmd,
					},
				},
			},
			{
				Name:  "hypothesis",
				Usage: "Manage hypotheses",
				Commands: []*cli.Command{
					{
						Name:   "create",
						Usage:  "Create a hypothesis",
						Action: hypothesisCreateCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "namespace", Aliases: []string{"n"}, Value: "/", Usage: "Namespace path"},
							&cli.StringFlag{Name: "verification-plan", Usage: "How to verify this hypothesis"},
							&cli.FloatFlag{Name: "confidence", Value: 0.5, Usage: "Confidence score (0-1)"},
							&cli.StringFlag{Name: "source-fact-ids", Usage: "Comma-separated fact IDs supporting this hypothesis"},
						},
					},
					{
						Name:   "list",
						Usage:  "List hypotheses",
						Action: hypothesisListCmd,
						Flags: []cli.Flag{
							&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to query (each includes descendants)"},
							&cli.StringFlag{Name: "status", Usage: "Filter by status: proposed, testing, confirmed, rejected"},
							&cli.IntFlag{Name: "limit", Value: 100, Usage: "Max results"},
							&cli.IntFlag{Name: "offset", Value: 0, Usage: "Result offset"},
						},
					},
					{
						Name:   "show",
						Usage:  "Show a hypothesis by ID",
						Action: hypothesisShowCmd,
					},
					{
						Name:   "test",
						Usage:  "Mark a hypothesis as testing",
						Action: hypothesisTestCmd,
					},
					{
						Name:   "confirm",
						Usage:  "Confirm a hypothesis and auto-create a fact",
						Action: hypothesisConfirmCmd,
					},
					{
						Name:   "reject",
						Usage:  "Reject a hypothesis",
						Action: hypothesisRejectCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "reason", Usage: "Rejection reason"},
						},
					},
					{
						Name:   "refine",
						Usage:  "Refine a hypothesis (resets to proposed)",
						Action: hypothesisRefineCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "content", Usage: "Updated hypothesis content"},
							&cli.StringFlag{Name: "verification-plan", Usage: "Updated verification plan"},
							&cli.FloatFlag{Name: "confidence", Usage: "Updated confidence score"},
						},
					},
					{
						Name:   "delete",
						Usage:  "Delete a hypothesis",
						Action: hypothesisDeleteCmd,
					},
				},
			},
			{
				Name:  "goal",
				Usage: "Manage goals",
				Commands: []*cli.Command{
					{
						Name:   "create",
						Usage:  "Create a goal",
						Action: goalCreateCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "namespace", Aliases: []string{"n"}, Value: "/", Usage: "Namespace path"},
							&cli.IntFlag{Name: "parent-id", Usage: "Parent goal ID for sub-goals"},
							&cli.IntFlag{Name: "priority", Value: 0, Usage: "Priority (higher = more important)"},
						},
					},
					{
						Name:   "list",
						Usage:  "List goals",
						Action: goalListCmd,
						Flags: []cli.Flag{
							&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to query (each includes descendants)"},
							&cli.StringFlag{Name: "status", Usage: "Filter by status: active, completed, abandoned"},
							&cli.IntFlag{Name: "parent-id", Usage: "Filter by parent goal ID"},
							&cli.IntFlag{Name: "limit", Value: 100, Usage: "Max results"},
							&cli.IntFlag{Name: "offset", Value: 0, Usage: "Result offset"},
						},
					},
					{
						Name:   "show",
						Usage:  "Show a goal with sub-goal progress",
						Action: goalShowCmd,
					},
					{
						Name:   "complete",
						Usage:  "Complete a goal",
						Action: goalCompleteCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "notes", Usage: "Completion notes"},
						},
					},
					{
						Name:   "abandon",
						Usage:  "Abandon a goal",
						Action: goalAbandonCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "notes", Usage: "Abandonment reason"},
						},
					},
					{
						Name:   "update",
						Usage:  "Update an active goal",
						Action: goalUpdateCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "content", Usage: "Updated goal content"},
							&cli.IntFlag{Name: "priority", Usage: "Updated priority"},
							&cli.StringFlag{Name: "notes", Usage: "Updated notes"},
						},
					},
					{
						Name:   "delete",
						Usage:  "Delete a goal and its sub-goals",
						Action: goalDeleteCmd,
					},
				},
			},
			{
				Name:  "failure",
				Usage: "Manage failure records",
				Commands: []*cli.Command{
					{
						Name:   "create",
						Usage:  "Record a failure",
						Action: failureCreateCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "namespace", Aliases: []string{"n"}, Value: "/", Usage: "Namespace path"},
							&cli.StringFlag{Name: "reason", Usage: "Why it failed (required)"},
							&cli.StringFlag{Name: "lesson", Usage: "What to do instead (required)"},
							&cli.IntFlag{Name: "goal-id", Usage: "Related goal ID"},
						},
					},
					{
						Name:   "list",
						Usage:  "List failures",
						Action: failureListCmd,
						Flags: []cli.Flag{
							&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to query (each includes descendants)"},
							&cli.IntFlag{Name: "goal-id", Usage: "Filter by goal ID"},
							&cli.IntFlag{Name: "limit", Value: 100, Usage: "Max results"},
							&cli.IntFlag{Name: "offset", Value: 0, Usage: "Result offset"},
						},
					},
					{
						Name:   "show",
						Usage:  "Show a failure by ID",
						Action: failureShowCmd,
					},
					{
						Name:   "delete",
						Usage:  "Delete a failure",
						Action: failureDeleteCmd,
					},
				},
			},
			{
				Name:  "mcp",
				Usage: "MCP server for agent integration",
				Commands: []*cli.Command{
					{
						Name:   "serve",
						Usage:  "Start MCP server over SSE",
						Action: mcpServeCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "host", Value: "0.0.0.0"},
							&cli.StringFlag{Name: "port", Value: "8080"},
							&cli.BoolFlag{Name: "with-consolidation", Usage: "Run consolidation in background alongside MCP server"},
							&cli.DurationFlag{Name: "consolidate-interval", Value: 5 * time.Minute, Usage: "Consolidation interval"},
							&cli.StringSliceFlag{Name: "consolidate-namespaces", Usage: "Namespaces to consolidate (default: all)"},
						},
					},
					{
						Name:   "execute",
						Usage:  "Start MCP server over stdio",
						Action: mcpExecuteCmd,
						Flags: []cli.Flag{
							&cli.BoolFlag{Name: "with-consolidation", Usage: "Run consolidation in background alongside MCP server"},
							&cli.DurationFlag{Name: "consolidate-interval", Value: 5 * time.Minute, Usage: "Consolidation interval"},
							&cli.StringSliceFlag{Name: "consolidate-namespaces", Usage: "Namespaces to consolidate (default: all)"},
						},
					},
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
