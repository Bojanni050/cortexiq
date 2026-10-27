// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/Bojanni050/cortexiq/internal/auth"
	"github.com/Bojanni050/cortexiq/internal/bootstrap"
	"github.com/Bojanni050/cortexiq/internal/brain"
	"github.com/Bojanni050/cortexiq/internal/scope"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/urfave/cli/v3"
)

//go:embed mcp_prompts.tmpl
var mcpPromptsFS string

var mcpTmpl *template.Template

func init() {
	var err error
	mcpTmpl, err = template.New("mcp_prompts").Parse(mcpPromptsFS)
	if err != nil {
		panic(fmt.Sprintf("parse mcp_prompts.tmpl: %v", err))
	}
}

func render(name string) string {
	var buf strings.Builder
	if err := mcpTmpl.ExecuteTemplate(&buf, name, nil); err != nil {
		return ""
	}
	return buf.String()
}

// skippedResult answers a write that was refused as phatic noise with a normal
// tool result instead of an error: the caller did nothing wrong, and an MCP error
// would invite a retry or an apology instead of the next step.
func skippedResult(err error) (*mcp.CallToolResult, error) {
	payload, _ := json.Marshal(map[string]any{"skipped": true, "reason": err.Error()})
	return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(payload)}}}, nil
}

// splitList parses the comma-separated form MCP arguments use for lists
// (namespaces, types, tags). Empty entries are dropped so a trailing comma is
// harmless.
func splitList(raw string) []string {
	var out []string
	for _, v := range strings.Split(raw, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// ErrNamespaceRequired is what a write tool answers when it was handed no
// namespace. It names the tool that answers "where does this belong" instead of
// only saying what is missing: the client reads this text and can correct itself
// in the next call, which a bare "namespace is required" does not allow.
var ErrNamespaceRequired = errors.New(
	`namespace is required: a memory filed without one disappears into everything else. ` +
		`Call list_namespaces and pass the path this belongs to, for example /projects/myapp. ` +
		`Pass "/" explicitly if it really belongs at the root.`)

// requiredNamespace validates the namespace a write was handed. An explicit "/"
// is a choice and is kept — the refusal catches the silence, it does not forbid
// the root.
func requiredNamespace(raw string) (string, error) {
	ns := strings.TrimSpace(raw)
	if ns == "" {
		return "", ErrNamespaceRequired
	}
	return ns, nil
}

// toolErrorResult turns a failed tool into a normal tool result carrying the
// error text. Without it the SDK answers a handler error with a JSON-RPC
// -32603, and hosted clients (Claude connectors, Le Chat) replace that with a
// generic "the connector returned an error or an invalid response": the caller
// learns nothing and cannot correct itself. A result with IsError is what the
// protocol asks for a tool that ran and failed, and the log line is the only
// trace a failed call leaves on the server.
func toolErrorResult(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := next(ctx, request)
		if err == nil {
			return result, nil
		}
		log.Printf("tool %s failed: %v", request.Params.Name, err)
		return mcp.NewToolResultError(err.Error()), nil
	}
}

func newMCPServer(bc *bootstrap.Context) *server.MCPServer {
	// WithInstructions is the field a client puts in front of the model on
	// initialize; WithDescription only labels the server. Both carry the same
	// text on purpose: the Prime Directive ("store without waiting to be asked")
	// only ever reaches the model through instructions, and a server that merely
	// describes itself gets a client that waits to be asked.
	primeDirective := render("server_description")
	mcpServer := server.NewMCPServer("cortexiq", "0.3.0",
		server.WithToolCapabilities(true),
		server.WithInstructions(primeDirective),
		server.WithDescription(primeDirective),
		server.WithToolHandlerMiddleware(toolErrorResult),
	)

	mcpServer.AddTool(mcp.NewTool("init",
		mcp.WithDescription(render("init_description")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		scaffold := []struct {
			slug        string
			name        string
			description string
		}{
			{"/self", "Self", "Agent's self-knowledge: capabilities, limits, preferences, and behavioral patterns."},
			{"/self/capabilities", "Capabilities", "What the agent can do well. Strengths and proven competencies."},
			{"/self/limits", "Limits", "What the agent struggles with or cannot do. Known failure modes."},
			{"/self/preferences", "Preferences", "How the agent works best. Behavioral patterns and standing instructions."},
		}

		var created []string
		for _, ns := range scaffold {
			_, err := bc.Brain.CreateNamespace(ctx, ns.slug, ns.name, ns.description)
			if err != nil {
				return nil, fmt.Errorf("init: create namespace %s: %w", ns.slug, err)
			}
			created = append(created, ns.slug)
		}

		result := map[string]any{"ok": true, "namespaces_created": created}
		b, _ := json.Marshal(result)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("remember",
		mcp.WithDescription(render("remember_description")),
		mcp.WithString("content", mcp.Description(render("remember_content")), mcp.Required()),
		mcp.WithString("namespace", mcp.Description(render("remember_namespace"))),
		mcp.WithString("occurred_at", mcp.Description(render("occurred_at_param"))),
		mcp.WithNumber("importance", mcp.Description(render("remember_importance")), mcp.DefaultNumber(0.5)),
		mcp.WithString("tags", mcp.Description(render("remember_tags"))),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		content := request.GetString("content", "")
		namespace, err := requiredNamespace(request.GetString("namespace", ""))
		if err != nil {
			return nil, err
		}
		importance := float32(request.GetFloat("importance", float64(brain.DefaultImportance)))
		occurredAt, err := parseAsOf(request.GetString("occurred_at", ""))
		if err != nil {
			return nil, fmt.Errorf("invalid occurred_at: %w", err)
		}

		id, decision, err := bc.Brain.RememberWithDecision(ctx, namespace, content, brain.RememberOptions{
			OccurredAt: occurredAt,
			Importance: importance,
			Tags:       splitList(request.GetString("tags", "")),
		})
		if errors.Is(err, brain.ErrSocialCloser) {
			return skippedResult(err)
		}
		if errors.Is(err, brain.ErrWriteFiltered) {
			b, _ := json.Marshal(map[string]any{
				"skipped":       true,
				"reason":        err.Error(),
				"secret_labels": decision.SecretLabels,
				"noise_score":   decision.NoiseScore,
			})
			return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
		}
		if err != nil {
			return nil, err
		}
		result := map[string]any{"id": id, "message": "Memory remembered successfully"}
		if decision.Action == "warn" {
			result["warning"] = "stored with warnings"
			result["reasons"] = decision.Reasons
			result["secret_labels"] = decision.SecretLabels
			result["noise_score"] = decision.NoiseScore
		}
		b, _ := json.Marshal(result)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("recall",
		mcp.WithDescription(render("recall_description")),
		mcp.WithString("query", mcp.Description(render("recall_query")), mcp.Required()),
		mcp.WithString("namespaces", mcp.Description(render("recall_namespaces"))),
		mcp.WithNumber("limit", mcp.Description(render("limit_param")), mcp.DefaultNumber(10)),
		mcp.WithString("as_of", mcp.Description(render("as_of_param"))),
		mcp.WithString("known_from", mcp.Description(render("known_from_param"))),
		mcp.WithString("known_to", mcp.Description(render("known_to_param"))),
		mcp.WithString("from", mcp.Description(render("from_param"))),
		mcp.WithString("to", mcp.Description(render("to_param"))),
		mcp.WithString("types", mcp.Description(render("recall_types"))),
		mcp.WithString("tags", mcp.Description(render("recall_tags"))),
		mcp.WithNumber("mmr", mcp.Description(render("recall_mmr"))),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := request.GetString("query", "")
		asOf, err := parseAsOf(request.GetString("as_of", ""))
		if err != nil {
			return nil, err
		}
		from, err := parseAsOf(request.GetString("from", ""))
		if err != nil {
			return nil, fmt.Errorf("invalid from: %w", err)
		}
		to, err := parseAsOf(request.GetString("to", ""))
		if err != nil {
			return nil, fmt.Errorf("invalid to: %w", err)
		}
		knownFrom, err := parseAsOf(request.GetString("known_from", ""))
		if err != nil {
			return nil, fmt.Errorf("invalid known_from: %w", err)
		}
		knownTo, err := parseAsOf(request.GetString("known_to", ""))
		if err != nil {
			return nil, fmt.Errorf("invalid known_to: %w", err)
		}
		var types []string
		for _, t := range splitList(request.GetString("types", "")) {
			// An unknown type would otherwise match neither store and return an
			// empty list that looks like "nothing remembered".
			if t != "fact" && t != "episode" && t != "failure" && t != "goal" {
				return nil, fmt.Errorf("unknown type %q: use fact, episode, failure or goal", t)
			}
			types = append(types, t)
		}
		nsRaw := request.GetString("namespaces", "/")
		limit := request.GetInt("limit", 10)

		var namespaces []string
		for _, ns := range strings.Split(nsRaw, ",") {
			if ns = strings.TrimSpace(ns); ns != "" {
				namespaces = append(namespaces, ns)
			}
		}

		var mmr *float64
		if request.GetFloat("mmr", -1) >= 0 {
			v := request.GetFloat("mmr", 0)
			mmr = &v
		}

		results, err := bc.Brain.RecallWith(ctx, namespaces, query, limit, brain.RecallOptions{AsOf: asOf, From: from, To: to, KnownFrom: knownFrom, KnownTo: knownTo, Types: types, Tags: splitList(request.GetString("tags", "")), MMR: mmr})
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(results)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("reflect",
		mcp.WithDescription(render("reflect_description")),
		mcp.WithString("query", mcp.Description(render("reflect_query")), mcp.Required()),
		mcp.WithString("namespaces", mcp.Description(render("reflect_namespaces"))),
		mcp.WithNumber("limit", mcp.Description(render("reflect_limit")), mcp.DefaultNumber(10)),
		mcp.WithString("context", mcp.Description(render("reflect_context"))),
		mcp.WithString("as_of", mcp.Description(render("as_of_param"))),
		mcp.WithString("from", mcp.Description(render("from_param"))),
		mcp.WithString("to", mcp.Description(render("to_param"))),
		mcp.WithString("known_from", mcp.Description(render("known_from_param"))),
		mcp.WithString("known_to", mcp.Description(render("known_to_param"))),
		mcp.WithString("types", mcp.Description(render("recall_types"))),
		mcp.WithString("tags", mcp.Description(render("reflect_tags"))),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := request.GetString("query", "")
		if strings.TrimSpace(query) == "" {
			return nil, fmt.Errorf("query is required")
		}

		asOf, err := parseAsOf(request.GetString("as_of", ""))
		if err != nil {
			return nil, err
		}
		from, err := parseAsOf(request.GetString("from", ""))
		if err != nil {
			return nil, fmt.Errorf("invalid from: %w", err)
		}
		to, err := parseAsOf(request.GetString("to", ""))
		if err != nil {
			return nil, fmt.Errorf("invalid to: %w", err)
		}
		knownFrom, err := parseAsOf(request.GetString("known_from", ""))
		if err != nil {
			return nil, fmt.Errorf("invalid known_from: %w", err)
		}
		knownTo, err := parseAsOf(request.GetString("known_to", ""))
		if err != nil {
			return nil, fmt.Errorf("invalid known_to: %w", err)
		}

		var types []string
		for _, t := range strings.Split(request.GetString("types", ""), ",") {
			if t = strings.TrimSpace(strings.ToLower(t)); t == "" {
				continue
			}
			if t != "fact" && t != "episode" && t != "failure" && t != "goal" {
				return nil, fmt.Errorf("unknown type %q: use fact, episode, failure or goal", t)
			}
			types = append(types, t)
		}

		var namespaces []string
		nsRaw := request.GetString("namespaces", "/")
		for _, ns := range strings.Split(nsRaw, ",") {
			if ns = strings.TrimSpace(ns); ns != "" {
				namespaces = append(namespaces, ns)
			}
		}

		limit := request.GetInt("limit", 10)
		contextStr := request.GetString("context", "")

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
			Tags:       splitList(request.GetString("tags", "")),
		})
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(result)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("forget",
		mcp.WithDescription(render("forget_description")),
		mcp.WithString("about", mcp.Description(render("forget_about")), mcp.Required()),
		mcp.WithString("namespaces", mcp.Description(render("namespaces_param"))),
		mcp.WithString("mode", mcp.Description(render("forget_mode"))),
		mcp.WithString("reason", mcp.Description(render("forget_reason"))),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		about := request.GetString("about", "")
		nsRaw := request.GetString("namespaces", "/")

		var namespaces []string
		for _, ns := range strings.Split(nsRaw, ",") {
			if ns = strings.TrimSpace(ns); ns != "" {
				namespaces = append(namespaces, ns)
			}
		}

		mode := strings.ToLower(strings.TrimSpace(request.GetString("mode", "forget")))
		reason := request.GetString("reason", "")

		var payload any = map[string]bool{"ok": true}
		switch mode {
		case "", "forget":
			if err := bc.Brain.ForgetEpisode(ctx, namespaces, about); err != nil {
				return nil, err
			}
		case "retract":
			r, err := bc.Brain.RetractEpisode(ctx, namespaces, about, reason, "mcp")
			if err != nil {
				return nil, err
			}
			payload = r
		case "purge":
			counts, err := bc.Brain.PurgeEpisodeByQuery(ctx, namespaces, about, brain.PurgeOptions{
				Reason: reason,
				Actor:  "mcp",
			})
			if err != nil {
				return nil, err
			}
			payload = counts
		default:
			return nil, fmt.Errorf("mode must be forget, retract or purge (got %q)", mode)
		}

		b, _ := json.Marshal(payload)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("mental_model_create",
		mcp.WithDescription("Create a mental model: a curated summary generated by reflect on a source query. Stored for fast retrieval and searched first in future reflect calls."),
		mcp.WithString("name", mcp.Description("Human-readable name for the mental model"), mcp.Required()),
		mcp.WithString("source_query", mcp.Description("The query to run to generate the mental model content"), mcp.Required()),
		mcp.WithString("namespaces", mcp.Description("File the model in this namespace and scope it there (it reads that namespace and its children). Omit or / for a global model that reads every namespace and is available everywhere.")),
		mcp.WithString("tags", mcp.Description("Comma-separated tags for scoped visibility")),
		mcp.WithNumber("max_tokens", mcp.Description("Maximum tokens for generated content (256-8192, default 2048)")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := request.GetString("name", "")
		query := request.GetString("source_query", "")
		if strings.TrimSpace(name) == "" || strings.TrimSpace(query) == "" {
			return nil, fmt.Errorf("name and source_query are required")
		}
		nsRaw := strings.TrimSpace(request.GetString("namespaces", "/"))
		var tags []string
		for _, t := range strings.Split(request.GetString("tags", ""), ",") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
		// "/" (the default) or an empty value means a global model: it reads every
		// namespace and is available to reflect everywhere. A named namespace
		// scopes it to that namespace and its children.
		var nsID *int64
		if nsRaw != "" && nsRaw != "/" {
			id, err := bc.Brain.NamespaceIDByPath(ctx, nsRaw)
			if err != nil {
				return nil, fmt.Errorf("namespace %q not found", nsRaw)
			}
			nsID = &id
		}
		maxTokens := request.GetInt("max_tokens", 2048)
		mm, err := bc.Brain.CreateMentalModel(ctx, brain.CreateMentalModelOptions{
			Name:        name,
			SourceQuery: query,
			Tags:        tags,
			MaxTokens:   maxTokens,
			NamespaceID: nsID,
		})
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(mm)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("mental_model_list",
		mcp.WithDescription("List mental models, optionally filtered by namespace and tags."),
		mcp.WithString("namespaces", mcp.Description("Comma-separated namespace paths (default: all)")),
		mcp.WithString("tags", mcp.Description("Comma-separated tags to filter by (any match)")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 100)")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nsRaw := request.GetString("namespaces", "")
		var slugs []string
		if nsRaw != "" {
			for _, ns := range strings.Split(nsRaw, ",") {
				if ns = strings.TrimSpace(ns); ns != "" {
					slugs = append(slugs, ns)
				}
			}
		}
		if len(slugs) == 0 {
			slugs = []string{"/"}
		}
		var tags []string
		for _, t := range strings.Split(request.GetString("tags", ""), ",") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
		limit := request.GetInt("limit", 100)
		models, err := bc.Brain.ListMentalModels(ctx, slugs, tags, brain.Pagination{Limit: limit})
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(map[string]any{"mental_models": models, "count": len(models)})
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("mental_model_get",
		mcp.WithDescription("Get a mental model by ID."),
		mcp.WithNumber("id", mcp.Description("Mental model ID"), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := int64(request.GetInt("id", 0))
		if id <= 0 {
			return nil, fmt.Errorf("id is required")
		}
		mm, err := bc.Brain.GetMentalModel(ctx, id)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(mm)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("mental_model_refresh",
		mcp.WithDescription("Refresh a mental model: regenerate its content by re-running reflect on its source query."),
		mcp.WithNumber("id", mcp.Description("Mental model ID"), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := int64(request.GetInt("id", 0))
		if id <= 0 {
			return nil, fmt.Errorf("id is required")
		}
		mm, err := bc.Brain.RefreshMentalModel(ctx, id)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(mm)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("mental_model_delete",
		mcp.WithDescription("Delete a mental model (soft delete)."),
		mcp.WithNumber("id", mcp.Description("Mental model ID"), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := int64(request.GetInt("id", 0))
		if id <= 0 {
			return nil, fmt.Errorf("id is required")
		}
		if err := bc.Brain.DeleteMentalModel(ctx, id); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: `{"ok": true}`}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("mental_model_search",
		mcp.WithDescription("Search mental models by semantic similarity. Returns curated summaries ranked by relevance to the query."),
		mcp.WithString("query", mcp.Description("Search query"), mcp.Required()),
		mcp.WithString("namespaces", mcp.Description("Comma-separated namespace paths (default: all)")),
		mcp.WithString("tags", mcp.Description("Comma-separated tags to filter by (any match)")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 5)")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := request.GetString("query", "")
		if strings.TrimSpace(query) == "" {
			return nil, fmt.Errorf("query is required")
		}
		nsRaw := request.GetString("namespaces", "")
		var slugs []string
		if nsRaw != "" {
			for _, ns := range strings.Split(nsRaw, ",") {
				if ns = strings.TrimSpace(ns); ns != "" {
					slugs = append(slugs, ns)
				}
			}
		}
		if len(slugs) == 0 {
			slugs = []string{"/"}
		}
		var tags []string
		for _, t := range strings.Split(request.GetString("tags", ""), ",") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
		limit := request.GetInt("limit", 5)
		results, err := bc.Brain.SearchMentalModels(ctx, slugs, query, limit, tags)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(map[string]any{"results": results, "count": len(results)})
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("rate_fact",
		mcp.WithDescription(render("rate_fact_description")),
		mcp.WithNumber("fact_id", mcp.Description(render("rate_fact_id")), mcp.Required()),
		mcp.WithBoolean("correct", mcp.Description(render("rate_fact_correct")), mcp.Required()),
		mcp.WithString("note", mcp.Description(render("rate_fact_note"))),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		verdict := -1
		if request.GetBool("correct", false) {
			verdict = 1
		}
		out, err := bc.Brain.RateFact(ctx, int64(request.GetInt("fact_id", 0)), verdict, request.GetString("note", ""))
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(out)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("consolidate",
		mcp.WithDescription(render("consolidate_description")),
		mcp.WithString("namespaces", mcp.Description(render("namespaces_param"))),
		mcp.WithString("tags", mcp.Description(render("consolidate_tags"))),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nsRaw := request.GetString("namespaces", "/")
		var namespaces []string
		for _, ns := range strings.Split(nsRaw, ",") {
			if ns = strings.TrimSpace(ns); ns != "" {
				namespaces = append(namespaces, ns)
			}
		}
		tags := splitList(request.GetString("tags", ""))

		ids, err := bc.Brain.ResolveNamespaceIDs(ctx, namespaces)
		if err != nil {
			return nil, err
		}

		var summaries []map[string]any
		for _, id := range ids {
			result, err := bc.Brain.ConsolidateByIDWithTags(ctx, id, tags)
			if err != nil {
				summaries = append(summaries, map[string]any{"namespace_id": id, "error": err.Error()})
				continue
			}
			summaries = append(summaries, map[string]any{
				"namespace":                    result.Namespace,
				"tag_filter":                   result.TagFilter,
				"episodes_read":                result.EpisodesRead,
				"facts_created":                result.FactsCreated,
				"facts_deduplicated":           result.FactsDeduplicated,
				"relationships_found":          result.RelationshipsFound,
				"causal_links_found":           result.CausalLinksFound,
				"patterns_found":               result.PatternsFound,
				"contradictions_found":         result.ContradictionsFound,
				"contradictions_auto_resolved": result.ContradictionsAutoResolved,
				"goals_annotated":              result.GoalsAnnotated,
				"goals_suggested_complete":     result.GoalsSuggestedComplete,
				"failure_repeats_detected":     result.FailureRepeatsDetected,
				"failure_patterns_found":       result.FailurePatternsFound,
				"hypotheses_auto_confirmed":    result.HypothesesAutoConfirmed,
				"hypotheses_auto_rejected":     result.HypothesesAutoRejected,
				"hypotheses_updated":           result.HypothesesUpdated,
				"facts_decayed":                result.FactsDecayed,
				"facts_expired":                result.FactsExpired,
				"llm_calls":                    result.LLMCalls,
				"duration":                     result.Duration.String(),
				"errors":                       result.Errors,
			})
		}
		b, _ := json.Marshal(summaries)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("set_context",
		mcp.WithDescription(render("set_context_description")),
		mcp.WithString("focus", mcp.Description(render("set_context_focus")), mcp.Required()),
		mcp.WithString("namespace", mcp.Description(render("context_namespace_param")), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		focus := request.GetString("focus", "")
		namespace := request.GetString("namespace", "")
		if namespace == "" {
			return nil, fmt.Errorf("namespace is required for context tools")
		}
		if err := bc.Brain.SetContext(ctx, namespace, focus, time.Now().UTC().Add(1*time.Hour)); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: `{"ok": true}`}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("get_context",
		mcp.WithDescription(render("get_context_description")),
		mcp.WithString("namespace", mcp.Description(render("context_namespace_param")), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := request.GetString("namespace", "")
		if namespace == "" {
			return nil, fmt.Errorf("namespace is required for context tools")
		}
		c, err := bc.Brain.GetContext(ctx, namespace)
		if err != nil {
			return nil, err
		}
		if c == nil {
			return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: `{"focus": ""}`}}}, nil
		}
		b, _ := json.Marshal(c)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("clear_context",
		mcp.WithDescription(render("clear_context_description")),
		mcp.WithString("namespace", mcp.Description(render("context_namespace_param")), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := request.GetString("namespace", "")
		if namespace == "" {
			return nil, fmt.Errorf("namespace is required for context tools")
		}
		if err := bc.Brain.ClearContext(ctx, namespace); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: `{"ok": true}`}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("list_namespaces",
		mcp.WithDescription(render("list_namespaces_description")),
		mcp.WithString("namespaces", mcp.Description(render("list_namespaces_namespaces"))),
		mcp.WithNumber("limit", mcp.Description(render("pagination_limit")), mcp.DefaultNumber(100)),
		mcp.WithNumber("offset", mcp.Description(render("pagination_offset")), mcp.DefaultNumber(0)),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nsRaw := request.GetString("namespaces", "")
		var patterns []string
		if nsRaw != "" {
			for _, ns := range strings.Split(nsRaw, ",") {
				if ns = strings.TrimSpace(ns); ns != "" {
					patterns = append(patterns, ns)
				}
			}
		}

		page := brain.Pagination{
			Limit:  request.GetInt("limit", 100),
			Offset: request.GetInt("offset", 0),
		}

		namespaces, err := bc.Brain.ListNamespaces(ctx, patterns, page)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(namespaces)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("list_tags",
		mcp.WithDescription(render("list_tags_description")),
		mcp.WithString("namespaces", mcp.Description(render("list_tags_namespaces"))),
		mcp.WithString("since", mcp.Description("Only count memories stored on or after this time (RFC3339 or YYYY-MM-DD). Use it for reporting a period.")),
		mcp.WithNumber("limit", mcp.Description(render("pagination_limit")), mcp.DefaultNumber(100)),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		since, err := parseAsOf(request.GetString("since", ""))
		if err != nil {
			return nil, fmt.Errorf("invalid since: %w", err)
		}
		tags, err := bc.Brain.ListTags(ctx, splitList(request.GetString("namespaces", "")), since, request.GetInt("limit", 100))
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(tags)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("memory_stats",
		mcp.WithDescription("Return memory statistics: total entries stored, database size, and the date range of the oldest and newest stored memories."),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		st, err := bc.Brain.Stats(ctx)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(st)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("create_namespace",
		mcp.WithDescription(render("create_namespace_description")),
		mcp.WithString("slug", mcp.Description(render("create_namespace_slug")), mcp.Required()),
		mcp.WithString("name", mcp.Description(render("create_namespace_name")), mcp.Required()),
		mcp.WithString("description", mcp.Description(render("create_namespace_description_param"))),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		slug := request.GetString("slug", "")
		name := request.GetString("name", slug)
		description := request.GetString("description", "")

		id, err := bc.Brain.CreateNamespace(ctx, slug, name, description)
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: fmt.Sprintf(`{"id": %d, "slug": "%s"}`, id, slug),
		}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("rename_namespace",
		mcp.WithDescription(render("rename_namespace_description")),
		mcp.WithString("from", mcp.Description(render("rename_namespace_from")), mcp.Required()),
		mcp.WithString("to", mcp.Description(render("rename_namespace_to")), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := bc.Brain.RenameNamespace(ctx, request.GetString("from", ""), request.GetString("to", ""))
		if err != nil {
			return nil, err
		}
		b, err := json.Marshal(res)
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("query_facts",
		mcp.WithDescription(render("query_facts_description")),
		mcp.WithString("namespaces", mcp.Description(render("namespaces_param"))),
		mcp.WithNumber("limit", mcp.Description(render("pagination_limit")), mcp.DefaultNumber(100)),
		mcp.WithNumber("offset", mcp.Description(render("pagination_offset")), mcp.DefaultNumber(0)),
		mcp.WithString("as_of", mcp.Description(render("as_of_param"))),
		mcp.WithString("known_from", mcp.Description(render("known_from_param"))),
		mcp.WithString("known_to", mcp.Description(render("known_to_param"))),
		mcp.WithString("from", mcp.Description(render("from_param"))),
		mcp.WithString("to", mcp.Description(render("to_param"))),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nsRaw := request.GetString("namespaces", "/")
		var namespaces []string
		for _, ns := range strings.Split(nsRaw, ",") {
			if ns = strings.TrimSpace(ns); ns != "" {
				namespaces = append(namespaces, ns)
			}
		}

		page := brain.Pagination{
			Limit:  request.GetInt("limit", 100),
			Offset: request.GetInt("offset", 0),
		}
		asOf, err := parseAsOf(request.GetString("as_of", ""))
		if err != nil {
			return nil, err
		}
		from, err := parseAsOf(request.GetString("from", ""))
		if err != nil {
			return nil, fmt.Errorf("invalid from: %w", err)
		}
		to, err := parseAsOf(request.GetString("to", ""))
		if err != nil {
			return nil, fmt.Errorf("invalid to: %w", err)
		}
		knownFrom, err := parseAsOf(request.GetString("known_from", ""))
		if err != nil {
			return nil, fmt.Errorf("invalid known_from: %w", err)
		}
		knownTo, err := parseAsOf(request.GetString("known_to", ""))
		if err != nil {
			return nil, fmt.Errorf("invalid known_to: %w", err)
		}

		facts, err := bc.Brain.QueryFactsWindow(ctx, namespaces, nil, nil, asOf, from, to, knownFrom, knownTo, nil, page)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(facts)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("query_relationships",
		mcp.WithDescription(render("query_relationships_description")),
		mcp.WithString("namespaces", mcp.Description(render("namespaces_param"))),
		mcp.WithNumber("limit", mcp.Description(render("pagination_limit")), mcp.DefaultNumber(100)),
		mcp.WithNumber("offset", mcp.Description(render("pagination_offset")), mcp.DefaultNumber(0)),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nsRaw := request.GetString("namespaces", "/")
		var namespaces []string
		for _, ns := range strings.Split(nsRaw, ",") {
			if ns = strings.TrimSpace(ns); ns != "" {
				namespaces = append(namespaces, ns)
			}
		}

		page := brain.Pagination{
			Limit:  request.GetInt("limit", 100),
			Offset: request.GetInt("offset", 0),
		}

		rels, err := bc.Brain.QueryRelationships(ctx, namespaces, page)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(rels)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("list_contradictions",
		mcp.WithDescription(render("list_contradictions_description")),
		mcp.WithString("namespaces", mcp.Description(render("namespaces_param"))),
		mcp.WithNumber("limit", mcp.Description(render("pagination_limit")), mcp.DefaultNumber(100)),
		mcp.WithNumber("offset", mcp.Description(render("pagination_offset")), mcp.DefaultNumber(0)),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nsRaw := request.GetString("namespaces", "/")
		var namespaces []string
		for _, ns := range strings.Split(nsRaw, ",") {
			if ns = strings.TrimSpace(ns); ns != "" {
				namespaces = append(namespaces, ns)
			}
		}

		page := brain.Pagination{
			Limit:  request.GetInt("limit", 100),
			Offset: request.GetInt("offset", 0),
		}

		contradictions, err := bc.Brain.ListContradictions(ctx, namespaces, "", page)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(contradictions)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("resolve_contradiction",
		mcp.WithDescription(render("resolve_contradiction_description")),
		mcp.WithNumber("id", mcp.Description(render("resolve_contradiction_id")), mcp.Required()),
		mcp.WithString("resolution", mcp.Description(render("resolve_contradiction_resolution"))),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := request.GetInt("id", 0)
		resolution := request.GetString("resolution", "resolved")

		if err := bc.Brain.ResolveContradiction(ctx, int64(id), resolution); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: `{"ok": true}`}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("list_causal_links",
		mcp.WithDescription(render("list_causal_links_description")),
		mcp.WithString("namespaces", mcp.Description(render("namespaces_param"))),
		mcp.WithNumber("limit", mcp.Description(render("pagination_limit")), mcp.DefaultNumber(100)),
		mcp.WithNumber("offset", mcp.Description(render("pagination_offset")), mcp.DefaultNumber(0)),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nsRaw := request.GetString("namespaces", "/")
		var namespaces []string
		for _, ns := range strings.Split(nsRaw, ",") {
			if ns = strings.TrimSpace(ns); ns != "" {
				namespaces = append(namespaces, ns)
			}
		}

		page := brain.Pagination{
			Limit:  request.GetInt("limit", 100),
			Offset: request.GetInt("offset", 0),
		}

		links, err := bc.Brain.ListCausalLinks(ctx, namespaces, page)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(links)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("create_causal_link",
		mcp.WithDescription(render("create_causal_link_description")),
		mcp.WithNumber("cause_id", mcp.Description(render("create_causal_link_cause_id")), mcp.Required()),
		mcp.WithNumber("effect_id", mcp.Description(render("create_causal_link_effect_id")), mcp.Required()),
		mcp.WithNumber("confidence", mcp.Description(render("create_causal_link_confidence")), mcp.DefaultNumber(0.8)),
		mcp.WithString("namespace", mcp.Description(render("namespace_param"))),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		causeID := request.GetInt("cause_id", 0)
		effectID := request.GetInt("effect_id", 0)
		confidence := float32(request.GetFloat("confidence", 0.8))
		namespace, err := requiredNamespace(request.GetString("namespace", ""))
		if err != nil {
			return nil, err
		}

		nsID, err := bc.Brain.NamespaceIDByPath(ctx, namespace)
		if err != nil {
			return nil, err
		}

		link, err := bc.Brain.CreateCausalLink(ctx, nsID, int64(causeID), int64(effectID), confidence)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(link)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("trace_causal_chain",
		mcp.WithDescription(render("trace_causal_chain_description")),
		mcp.WithNumber("fact_id", mcp.Description(render("trace_causal_chain_fact_id")), mcp.Required()),
		mcp.WithString("direction", mcp.Description(render("trace_causal_chain_direction")), mcp.DefaultString("forward")),
		mcp.WithNumber("max_depth", mcp.Description(render("trace_causal_chain_max_depth")), mcp.DefaultNumber(10)),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		factID := request.GetInt("fact_id", 0)
		direction := request.GetString("direction", "forward")
		maxDepth := request.GetInt("max_depth", 10)

		chain, err := bc.Brain.TraceCausalChain(ctx, int64(factID), direction, maxDepth)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(chain)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("list_hypotheses",
		mcp.WithDescription(render("list_hypotheses_description")),
		mcp.WithString("namespaces", mcp.Description(render("namespaces_param"))),
		mcp.WithString("status", mcp.Description(render("list_hypotheses_status"))),
		mcp.WithNumber("limit", mcp.Description(render("pagination_limit")), mcp.DefaultNumber(100)),
		mcp.WithNumber("offset", mcp.Description(render("pagination_offset")), mcp.DefaultNumber(0)),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nsRaw := request.GetString("namespaces", "/")
		var namespaces []string
		for _, ns := range strings.Split(nsRaw, ",") {
			if ns = strings.TrimSpace(ns); ns != "" {
				namespaces = append(namespaces, ns)
			}
		}

		page := brain.Pagination{
			Limit:  request.GetInt("limit", 100),
			Offset: request.GetInt("offset", 0),
		}
		status := request.GetString("status", "")

		hypotheses, err := bc.Brain.ListHypotheses(ctx, namespaces, status, page)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(hypotheses)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("create_hypothesis",
		mcp.WithDescription(render("create_hypothesis_description")),
		mcp.WithString("content", mcp.Description(render("create_hypothesis_content")), mcp.Required()),
		mcp.WithString("verification_plan", mcp.Description(render("create_hypothesis_verification_plan")), mcp.Required()),
		mcp.WithNumber("confidence", mcp.Description(render("create_hypothesis_confidence")), mcp.DefaultNumber(0.5)),
		mcp.WithString("namespace", mcp.Description(render("namespace_param"))),
		mcp.WithString("source_fact_ids", mcp.Description(render("create_hypothesis_source_fact_ids"))),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		content := request.GetString("content", "")
		verificationPlan := request.GetString("verification_plan", "")
		confidence := float32(request.GetFloat("confidence", 0.5))
		namespace, err := requiredNamespace(request.GetString("namespace", ""))
		if err != nil {
			return nil, err
		}

		var sourceFactIDs []int64
		if raw := request.GetString("source_fact_ids", ""); raw != "" {
			for _, s := range strings.Split(raw, ",") {
				if id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
					sourceFactIDs = append(sourceFactIDs, id)
				}
			}
		}

		nsID, err := bc.Brain.NamespaceIDByPath(ctx, namespace)
		if err != nil {
			return nil, err
		}

		h, err := bc.Brain.CreateHypothesis(ctx, nsID, content, verificationPlan, confidence, sourceFactIDs)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(h)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("confirm_hypothesis",
		mcp.WithDescription(render("confirm_hypothesis_description")),
		mcp.WithNumber("id", mcp.Description(render("confirm_hypothesis_id")), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := request.GetInt("id", 0)

		h, f, err := bc.Brain.ConfirmHypothesis(ctx, int64(id))
		if err != nil {
			return nil, err
		}
		result := map[string]any{
			"hypothesis": h,
			"fact":       f,
		}
		b, _ := json.Marshal(result)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("reject_hypothesis",
		mcp.WithDescription(render("reject_hypothesis_description")),
		mcp.WithNumber("id", mcp.Description(render("reject_hypothesis_id")), mcp.Required()),
		mcp.WithString("reason", mcp.Description(render("reject_hypothesis_reason"))),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := request.GetInt("id", 0)
		reason := request.GetString("reason", "")

		h, err := bc.Brain.RejectHypothesis(ctx, int64(id), reason)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(h)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("list_goals",
		mcp.WithDescription(render("list_goals_description")),
		mcp.WithString("namespaces", mcp.Description(render("namespaces_param"))),
		mcp.WithString("status", mcp.Description(render("list_goals_status"))),
		mcp.WithNumber("parent_id", mcp.Description(render("list_goals_parent_id"))),
		mcp.WithNumber("limit", mcp.Description(render("pagination_limit")), mcp.DefaultNumber(100)),
		mcp.WithNumber("offset", mcp.Description(render("pagination_offset")), mcp.DefaultNumber(0)),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nsRaw := request.GetString("namespaces", "/")
		var namespaces []string
		for _, ns := range strings.Split(nsRaw, ",") {
			if ns = strings.TrimSpace(ns); ns != "" {
				namespaces = append(namespaces, ns)
			}
		}

		page := brain.Pagination{
			Limit:  request.GetInt("limit", 100),
			Offset: request.GetInt("offset", 0),
		}
		status := request.GetString("status", "")

		var parentID *int64
		if pid := request.GetInt("parent_id", 0); pid != 0 {
			pid64 := int64(pid)
			parentID = &pid64
		}

		goals, err := bc.Brain.ListGoals(ctx, namespaces, status, parentID, page)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(goals)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("create_goal",
		mcp.WithDescription(render("create_goal_description")),
		mcp.WithString("content", mcp.Description(render("create_goal_content")), mcp.Required()),
		mcp.WithNumber("parent_id", mcp.Description(render("create_goal_parent_id"))),
		mcp.WithNumber("priority", mcp.Description(render("create_goal_priority")), mcp.DefaultNumber(0)),
		mcp.WithString("namespace", mcp.Description(render("namespace_param"))),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		content := request.GetString("content", "")
		priority := request.GetInt("priority", 0)
		namespace, err := requiredNamespace(request.GetString("namespace", ""))
		if err != nil {
			return nil, err
		}

		var parentID *int64
		if pid := request.GetInt("parent_id", 0); pid != 0 {
			pid64 := int64(pid)
			parentID = &pid64
		}

		nsID, err := bc.Brain.NamespaceIDByPath(ctx, namespace)
		if err != nil {
			return nil, err
		}

		g, err := bc.Brain.CreateGoal(ctx, nsID, content, parentID, priority)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(g)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("complete_goal",
		mcp.WithDescription(render("complete_goal_description")),
		mcp.WithNumber("id", mcp.Description(render("complete_goal_id")), mcp.Required()),
		mcp.WithString("notes", mcp.Description(render("complete_goal_notes"))),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := request.GetInt("id", 0)
		notes := request.GetString("notes", "")

		g, err := bc.Brain.CompleteGoal(ctx, int64(id), notes)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(g)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("abandon_goal",
		mcp.WithDescription(render("abandon_goal_description")),
		mcp.WithNumber("id", mcp.Description(render("abandon_goal_id")), mcp.Required()),
		mcp.WithString("notes", mcp.Description(render("abandon_goal_notes"))),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := request.GetInt("id", 0)
		notes := request.GetString("notes", "")

		g, err := bc.Brain.AbandonGoal(ctx, int64(id), notes)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(g)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("list_failures",
		mcp.WithDescription(render("list_failures_description")),
		mcp.WithString("namespaces", mcp.Description(render("namespaces_param"))),
		mcp.WithNumber("goal_id", mcp.Description(render("list_failures_goal_id"))),
		mcp.WithNumber("limit", mcp.Description(render("pagination_limit")), mcp.DefaultNumber(100)),
		mcp.WithNumber("offset", mcp.Description(render("pagination_offset")), mcp.DefaultNumber(0)),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nsRaw := request.GetString("namespaces", "/")
		var namespaces []string
		for _, ns := range strings.Split(nsRaw, ",") {
			if ns = strings.TrimSpace(ns); ns != "" {
				namespaces = append(namespaces, ns)
			}
		}

		page := brain.Pagination{
			Limit:  request.GetInt("limit", 100),
			Offset: request.GetInt("offset", 0),
		}

		var goalID *int64
		if gid := request.GetInt("goal_id", 0); gid != 0 {
			gid64 := int64(gid)
			goalID = &gid64
		}

		failures, err := bc.Brain.ListFailures(ctx, namespaces, goalID, page)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(failures)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("create_failure",
		mcp.WithDescription(render("create_failure_description")),
		mcp.WithString("content", mcp.Description(render("create_failure_content")), mcp.Required()),
		mcp.WithString("reason", mcp.Description(render("create_failure_reason")), mcp.Required()),
		mcp.WithString("lesson", mcp.Description(render("create_failure_lesson")), mcp.Required()),
		mcp.WithNumber("goal_id", mcp.Description(render("create_failure_goal_id"))),
		mcp.WithString("namespace", mcp.Description(render("namespace_param"))),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		content := request.GetString("content", "")
		reason := request.GetString("reason", "")
		lesson := request.GetString("lesson", "")
		namespace, err := requiredNamespace(request.GetString("namespace", ""))
		if err != nil {
			return nil, err
		}

		var goalID *int64
		if gid := request.GetInt("goal_id", 0); gid != 0 {
			gid64 := int64(gid)
			goalID = &gid64
		}

		nsID, err := bc.Brain.NamespaceIDByPath(ctx, namespace)
		if err != nil {
			return nil, err
		}

		f, err := bc.Brain.CreateFailure(ctx, nsID, content, reason, lesson, goalID)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(f)
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(b)}}}, nil
	})

	mcpServer.AddTool(mcp.NewTool("delete_failure",
		mcp.WithDescription(render("delete_failure_description")),
		mcp.WithNumber("id", mcp.Description(render("delete_failure_id")), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := request.GetInt("id", 0)

		if err := bc.Brain.DeleteFailure(ctx, int64(id)); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: `{"ok": true}`}}}, nil
	})

	return mcpServer
}

// readOnlyTools are the MCP tools a read-only token may call. Everything else
// (remember, consolidate, create_*, delete_*, ...) is rejected before it reaches
// the server.
var readOnlyTools = map[string]bool{
	"recall":              true,
	"query_facts":         true,
	"query_relationships": true,
	"list_namespaces":     true,
	"list_tags":           true,
	"list_goals":          true,
	"list_hypotheses":     true,
	"list_contradictions": true,
	"list_failures":       true,
	"list_causal_links":   true,
	"get_context":         true,
	"trace_causal_chain":  true,
	"memory_stats":        true,
}

// readOnlyMCP rejects tools/call requests for mutating tools when the token is
// read-only. The MCP transport is POST-based, so the tool name is read from the
// JSON-RPC body; other methods (initialize, tools/list, ping) are read-only.
func readOnlyMCP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !scope.ReadOnly(r.Context()) || r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var msg struct {
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		_ = json.Unmarshal(body, &msg)
		if msg.Method == "tools/call" && !readOnlyTools[msg.Params.Name] {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"read-only token: tool not allowed"}` + "\n"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// mcpKeepAliveInterval is how often an idle MCP stream is kept warm: a JSON-RPC
// ping on the SSE transport, a heartbeat on the streamable HTTP GET stream. The
// MCP javascript client (mcp-remote/undici) aborts a silent response body after
// 300s, and reverse proxies in front of us are often stricter.
const mcpKeepAliveInterval = 15 * time.Second

// mcpHandler routes the two MCP transports CortexIQ serves: SSE under /sse
// (with its POST channel /message) for clients that still speak it, and
// Streamable HTTP under /mcp for the rest. Hosted clients (Mistral Le Chat and
// Vibe, monday, Gemini Enterprise) only support Streamable HTTP.
func mcpHandler(sse *server.SSEServer, streamable *server.StreamableHTTPServer) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/sse", sse)
	mux.Handle("/message", sse)
	mux.Handle("/mcp", streamable)
	return mux
}

func mcpServeCmd(ctx context.Context, cmd *cli.Command) error {
	bc := getBootstrap(cmd)
	// Fetch the embedding model now, so the first recall after a restart is warm.
	bc.WarmEmbedder(ctx)
	mcpServer := newMCPServer(bc)

	host := cmd.String("host")
	port := cmd.String("port")
	addr := host + ":" + port

	// The SSE server is mounted behind the same Bearer-token check as the
	// dashboard API. An empty token set leaves MCP open (as before).
	httpSrv := &http.Server{Addr: addr}
	// The SSE stream carries no traffic while the agent is thinking, and clients
	// sit behind proxies (tailscale serve/funnel) that drop idle connections.
	// undici/mcp-remote gives up after 300s without bytes, so we ping well
	// inside that window to keep the stream alive.
	sseServer := server.NewSSEServer(mcpServer,
		server.WithHTTPServer(httpSrv),
		server.WithKeepAliveInterval(mcpKeepAliveInterval),
	)
	// Streamable HTTP is a second, request-based transport on the same server.
	// Its optional long-lived GET stream gets the same treatment as SSE.
	streamableServer := server.NewStreamableHTTPServer(mcpServer,
		server.WithHeartbeatInterval(mcpKeepAliveInterval),
	)
	httpSrv.Handler = auth.Require(bc.Auth, readOnlyMCP(mcpHandler(sseServer, streamableServer)))
	if !bc.Auth.Enabled() {
		log.Println("WARNING: no API token configured; the MCP server is unauthenticated")
	}

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup

	if cmd.Bool("with-consolidation") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runConsolidationTicker(ctx, bc, cmd)
		}()
	}

	fmt.Printf("Starting MCP server on %s (sse: /sse, streamable http: /mcp)\n", addr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- sseServer.Start(addr)
	}()

	select {
	case <-ctx.Done():
		fmt.Println("\nMCP server shutting down")
		wg.Wait()
		return nil
	case err := <-errCh:
		cancel()
		wg.Wait()
		return err
	}
}

func mcpExecuteCmd(ctx context.Context, cmd *cli.Command) error {
	bc := getBootstrap(cmd)
	mcpServer := newMCPServer(bc)

	var wg sync.WaitGroup

	if cmd.Bool("with-consolidation") {
		ctx2, cancel := context.WithCancel(ctx)
		defer cancel()
		wg.Add(1)
		go func() {
			defer wg.Done()
			runConsolidationTicker(ctx2, bc, cmd)
		}()
	}

	err := server.ServeStdio(mcpServer)
	wg.Wait()
	return err
}

func runConsolidationTicker(ctx context.Context, bc *bootstrap.Context, cmd *cli.Command) {
	if err := runSchedule(ctx, bc, cmd.Duration("consolidate-interval"), cmd.StringSlice("consolidate-namespaces")); err != nil {
		log.Printf("Consolidation service stopped: %v", err)
	}
}
