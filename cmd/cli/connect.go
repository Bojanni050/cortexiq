// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Bojanni050/cortexiq/internal/clients"
	"github.com/urfave/cli/v3"
)

// defaultConnectURL points at a local server. The MCP endpoint is the
// streamable HTTP one: the older SSE transport at /sse still exists but no
// client needs it.
const defaultConnectURL = "http://127.0.0.1:8080/mcp"

// connectEnvRef is the variable connect writes into a client when the caller
// asks for a reference instead of a literal secret.
const connectEnvRef = "CORTEXIQ_API_TOKEN"

func connectCmd(ctx context.Context, cmd *cli.Command) error {
	apps, err := clients.Parse(cmd.String("app"))
	if err != nil {
		return err
	}
	target := clients.Target{
		Name:  cmd.String("name"),
		URL:   cmd.String("url"),
		Token: firstNonEmpty(cmd.String("token"), os.Getenv(connectEnvRef)),
	}
	if ref := cmd.String("env-ref"); ref != "" {
		target.Token = ""
		target.EnvRef = ref
	}
	if err := target.Validate(); err != nil {
		// The token is not something connect can invent: it has to come from
		// the server's configuration. Say where to get one instead of asking.
		return fmt.Errorf("%w (pass --token, set $%s, or run: cortexiq token new)", err, connectEnvRef)
	}

	global := cmd.Bool("global")
	dryRun := cmd.Bool("dry-run")
	var failed int
	for _, app := range apps {
		err := connectApp(ctx, app, target, global, dryRun)
		switch {
		case errors.Is(err, errNotInstalled):
			fmt.Printf("%-9s skipped  %s is not installed here\n", app, app)
		case err != nil:
			failed++
			fmt.Printf("%-9s failed   %v\n", app, err)
		default:
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d client(s) could not be configured", failed)
	}
	if !dryRun {
		fmt.Println()
		fmt.Println("Restart the client so it picks the server up. Writes that do not name a namespace")
		fmt.Println("land in the namespace bound to this token, so one token per app keeps the apps apart.")
	}
	return nil
}

// connectApp registers the server with one client. Cursor is a file edit; the
// other two are driven through their own CLI, which keeps whatever else those
// tools store in their configuration intact.
func connectApp(ctx context.Context, app clients.App, target clients.Target, global, dryRun bool) error {
	name := app.String()

	switch app {
	case clients.AppCursor:
		path, err := cursorConfigPath(global)
		if err != nil {
			return err
		}
		doc, err := readConfigFile(path)
		if err != nil {
			return err
		}
		next, changed, err := clients.MergeServer(app, doc, target.Name, target.Entry(app))
		if err != nil {
			return err
		}
		if !changed {
			fmt.Printf("%-9s ok       %s already set\n", name, path)
			return nil
		}
		if dryRun {
			fmt.Printf("%-9s would write %s\n", name, path)
			return nil
		}
		// The directory may not exist yet: Cursor creates ~/.cursor only once a
		// server is added, so a machine that has never used MCP has no
		// .cursor directory to write into.
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		// 0600: the file holds an API token when no environment reference is
		// used, and nothing outside this user needs to read it.
		if err := os.WriteFile(path, next, 0o600); err != nil {
			return err
		}
		fmt.Printf("%-9s written  %s\n", name, path)
		return nil

	case clients.AppClaude, clients.AppOpenCode:
		bin, err := exec.LookPath(string(app))
		if err != nil {
			return errNotInstalled
		}
		args, err := clients.AddArgs(app, target, global)
		if err != nil {
			return err
		}
		if dryRun {
			fmt.Printf("%-9s would run %s %s\n", name, bin, strings.Join(redactedArgs(app, args), " "))
			return nil
		}
		out, err := runClient(ctx, bin, args)
		if err != nil {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(out))
		}
		fmt.Printf("%-9s added    %s\n", name, strings.TrimSpace(firstLine(out)))
		return nil

	default:
		return fmt.Errorf("unsupported client %q", app)
	}
}

