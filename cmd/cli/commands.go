// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Bojanni050/cortexiq/internal/bootstrap"
	"github.com/Bojanni050/cortexiq/internal/brain"
	"github.com/urfave/cli/v3"
)

func rememberCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("content argument is required")
	}
	content := args.First()
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("content cannot be empty")
	}

	namespace := cmd.String("namespace")
	var occurredAt *time.Time
	if oa := cmd.String("occurred-at"); oa != "" {
		t, err := time.Parse(time.RFC3339, oa)
		if err != nil {
			return fmt.Errorf("invalid occurred-at format: %w", err)
		}
		occurredAt = &t
	}

	bc := getBootstrap(cmd)
	id, err := bc.Brain.RememberWith(ctx, namespace, content, brain.RememberOptions{
		OccurredAt: occurredAt,
		Importance: float32(cmd.Float("importance")),
		Tags:       cmd.StringSlice("tag"),
	})
	if errors.Is(err, brain.ErrSocialCloser) {
		return printJSON(map[string]any{"skipped": true, "reason": err.Error()})
	}
	if err != nil {
		return err
	}

	output := map[string]any{"id": id, "message": "Memory remembered successfully"}
	return printJSON(output)
}

func recallCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("query argument is required")
	}
	query := args.First()
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("query cannot be empty")
	}

	namespaces := cmd.StringSlice("namespaces")
	limit := cmd.Int("limit")
	asOf, err := parseAsOf(cmd.String("as-of"))
	if err != nil {
		return err
	}
	from, err := parseAsOf(cmd.String("from"))
	if err != nil {
		return fmt.Errorf("invalid from: %w", err)
	}
	to, err := parseAsOf(cmd.String("to"))
	if err != nil {
		return fmt.Errorf("invalid to: %w", err)
	}
	knownFrom, err := parseAsOf(cmd.String("known-from"))
	if err != nil {
		return fmt.Errorf("invalid known-from: %w", err)
	}
	knownTo, err := parseAsOf(cmd.String("known-to"))
	if err != nil {
		return fmt.Errorf("invalid known-to: %w", err)
	}

	var mmr *float64
	if cmd.IsSet("mmr") {
		v := cmd.Float("mmr")
		mmr = &v
	}

	bc := getBootstrap(cmd)
	results, err := bc.Brain.RecallWith(ctx, namespaces, query, limit, brain.RecallOptions{AsOf: asOf, From: from, To: to, KnownFrom: knownFrom, KnownTo: knownTo, Tags: cmd.StringSlice("tag"), MMR: mmr})
	if err != nil {
		return err
	}

	return printJSON(results)
}

func reflectCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("query argument is required")
	}
	query := args.First()
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("query cannot be empty")
	}

	namespaces := cmd.StringSlice("namespaces")
	limit := cmd.Int("limit")
	contextStr := cmd.String("context")

	asOf, err := parseAsOf(cmd.String("as-of"))
	if err != nil {
		return err
	}
	from, err := parseAsOf(cmd.String("from"))
	if err != nil {
		return fmt.Errorf("invalid from: %w", err)
	}
	to, err := parseAsOf(cmd.String("to"))
	if err != nil {
		return fmt.Errorf("invalid to: %w", err)
	}
	knownFrom, err := parseAsOf(cmd.String("known-from"))
	if err != nil {
		return fmt.Errorf("invalid known-from: %w", err)
	}
	knownTo, err := parseAsOf(cmd.String("known-to"))
	if err != nil {
		return fmt.Errorf("invalid known-to: %w", err)
	}

	var types []string
	for _, t := range strings.Split(cmd.String("types"), ",") {
		if t = strings.TrimSpace(strings.ToLower(t)); t == "" {
			continue
		}
		if t != "fact" && t != "episode" && t != "failure" && t != "goal" {
			return fmt.Errorf("unknown type %q: use fact, episode, failure or goal", t)
		}
		types = append(types, t)
	}

	bc := getBootstrap(cmd)
	result, err := bc.Brain.Reflect(ctx, query, brain.ReflectOptions{
		Namespaces: namespaces,
		Limit:      limit,
		Context:    contextStr,
		AsOf:       asOf,
		From:       from,
		To:         to,
		KnownFrom:  knownFrom,
		KnownTo:    knownTo,
		Types:      types,
		Tags:       cmd.StringSlice("tag"),
	})
	if err != nil {
		return err
	}

	return printJSON(result)
}

func forgetCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("description argument is required")
	}
	query := args.First()
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("description cannot be empty")
	}

	namespaces := cmd.StringSlice("namespaces")
	bc := getBootstrap(cmd)
	if err := bc.Brain.ForgetEpisode(ctx, namespaces, query); err != nil {
		return err
	}

	return printJSON(map[string]string{"message": "Memory forgotten successfully"})
}

