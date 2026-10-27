// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Bojanni050/cortexiq/internal/bootstrap"
	"github.com/Bojanni050/cortexiq/internal/version"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:    "cortexiq",
		Usage:   "CortexIQ - Persistent Memory for AI",
		Version: version.Version,
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			// Version must work without a database: it is the one command an
			// operator runs when the service will not start. The root Before
			// hook runs before the subcommand is resolved (cmd is always the
			// root here), so read the first positional argument instead.
			// The client-side commands must run on a machine that has no
			// database and possibly no CortexIQ server at all: `connect` is
			// what an operator runs on their own laptop, and `token new` is
			// what they run before the server knows the token.
			if skipBootstrap(firstCommand(os.Args[1:])) {
				return ctx, nil
			}
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
				Name:  "version",
				Usage: "Print the version, commit and build date of this binary",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					fmt.Print(version.Details())
					return nil
				},
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
					{
						Name:      "config",
						Usage:     "Manage namespace configuration overrides",
						ArgsUsage: "<slug>",
						Commands: []*cli.Command{
							{
								Name:      "get",
								Usage:     "Show the effective configuration for a namespace",
								ArgsUsage: "<slug>",
								Action:    namespaceConfigGetCmd,
							},
							{
								Name:      "set",
								Usage:     "Set overrides for a namespace (key=value pairs)",
								ArgsUsage: "<slug> [key=value ...]",
								Action:    namespaceConfigSetCmd,
							},
							{
								Name:      "unset",
								Usage:     "Remove one override key, or all overrides when no key is given",
								ArgsUsage: "<slug> [key]",
								Action:    namespaceConfigUnsetCmd,
							},
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
					&cli.StringFlag{Name: "occurred-at", Usage: "When it occurred (RFC3339 or YYYY-MM-DD)"},
					&cli.FloatFlag{Name: "importance", Value: 0.5, Usage: "How much it matters, 0..1 (0.5 = neutral)"},
					&cli.StringSliceFlag{Name: "tag", Aliases: []string{"t"}, Usage: "Tag describing what it is about (repeatable)"},
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
					&cli.StringFlag{Name: "as-of", Usage: "Recall what the system knew at this time (RFC3339 or YYYY-MM-DD)"},
					&cli.StringFlag{Name: "known-from", Usage: "Recall what the system held at some point in [known-from, known-to) (RFC3339 or YYYY-MM-DD); not with --as-of"},
					&cli.StringFlag{Name: "known-to", Usage: "Exclusive end of the knowledge window (RFC3339 or YYYY-MM-DD)"},
					&cli.StringFlag{Name: "from", Usage: "Include event/valid time on or after this bound (RFC3339 or YYYY-MM-DD)"},
					&cli.StringFlag{Name: "to", Usage: "Exclude event/valid time at or after this bound (RFC3339 or YYYY-MM-DD)"},
					&cli.FloatFlag{Name: "mmr", Usage: "MMR diversity 0..1 (0=relevance, 1=diversity, 0.5=balanced); default: server config"},
					&cli.StringSliceFlag{Name: "tag", Aliases: []string{"t"}, Usage: "Only memories carrying every one of these tags (repeatable)"},
				},
			},
			{
				Name:    "reflect",
				Aliases: []string{"answer"},
				Usage:   "Answer a question by synthesizing recalled memories",
				Action:  reflectCmd,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to search (each includes descendants)"},
					&cli.IntFlag{Name: "limit", Value: 10, Usage: "Max memories to recall for grounding"},
					&cli.StringFlag{Name: "context", Usage: "Additional context to include in the answer synthesis"},
					&cli.StringFlag{Name: "types", Usage: "Comma-separated filter: fact, episode, failure, goal"},
					&cli.StringSliceFlag{Name: "tag", Aliases: []string{"t"}, Usage: "Ground the answer only on memories carrying every one of these tags (repeatable)"},
					&cli.StringFlag{Name: "as-of", Usage: "Answer from what the system knew at this time (RFC3339 or YYYY-MM-DD)"},
					&cli.StringFlag{Name: "known-from", Usage: "Answer from what the system held in [known-from, known-to) (RFC3339 or YYYY-MM-DD); not with --as-of"},
					&cli.StringFlag{Name: "known-to", Usage: "Exclusive end of the knowledge window (RFC3339 or YYYY-MM-DD)"},
					&cli.StringFlag{Name: "from", Usage: "Include event/valid time on or after this bound (RFC3339 or YYYY-MM-DD)"},
					&cli.StringFlag{Name: "to", Usage: "Exclude event/valid time at or after this bound (RFC3339 or YYYY-MM-DD)"},
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
				Name:    "retract",
				Aliases: []string{"correct"},
				Usage:   "Withdraw a claim because it was wrong, and record the correction",
				Action:  retractCmd,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "reason", Aliases: []string{"r"}, Usage: "What was wrong and what is true instead (required)"},
					&cli.Int64Flag{Name: "fact", Usage: "Retract this fact id instead of searching by description"},
					&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to search (each includes descendants)"},
					&cli.StringFlag{Name: "actor", Usage: "Who is retracting (default: cli)"},
				},
			},
			{
				Name:   "retractions",
				Usage:  "List recorded retractions, newest first",
				Action: retractionsCmd,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to list (each includes descendants)"},
					&cli.IntFlag{Name: "limit", Value: 50, Usage: "Max retractions"},
				},
			},
			{
				Name:  "purge",
				Usage: "Hard-delete memory and log what was removed",
				Commands: []*cli.Command{
					{
						Name:   "episode",
						Usage:  "Hard-delete an episode by ID, with facts only it sourced",
						Action: purgeEpisodeCmd,
						Flags:  purgeFlags(),
					},
					{
						Name:   "fact",
						Usage:  "Hard-delete a fact by ID",
						Action: purgeFactCmd,
						Flags:  purgeFlags(),
					},
					{
						Name:      "namespace",
						Usage:     "Hard-delete a namespace and everything filed under it",
						ArgsUsage: "<slug>",
						Action:    purgeNamespaceCmd,
						Flags: append(purgeFlags(),
							&cli.StringFlag{Name: "confirm", Usage: "Repeat the slug to confirm (required)"},
						),
					},
				},
			},
			{
				Name:   "purge-log",
				Usage:  "List recent hard deletes: what, when, by whom and why",
				Action: purgeLogCmd,
				Flags: []cli.Flag{
					&cli.IntFlag{Name: "limit", Value: 50, Usage: "Max entries"},
				},
			},
			{
				Name:   "tags",
				Usage:  "List the tag vocabulary with fact and episode counts (newest first)",
				Action: tagsCmd,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to count (each includes descendants; default all)"},
					&cli.StringFlag{Name: "since", Usage: "Only count memories stored on or after this time (RFC3339 or YYYY-MM-DD)"},
					&cli.IntFlag{Name: "limit", Value: 100, Usage: "Max tags"},
				},
			},
			{
				Name:   "facts",
				Usage:  "Query facts",
				Action: factsListCmd,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to query (each includes descendants)"},
					&cli.StringSliceFlag{Name: "tag", Aliases: []string{"t"}, Usage: "Only facts carrying every one of these tags (repeatable)"},
					&cli.StringFlag{Name: "since", Usage: "Created since (RFC3339)"},
					&cli.StringFlag{Name: "until", Usage: "Created until (RFC3339)"},
					&cli.StringFlag{Name: "as-of", Usage: "Only facts the system held at this time (RFC3339 or YYYY-MM-DD)"},
					&cli.StringFlag{Name: "known-from", Usage: "Only facts held at some point in [known-from, known-to) (RFC3339 or YYYY-MM-DD); not with --as-of"},
					&cli.StringFlag{Name: "known-to", Usage: "Exclusive end of the knowledge window (RFC3339 or YYYY-MM-DD)"},
					&cli.StringFlag{Name: "from", Usage: "Only facts valid during/after this bound (RFC3339 or YYYY-MM-DD)"},
					&cli.StringFlag{Name: "to", Usage: "Only facts valid before this bound (RFC3339 or YYYY-MM-DD)"},
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
						Name:   "graph",
						Usage:  "Recall with and without the graph arm over labelled facts, entities and relationships, in a temporary namespace",
						Action: evalGraphCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "corpus", Value: "eval/graph_corpus.jsonl", Usage: "JSON Lines facts (with entity) and relationships"},
							&cli.StringFlag{Name: "queries", Value: "eval/graph_queries.jsonl", Usage: "JSON Lines questions with the expected fact ids"},
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
					{
						Name:   "cluster",
						Usage:  "Does the lexical arm bring a twin into the seed's cluster that cosine alone misses?",
						Action: evalClusterCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "file", Aliases: []string{"f"}, Value: "eval/dedup_pairs.jsonl", Usage: "JSON Lines pair set"},
							&cli.FloatFlag{Name: "threshold", Usage: "Cosine threshold of the semantic arm (default: CORTEXIQ_CONSOLIDATION_SIMILARITY_THRESHOLD)"},
							&cli.IntSliceFlag{Name: "cap", Value: []int{2, 4, 8}, Usage: "Lexical caps to compare against threshold-only clustering"},
							&cli.FloatSliceFlag{Name: "floor", Value: []float64{0, 0.15, 0.2, 0.25, 0.3}, Usage: "Lexical floors to sweep per cap"},
						},
					},
				},
			},
			{
				Name:  "benchmark",
				Usage: "Run standardized memory benchmarks (LongMemEval-style)",
				Commands: []*cli.Command{
					{
						Name:      "run",
						Usage:     "Run a benchmark dataset and report scores",
						ArgsUsage: "<dataset.json>",
						Action:    benchmarkRunCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "dataset", Aliases: []string{"d"}, Usage: "Path to the benchmark dataset JSON file"},
							&cli.StringFlag{Name: "namespace", Aliases: []string{"n"}, Value: "/benchmark", Usage: "Namespace prefix for benchmark data (cleaned up after unless --keep)"},
							&cli.IntFlag{Name: "limit", Value: 10, Usage: "Max memories to recall per question (reflect limit)"},
							&cli.IntFlag{Name: "max-items", Usage: "Process at most this many items (0 = all)"},
							&cli.BoolFlag{Name: "llm-judge", Usage: "Use the LLM to judge answers (slower, more accurate than exact match)"},
							&cli.StringFlag{Name: "judge-model", Usage: "Model for the LLM judge (default: the configured reasoner model)"},
							&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Usage: "Write the JSON report to this file (default: stdout)"},
							&cli.BoolFlag{Name: "keep", Usage: "Keep the benchmark namespace after the run"},
						},
					},
					{
						Name:   "list",
						Usage:  "List available benchmark datasets",
						Action: benchmarkListCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "dir", Value: "eval/benchmark", Usage: "Directory to scan for datasets"},
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
							&cli.StringSliceFlag{Name: "tag", Aliases: []string{"t"}, Usage: "Only consolidate episodes carrying every one of these tags (targeted run; leaves the checkpoint alone)"},
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
				Name:    "mental-model",
				Aliases: []string{"mm"},
				Usage:   "Manage mental models (curated summaries)",
				Commands: []*cli.Command{
					{
						Name:      "create",
						Usage:     "Create a mental model from a source query",
						ArgsUsage: "<name> <source-query>",
						Action:    mentalModelCreateCmd,
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "namespace", Aliases: []string{"n"}, Value: "/", Usage: "File the model in this namespace and scope it there; / means all namespaces (global)"},
							&cli.StringSliceFlag{Name: "tags", Usage: "Tags for scoped visibility"},
							&cli.IntFlag{Name: "max-tokens", Value: 2048, Usage: "Maximum tokens for generated content"},
						},
					},
					{
						Name:   "list",
						Usage:  "List mental models",
						Action: mentalModelListCmd,
						Flags: []cli.Flag{
							&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to query (each includes descendants)"},
							&cli.StringSliceFlag{Name: "tags", Usage: "Filter by tags (any match)"},
							&cli.IntFlag{Name: "limit", Value: 100, Usage: "Max results"},
							&cli.IntFlag{Name: "offset", Value: 0, Usage: "Result offset"},
						},
					},
					{
						Name:      "show",
						Usage:     "Show a mental model by ID",
						ArgsUsage: "<id>",
						Action:    mentalModelShowCmd,
					},
					{
						Name:      "refresh",
						Usage:     "Refresh a mental model (regenerate content from its source query)",
						ArgsUsage: "<id>",
						Action:    mentalModelRefreshCmd,
					},
					{
						Name:      "delete",
						Usage:     "Delete a mental model",
						ArgsUsage: "<id>",
						Action:    mentalModelDeleteCmd,
					},
					{
						Name:      "search",
						Usage:     "Search mental models by semantic similarity",
						ArgsUsage: "<query>",
						Action:    mentalModelSearchCmd,
						Flags: []cli.Flag{
							&cli.StringSliceFlag{Name: "namespaces", Aliases: []string{"n"}, Usage: "Namespace paths to search (each includes descendants)"},
							&cli.StringSliceFlag{Name: "tags", Usage: "Filter by tags (any match)"},
							&cli.IntFlag{Name: "limit", Value: 5, Usage: "Max results"},
						},
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
				Name:  "connect",
				Usage: "Register CortexIQ as an MCP server in the agent clients on this machine",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "app", Aliases: []string{"a"}, Value: "all", Usage: "Client to configure: cursor, claude, opencode, or all"},
					&cli.StringFlag{Name: "url", Aliases: []string{"u"}, Value: defaultConnectURL, Usage: "MCP endpoint (streamable HTTP)"},
					&cli.StringFlag{Name: "token", Aliases: []string{"t"}, Usage: "API token to send (default: $CORTEXIQ_API_TOKEN)"},
					&cli.StringFlag{Name: "env-ref", Usage: "Write an environment reference instead of the secret"},
					&cli.StringFlag{Name: "name", Value: "cortexiq", Usage: "Server name the client files it under"},
					&cli.BoolFlag{Name: "global", Usage: "Write the machine-wide configuration instead of the project's"},
					&cli.BoolFlag{Name: "dry-run", Usage: "Report what would change without changing it"},
				},
				Action: connectCmd,
			},
			{
				Name:  "disconnect",
				Usage: "Remove CortexIQ from the agent clients on this machine",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "app", Aliases: []string{"a"}, Value: "all", Usage: "Client to change: cursor, claude, opencode, or all"},
					&cli.StringFlag{Name: "name", Value: "cortexiq", Usage: "Server name to remove"},
					&cli.BoolFlag{Name: "global", Usage: "Change the machine-wide configuration instead of the project's"},
					&cli.BoolFlag{Name: "dry-run", Usage: "Report what would change without changing it"},
				},
				Action: disconnectCmd,
			},
			{
				Name:  "token",
				Usage: "API token helpers",
				Commands: []*cli.Command{
					{
						Name:  "new",
						Usage: "Generate a token and print the CORTEXIQ_API_TOKENS entry for it",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "name", Aliases: []string{"n"}, Usage: "Name for the token, e.g. the app it belongs to (required)"},
							// Removed on purpose, but still accepted so a caller who
							// read the earlier version gets the reason instead of
							// "flag provided but not defined". Hidden, so nobody
							// discovers a flag that only refuses.
							&cli.StringFlag{Name: "namespace", Hidden: true, Usage: "Removed: a token is per app, a namespace is per project"},
							&cli.StringFlag{Name: "scope", Value: "rw", Usage: "rw (read-write) or ro (read-only)"},
						},
						Action: tokenNewCmd,
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

// firstCommand returns the first positional argument, ignoring flags. It is
// used only to let `version` run before the database is opened: the root
// Before hook fires before the subcommand is known, so the argument list is
// the only thing that distinguishes `cortexiq version` from any other command.
func firstCommand(args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return arg
	}
	return ""
}

// skipBootstrap reports whether a command runs without the database. The
// command is not resolved yet when the root Before hook fires, so the argument
// list is the only thing available: that is what firstCommand exists for.
func skipBootstrap(command string) bool {
	switch command {
	case "version", "connect", "disconnect", "token":
		return true
	default:
		return false
	}
}

// purgeFlags are shared by every purge subcommand.
func purgeFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "reason", Aliases: []string{"r"}, Usage: "Why this is being deleted (recorded in purge_log)"},
		&cli.StringFlag{Name: "actor", Usage: "Who is deleting (default: cli)"},
		&cli.BoolFlag{Name: "dry-run", Usage: "Report what would be removed without removing it"},
	}
}
