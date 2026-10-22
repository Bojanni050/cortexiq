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
	id, err := bc.Brain.RememberWithImportance(ctx, namespace, content, occurredAt, float32(cmd.Float("importance")))
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
	results, err := bc.Brain.RecallWith(ctx, namespaces, query, limit, brain.RecallOptions{AsOf: asOf, From: from, To: to, KnownFrom: knownFrom, KnownTo: knownTo, MMR: mmr})
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
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("episode ID is required")
	}
	var id int64
	if _, err := fmt.Sscanf(args.First(), "%d", &id); err != nil {
		return fmt.Errorf("invalid episode ID: %w", err)
	}

	bc := getBootstrap(cmd)
	if err := bc.Brain.PurgeEpisode(ctx, id); err != nil {
		return err
	}
	return printJSON(map[string]string{"message": "Episode purged successfully"})
}

func purgeFactCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("fact ID is required")
	}
	var id int64
	if _, err := fmt.Sscanf(args.First(), "%d", &id); err != nil {
		return fmt.Errorf("invalid fact ID: %w", err)
	}

	bc := getBootstrap(cmd)
	if err := bc.Brain.PurgeFact(ctx, id); err != nil {
		return err
	}
	return printJSON(map[string]string{"message": "Fact purged successfully"})
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
	facts, err := bc.Brain.QueryFactsWindow(ctx, namespaces, since, until, asOf, from, to, knownFrom, knownTo, page)
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