func purgeEpisodeCmd(ctx context.Context, cmd *cli.Command) error {
	return runPurge(cmd, "episode ID is required", func(bc *bootstrap.Context, id int64) (brain.PurgeCounts, error) {
		return bc.Brain.PurgeEpisode(ctx, id, purgeOptions(cmd))
	})
}

func purgeFactCmd(ctx context.Context, cmd *cli.Command) error {
	return runPurge(cmd, "fact ID is required", func(bc *bootstrap.Context, id int64) (brain.PurgeCounts, error) {
		return bc.Brain.PurgeFact(ctx, id, purgeOptions(cmd))
	})
}

// runPurge parses the id, runs one purge, and prints what was (or would be)
// removed. A dry run says so in the output so its counts cannot be mistaken
// for a completed delete.
func runPurge(cmd *cli.Command, missing string, purge func(*bootstrap.Context, int64) (brain.PurgeCounts, error)) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("%s", missing)
	}
	var id int64
	if _, err := fmt.Sscanf(args.First(), "%d", &id); err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}

	counts, err := purge(getBootstrap(cmd), id)
	if err != nil {
		return err
	}
	out := map[string]any{"removed": counts}
	if cmd.Bool("dry-run") {
		out["dry_run"] = true
		out["message"] = "nothing was deleted; re-run without --dry-run to apply"
	} else {
		out["message"] = "purged"
	}
	return printJSON(out)
}

func purgeOptions(cmd *cli.Command) brain.PurgeOptions {
	return brain.PurgeOptions{
		Reason: cmd.String("reason"),
		Actor:  cmd.String("actor"),
		DryRun: cmd.Bool("dry-run"),
	}
}

// purgeNamespaceCmd requires the slug twice: --confirm must repeat the
// positional argument. A namespace purge cascades to every memory filed under
// it, so a single typo must not be enough to run it.
func purgeNamespaceCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("namespace slug is required")
	}
	slug := args.First()
	if cmd.String("confirm") != slug {
		return fmt.Errorf("refusing to purge %q: pass --confirm %q to confirm", slug, slug)
	}

	counts, err := getBootstrap(cmd).Brain.PurgeNamespace(ctx, slug, purgeOptions(cmd))
	if err != nil {
		return err
	}
	out := map[string]any{"namespace": slug, "removed": counts}
	if cmd.Bool("dry-run") {
		out["dry_run"] = true
		out["message"] = "nothing was deleted; re-run without --dry-run to apply"
	} else {
		out["message"] = "purged"
	}
	return printJSON(out)
}

// retractCmd withdraws a claim and records why. Either a query (the episode it
// best matches) or --fact selects the target.
func retractCmd(ctx context.Context, cmd *cli.Command) (err error) {
	reason := cmd.String("reason")
	if factID := cmd.Int64("fact"); factID > 0 {
		var r *brain.Retraction
		r, err = getBootstrap(cmd).Brain.RetractFact(ctx, factID, reason, cmd.String("actor"))
		if err != nil {
			return err
		}
		return printJSON(r)
	}

	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("a description of the claim is required (or pass --fact <id>)")
	}
	query := args.First()
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("description cannot be empty")
	}

	r, err := getBootstrap(cmd).Brain.RetractEpisode(ctx, cmd.StringSlice("namespaces"), query, reason, cmd.String("actor"))
	if err != nil {
		return err
	}
	return printJSON(r)
}

func retractionsCmd(ctx context.Context, cmd *cli.Command) error {
	rows, err := getBootstrap(cmd).Brain.Retractions(ctx, cmd.StringSlice("namespaces"), cmd.Int("limit"))
	if err != nil {
		return err
	}
	return printJSON(rows)
}

func purgeLogCmd(ctx context.Context, cmd *cli.Command) error {
	rows, err := getBootstrap(cmd).Brain.PurgeLog(ctx, cmd.Int("limit"))
	if err != nil {
		return err
	}
	return printJSON(rows)
}

func factsListCmd(ctx context.Context, cmd *cli.Command) error {
	namespaces := cmd.StringSlice("namespaces")
	var since, until *time.Time
	if s := cmd.String("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return fmt.Errorf("invalid since format: %w", err)
		}
		since = &t
	}
	if u := cmd.String("until"); u != "" {
		t, err := time.Parse(time.RFC3339, u)
		if err != nil {
			return fmt.Errorf("invalid until format: %w", err)
		}
		until = &t
	}

	asOf, err := parseAsOf(cmd.String("as-of"))
	if err != nil {
		return err
	}
	from, err := parseAsOf(cmd.String("from"))
	if err != nil {
		return fmt.Errorf("invalid from: %w", err)
	}
	to, err := parseAsOf(cmd.String("to"))
	if err != nil {
		return fmt.Errorf("invalid to: %w", err)
	}
	knownFrom, err := parseAsOf(cmd.String("known-from"))
	if err != nil {
		return fmt.Errorf("invalid known-from: %w", err)
	}
	knownTo, err := parseAsOf(cmd.String("known-to"))
	if err != nil {
		return fmt.Errorf("invalid known-to: %w", err)
	}

	page := brain.Pagination{
		Offset: cmd.Int("offset"),
		Limit:  cmd.Int("limit"),
	}

	bc := getBootstrap(cmd)
	facts, err := bc.Brain.QueryFactsWindow(ctx, namespaces, since, until, asOf, from, to, knownFrom, knownTo, cmd.StringSlice("tag"), page)
	if err != nil {
		return err
	}
	return printJSON(facts)
}

