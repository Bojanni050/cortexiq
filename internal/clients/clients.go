// Modifications copyright (c) 2026 Bojan van der Heide

// Package clients writes a CortexIQ MCP server entry into the configuration of
// the agent clients installed on a machine. It is the only place that knows
// how each client wants to be told: the command layer decides *what* to
// connect, this package turns that into a file edit or an argv.
//
// Nothing here talks to the network or to a database. A client that connects
// to a running server is the caller's problem; this package only makes sure the
// configuration that points at it is right.
package clients

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// App names a supported MCP client. The string is also the value accepted on
// the command line, so it must stay stable.
type App string

const (
	// AppCursor reads a plain JSON file holding an mcpServers map.
	AppCursor App = "cursor"
	// AppClaude is configured through its own CLI so ~/.claude.json keeps the
	// state Claude Code stores next to the servers we add.
	AppClaude App = "claude"
	// AppOpenCode is configured through its own CLI for the same reason: its
	// config is JSONC, which a rewrite would strip of comments.
	AppOpenCode App = "opencode"
)

// All lists the clients in the order reports mention them.
func All() []App { return []App{AppCursor, AppClaude, AppOpenCode} }

// String makes an App printable as the name the command line uses.
func (a App) String() string { return string(a) }

// Parse turns a --app value into a list. "all" and the empty string mean every
// supported client; a comma-separated list names them explicitly. An unknown
// name is an error rather than a silent skip: a typo would otherwise leave the
// user with a client they believe is connected.
func Parse(spec string) ([]App, error) {
	spec = strings.TrimSpace(strings.ToLower(spec))
	if spec == "" || spec == "all" {
		return All(), nil
	}
	var apps []App
	seen := map[App]bool{}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		app := App(part)
		if !known(app) {
			return nil, fmt.Errorf("unknown client %q (known: cursor, claude, opencode)", part)
		}
		if seen[app] {
			continue
		}
		seen[app] = true
		apps = append(apps, app)
	}
	if len(apps) == 0 {
		return nil, errors.New("no client named")
	}
	return apps, nil
}

func known(app App) bool {
	for _, a := range All() {
		if a == app {
			return true
		}
	}
	return false
}

// Target is one MCP server entry as it is about to be written.
type Target struct {
	// Name is the key the client files the server under.
	Name string
	// URL is the MCP endpoint, which for CortexIQ is the streamable HTTP
	// transport (/mcp), not the older SSE one.
	URL string
	// Token is a literal secret. Exactly one of Token and EnvRef is set: a
	// literal works immediately, an EnvRef keeps the secret out of a config
	// file that may be copied, committed or shared.
	Token string
	// EnvRef is the name of the environment variable the client expands at
	// connection time.
	EnvRef string
}

// Validate rejects a target that would produce a client entry pointing nowhere.
func (t Target) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("server name is empty")
	}
	if strings.TrimSpace(t.URL) == "" {
		return errors.New("server URL is empty")
	}
	if t.Token == "" && t.EnvRef == "" {
		return errors.New("no token: pass one or name an environment variable to expand")
	}
	if t.Token != "" && t.EnvRef != "" {
		return errors.New("both a literal token and an environment reference are set; pick one")
	}
	return nil
}

// HeaderValue is the Authorization value the client will send. Each client has
// its own syntax for expanding an environment variable, which is the only
// difference between them.
func (t Target) HeaderValue(app App) string {
	if t.Token != "" {
		return "Bearer " + t.Token
	}
	switch app {
	case AppCursor:
		// Cursor expands ${env:NAME} in MCP headers.
		return "Bearer ${env:" + t.EnvRef + "}"
	case AppOpenCode:
		// OpenCode expands {env:NAME}.
		return "Bearer {env:" + t.EnvRef + "}"
	default:
		// Claude Code expands ${NAME} in MCP configuration.
		return "Bearer ${" + t.EnvRef + "}"
	}
}

// Entry is the server object a JSON-configured client stores. The three shapes
// differ in the type field: Cursor infers HTTP from the url, OpenCode wants
// "remote", Claude Code wants "http".
func (t Target) Entry(app App) map[string]any {
	headers := map[string]any{"Authorization": t.HeaderValue(app)}
	switch app {
	case AppOpenCode:
		return map[string]any{
			"type":    "remote",
			"url":     t.URL,
			"oauth":   false,
			"headers": headers,
		}
	default:
		return map[string]any{"url": t.URL, "headers": headers}
	}
}

