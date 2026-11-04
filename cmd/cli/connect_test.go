// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/clients"
	"github.com/urfave/cli/v3"
)

func TestRedactedArgsKeepsTheHeaderName(t *testing.T) {
	args := []string{"mcp", "add", "--header", "Authorization: Bearer s3cret"}
	got := redactedArgs(clients.AppClaude, args)
	if got[3] != "Authorization: Bearer ***" {
		t.Errorf("got %q", got[3])
	}
	if strings.Contains(strings.Join(got, " "), "s3cret") {
		t.Error("the secret survived redaction")
	}
	// A dry run prints the argv, so the original must be left alone for use.
	if args[3] != "Authorization: Bearer s3cret" {
		t.Error("redaction must not modify the caller's slice")
	}
}

func TestFirstNonEmptyAndFirstLine(t *testing.T) {
	if got := firstNonEmpty("", "  ", "second", "third"); got != "second" {
		t.Errorf("firstNonEmpty = %q", got)
	}
	if got := firstNonEmpty("", "  "); got != "" {
		t.Errorf("firstNonEmpty with no value = %q", got)
	}
	if got := firstLine("one\ntwo"); got != "one" {
		t.Errorf("firstLine = %q", got)
	}
	if got := firstLine("one"); got != "one" {
		t.Errorf("firstLine without a newline = %q", got)
	}
}

// withFakeHome points the user's home at a temporary directory so the tests
// never touch the real Cursor configuration.
func withFakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	return home
}

func connectAction(t *testing.T, args ...string) *cli.Command {
	t.Helper()
	var captured *cli.Command
	cmd := &cli.Command{
		Name: "connect",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "app", Value: "all"},
			&cli.StringFlag{Name: "url", Value: defaultConnectURL},
			&cli.StringFlag{Name: "token"},
			&cli.StringFlag{Name: "env-ref"},
			&cli.StringFlag{Name: "name", Value: "cortexiq"},
			&cli.BoolFlag{Name: "global"},
			&cli.BoolFlag{Name: "dry-run"},
		},
		Action: func(_ context.Context, c *cli.Command) error {
			captured = c
			return connectCmd(context.Background(), c)
		},
	}
	argv := append([]string{"connect"}, args...)
	if err := cmd.Run(context.Background(), argv); err != nil {
		t.Fatalf("connect %v: %v", args, err)
	}
	return captured
}

func TestConnectWritesCursorConfig(t *testing.T) {
	home := withFakeHome(t)
	connectAction(t, "--app", "cursor", "--global", "--token", "abc123", "--url", "http://srv:18100/mcp")

	path := filepath.Join(home, ".cursor", "mcp.json")
	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	var parsed struct {
		MCPServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(doc, &parsed); err != nil {
		t.Fatalf("config is not JSON: %v\n%s", err, doc)
	}
	entry, ok := parsed.MCPServers["cortexiq"]
	if !ok {
		t.Fatalf("no entry written: %s", doc)
	}
	if entry.URL != "http://srv:18100/mcp" {
		t.Errorf("url = %q", entry.URL)
	}
	if entry.Headers["Authorization"] != "Bearer abc123" {
		t.Errorf("header = %q", entry.Headers["Authorization"])
	}
}

func TestConnectEnvRefWritesNoSecret(t *testing.T) {
	home := withFakeHome(t)
	connectAction(t, "--app", "cursor", "--global", "--env-ref", "MY_TOKEN", "--url", "http://srv/mcp")

	doc, err := os.ReadFile(filepath.Join(home, ".cursor", "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), "Bearer ${env:MY_TOKEN}") {
		t.Errorf("expected a Cursor env reference, got:\n%s", doc)
	}
}

func TestConnectDoesNotDeleteOtherConfiguration(t *testing.T) {
	home := withFakeHome(t)
	path := filepath.Join(home, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := []byte(`{"theme":"dark","mcpServers":{"other":{"url":"http://other/sse"}}}`)
	if err := os.WriteFile(path, existing, 0o600); err != nil {
		t.Fatal(err)
	}

	connectAction(t, "--app", "cursor", "--global", "--token", "abc123", "--url", "http://srv/mcp")

	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	names, err := clients.ServerNames(clients.AppCursor, doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Errorf("expected both servers, got %v", names)
	}
	if !strings.Contains(string(doc), `"theme"`) {
		t.Errorf("an unrelated key was dropped:\n%s", doc)
	}
}

func TestConnectDryRunChangesNothing(t *testing.T) {
	home := withFakeHome(t)
	connectAction(t, "--app", "cursor", "--global", "--dry-run", "--token", "abc123")

	if _, err := os.Stat(filepath.Join(home, ".cursor", "mcp.json")); !os.IsNotExist(err) {
		t.Errorf("a dry run wrote the file (err=%v)", err)
	}
}

func TestConnectRejectsCommentsInsteadOfRewriting(t *testing.T) {
	home := withFakeHome(t)
	path := filepath.Join(home, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("{\n  // keep me\n  \"mcpServers\": {}\n}\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	err := connectApp(context.Background(), clients.AppCursor,
		clients.Target{Name: "cortexiq", URL: "http://srv/mcp", Token: "t"}, true, false)
	if err == nil {
		t.Fatal("expected the comment-laden file to be refused")
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(original) {
		t.Error("the file was rewritten despite the refusal")
	}
}

func TestDisconnectRemovesOnlyOurEntry(t *testing.T) {
	home := withFakeHome(t)
	path := filepath.Join(home, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	doc := []byte(`{"mcpServers":{"cortexiq":{"url":"u"},"other":{"url":"o"}}}`)
	if err := os.WriteFile(path, doc, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := disconnectApp(context.Background(), clients.AppCursor, "cortexiq", true, false); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	names, err := clients.ServerNames(clients.AppCursor, after)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "other" {
		t.Errorf("after disconnect: %v", names)
	}
}

func TestTokenNewValidatesInput(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no name", []string{}},
		{"bad namespace", []string{"--name", "x", "--namespace", "/Bad"}},
		{"root namespace", []string{"--name", "x", "--namespace", "/"}},
		{"bad scope", []string{"--name", "x", "--scope", "write"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cli.Command{
				Name: "new",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "name"},
					&cli.StringFlag{Name: "namespace"},
					&cli.StringFlag{Name: "scope"},
				},
				Action: tokenNewCmd,
			}
			argv := append([]string{"new"}, tc.args...)
			if err := cmd.Run(context.Background(), argv); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestNewTokenSecretIsUniqueAndURLSafe(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 16; i++ {
		secret, err := newTokenSecret()
		if err != nil {
			t.Fatal(err)
		}
		if len(secret) < 40 {
			t.Errorf("secret looks short: %q", secret)
		}
		if strings.ContainsAny(secret, "+/= ") {
			t.Errorf("secret needs quoting: %q", secret)
		}
		if seen[secret] {
			t.Errorf("repeated secret %q", secret)
		}
		seen[secret] = true
	}
}