// parseAsOf reads an optional point in time: RFC3339 or a plain date (midnight UTC).
// An empty string means "now" and returns nil.
func parseAsOf(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("invalid as-of %q: use RFC3339 (2026-09-01T12:00:00Z) or a date (2026-09-01)", s)
}

func getBootstrap(cmd *cli.Command) *bootstrap.Context {
	bc, _ := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	return bc
}

func printJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	fmt.Println(string(b))
	return nil
}

func mentalModelCreateCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() < 2 {
		return fmt.Errorf("usage: cortexiq mental-model create <name> <source-query> [--namespace /] [--tags a,b] [--max-tokens 2048]")
	}
	name := args.Get(0)
	query := args.Get(1)
	namespace := cmd.String("namespace")
	tags := cmd.StringSlice("tags")
	maxTokens := cmd.Int("max-tokens")

	bc := getBootstrap(cmd)
	// "/" (the default) means a global model; a named namespace scopes it there.
	var nsID *int64
	if namespace != "" && namespace != "/" {
		id, err := bc.Brain.NamespaceIDByPath(ctx, namespace)
		if err != nil {
			return fmt.Errorf("namespace %q not found", namespace)
		}
		nsID = &id
	}
	mm, err := bc.Brain.CreateMentalModel(ctx, brain.CreateMentalModelOptions{
		Name:        name,
		SourceQuery: query,
		Tags:        tags,
		MaxTokens:   maxTokens,
		NamespaceID: nsID,
	})
	if err != nil {
		return err
	}
	return printJSON(mm)
}

func mentalModelListCmd(ctx context.Context, cmd *cli.Command) error {
	bc := getBootstrap(cmd)
	namespaces := cmd.StringSlice("namespaces")
	if len(namespaces) == 0 {
		namespaces = []string{"/"}
	}
	tags := cmd.StringSlice("tags")
	items, err := bc.Brain.ListMentalModels(ctx, namespaces, tags, brain.Pagination{
		Limit:  cmd.Int("limit"),
		Offset: cmd.Int("offset"),
	})
	if err != nil {
		return err
	}
	return printJSON(map[string]any{"mental_models": items, "count": len(items)})
}

func mentalModelShowCmd(ctx context.Context, cmd *cli.Command) error {
	id, err := strconv.ParseInt(cmd.Args().First(), 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("usage: cortexiq mental-model show <id>")
	}
	bc := getBootstrap(cmd)
	mm, err := bc.Brain.GetMentalModel(ctx, id)
	if err != nil {
		return err
	}
	return printJSON(mm)
}

func mentalModelRefreshCmd(ctx context.Context, cmd *cli.Command) error {
	id, err := strconv.ParseInt(cmd.Args().First(), 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("usage: cortexiq mental-model refresh <id>")
	}
	bc := getBootstrap(cmd)
	mm, err := bc.Brain.RefreshMentalModel(ctx, id)
	if err != nil {
		return err
	}
	return printJSON(mm)
}

func mentalModelDeleteCmd(ctx context.Context, cmd *cli.Command) error {
	id, err := strconv.ParseInt(cmd.Args().First(), 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("usage: cortexiq mental-model delete <id>")
	}
	bc := getBootstrap(cmd)
	if err := bc.Brain.DeleteMentalModel(ctx, id); err != nil {
		return err
	}
	return printJSON(map[string]any{"ok": true, "id": id})
}

func mentalModelSearchCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("usage: cortexiq mental-model search <query> [--namespaces /,...] [--tags a,b] [--limit 5]")
	}
	query := args.First()
	namespaces := cmd.StringSlice("namespaces")
	if len(namespaces) == 0 {
		namespaces = []string{"/"}
	}
	tags := cmd.StringSlice("tags")
	bc := getBootstrap(cmd)
	results, err := bc.Brain.SearchMentalModels(ctx, namespaces, query, cmd.Int("limit"), tags)
	if err != nil {
		return err
	}
	return printJSON(map[string]any{"results": results, "count": len(results)})
}