// AddArgs is the command that registers the server through the client's own
// CLI. Cursor has no CLI for this, so it reports that its file must be edited.
func AddArgs(app App, t Target, global bool) ([]string, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	switch app {
	case AppClaude:
		return []string{
			"mcp", "add", "--transport", "http", t.Name, t.URL,
			"--header", "Authorization: " + t.HeaderValue(app),
			"--scope", scopeArg(global),
		}, nil
	case AppOpenCode:
		args := []string{"mcp", "add", t.Name, "--url", t.URL, "--header", "Authorization=" + t.HeaderValue(app)}
		if global {
			args = append(args, "--global")
		}
		return args, nil
	default:
		return nil, fmt.Errorf("%s is configured by writing its mcp.json", app)
	}
}

// RemoveArgs is the command that unregisters the server. OpenCode has no remove
// verb, so it reports ErrNoRemove and the caller tells the user which entry to
// delete.
func RemoveArgs(app App, name string, global bool) ([]string, error) {
	switch app {
	case AppClaude:
		return []string{"mcp", "remove", name, "--scope", scopeArg(global)}, nil
	case AppOpenCode:
		return nil, ErrNoRemove
	default:
		return nil, fmt.Errorf("%s is configured by writing its mcp.json", app)
	}
}

// ErrNoRemove is returned when a client offers no way to unregister a server
// other than editing its configuration.
var ErrNoRemove = errors.New("this client has no command to remove a server")

func scopeArg(global bool) string {
	if global {
		return "user"
	}
	return "local"
}

// ErrComments is returned when a configuration file cannot be rewritten without
// destroying something the user put there.
var ErrComments = errors.New("file contains comments; edit it by hand")

// HasComments reports whether a JSON document carries // or /* */ comments. The
// scan is deliberately textual: a comment marker inside a string does not
// count, and a document that has one outside a string does. Rewriting such a
// file would silently delete the comments, which is why callers refuse.
func HasComments(doc []byte) bool {
	inString := false
	escaped := false
	for i := 0; i < len(doc); i++ {
		c := doc[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '/':
			if i+1 < len(doc) && (doc[i+1] == '/' || doc[i+1] == '*') {
				return true
			}
		}
	}
	return false
}

// serverKey is where each client keeps its MCP servers inside its JSON document.
func serverKey(app App) string {
	if app == AppOpenCode {
		return "mcp"
	}
	return "mcpServers"
}

// MergeServer adds or replaces one entry in an MCP JSON document and reports
// whether anything changed. Every other key in the document is carried through
// untouched, because the file belongs to the user and the client, not to us.
// An empty document is treated as an empty object, so a client that has never
// been configured gets a valid file rather than an error.
func MergeServer(app App, doc []byte, name string, entry map[string]any) ([]byte, bool, error) {
	if HasComments(doc) {
		return nil, false, ErrComments
	}
	root, err := decodeObject(doc)
	if err != nil {
		return nil, false, err
	}
	key := serverKey(app)
	servers, err := decodeObject(root[key])
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", key, err)
	}
	want, err := json.Marshal(entry)
	if err != nil {
		return nil, false, err
	}
	if have, ok := servers[name]; ok && bytes.Equal(compact(have), want) {
		return doc, false, nil
	}
	servers[name] = want
	encoded, err := json.Marshal(servers)
	if err != nil {
		return nil, false, err
	}
	root[key] = encoded
	out, err := encodeObject(root)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

// RemoveServer deletes one entry and reports whether it was there. The document
// is left byte-identical when the entry is absent, so a repeated disconnect is
// not a rewrite.
func RemoveServer(app App, doc []byte, name string) ([]byte, bool, error) {
	if HasComments(doc) {
		return nil, false, ErrComments
	}
	root, err := decodeObject(doc)
	if err != nil {
		return nil, false, err
	}
	key := serverKey(app)
	servers, err := decodeObject(root[key])
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", key, err)
	}
	if _, ok := servers[name]; !ok {
		return doc, false, nil
	}
	delete(servers, name)
	encoded, err := json.Marshal(servers)
	if err != nil {
		return nil, false, err
	}
	root[key] = encoded
	out, err := encodeObject(root)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

// ServerNames lists the servers a document already defines, sorted so output is
// stable.
func ServerNames(app App, doc []byte) ([]string, error) {
	root, err := decodeObject(doc)
	if err != nil {
		return nil, err
	}
	servers, err := decodeObject(root[serverKey(app)])
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// decodeObject parses a JSON object, treating empty input as an empty object.
// Values are kept raw so unrelated entries keep the shape their client wrote,
// instead of being re-encoded through a map that would reorder their keys.
func decodeObject(raw []byte) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func encodeObject(obj map[string]json.RawMessage) ([]byte, error) {
	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// compact strips the whitespace an indented document carries. An entry that was
// written once comes back from the file pretty-printed, so comparing raw bytes
// would call every re-connect a change and rewrite the file for nothing.
func compact(raw []byte) []byte {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return raw
	}
	return buf.Bytes()
}