func disconnectCmd(ctx context.Context, cmd *cli.Command) error {
	apps, err := clients.Parse(cmd.String("app"))
	if err != nil {
		return err
	}
	global := cmd.Bool("global")
	dryRun := cmd.Bool("dry-run")
	name := cmd.String("name")

	var failed int
	for _, app := range apps {
		err := disconnectApp(ctx, app, name, global, dryRun)
		switch {
		case errors.Is(err, errNotInstalled):
			fmt.Printf("%-9s skipped  %s is not installed here\n", app, app)
		case err != nil:
			failed++
			fmt.Printf("%-9s failed   %v\n", app, err)
		default:
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d client(s) could not be reached", failed)
	}
	return nil
}

func disconnectApp(ctx context.Context, app clients.App, name string, global, dryRun bool) error {
	switch app {
	case clients.AppCursor:
		path, err := cursorConfigPath(global)
		if err != nil {
			return err
		}
		doc, err := readConfigFile(path)
		if err != nil {
			return err
		}
		next, removed, err := clients.RemoveServer(app, doc, name)
		if err != nil {
			return err
		}
		if !removed {
			fmt.Printf("%-9s ok       %s was not configured\n", app, path)
			return nil
		}
		if dryRun {
			fmt.Printf("%-9s would remove from %s\n", app, path)
			return nil
		}
		if err := os.WriteFile(path, next, 0o600); err != nil {
			return err
		}
		fmt.Printf("%-9s removed  %s\n", app, path)
		return nil

	case clients.AppClaude:
		bin, err := exec.LookPath(string(app))
		if err != nil {
			return errNotInstalled
		}
		args, _ := clients.RemoveArgs(app, name, global)
		if dryRun {
			fmt.Printf("%-9s would run %s %s\n", app, bin, strings.Join(args, " "))
			return nil
		}
		out, err := runClient(ctx, bin, args)
		if err != nil {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(out))
		}
		fmt.Printf("%-9s removed  %s\n", app, strings.TrimSpace(firstLine(out)))
		return nil

	case clients.AppOpenCode:
		// The client has no remove verb, and its config may carry comments, so
		// there is nothing safe to automate. Point at the entry instead.
		path := openCodeConfigHint(global)
		fmt.Printf("%-9s manual   OpenCode has no remove command: delete the %q entry in %s\n", app, name, path)
		return nil

	default:
		return fmt.Errorf("unsupported client %q", app)
	}
}

var errNotInstalled = errors.New("client not installed")

// cursorConfigPath is where Cursor reads its MCP servers from. The project file
// wins over the global one for the projects that have it, which is why connect
// has to be told which of the two the user wants.
func cursorConfigPath(global bool) (string, error) {
	if !global {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, ".cursor", "mcp.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cursor", "mcp.json"), nil
}

// openCodeConfigHint is the file the user has to edit by hand when disconnect
// cannot do it for them. OpenCode keeps its global configuration in a .config
// directory even on Windows, and either .json or .jsonc depending on how it was
// first written, so the existing file is preferred over the conventional path.
func openCodeConfigHint(global bool) string {
	if !global {
		if cwd, err := os.Getwd(); err == nil {
			return filepath.Join(cwd, "opencode.json")
		}
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	dirs := make([]string, 0, 2)
	if dir != "" {
		dirs = append(dirs, dir)
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".config"))
	}
	if dir, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, dir)
	}
	var fallback string
	for _, base := range dirs {
		for _, ext := range []string{"json", "jsonc"} {
			path := filepath.Join(base, "opencode", "opencode."+ext)
			if fallback == "" {
				fallback = path
			}
			if fileExists(path) {
				return path
			}
		}
	}
	return fallback
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// readConfigFile returns the file's contents, or nil when it does not exist:
// a client that has never been configured is not an error.
func readConfigFile(path string) ([]byte, error) {
	doc, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// runClient runs one of the client CLIs and returns its combined output. The
// output matters: these tools explain a refused configuration on stdout, and
// that text is more useful than the exit status.
func runClient(ctx context.Context, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// redactedArgs hides the token in a dry run. The secret is in the header value,
// and printing it would put it in shell history and CI logs. The header name
// and the "Bearer" scheme stay visible, because those are what the user is
// checking.
func redactedArgs(app clients.App, args []string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		if head, _, ok := strings.Cut(arg, "Bearer "); ok {
			arg = head + "Bearer ***"
		}
		out[i] = arg
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
