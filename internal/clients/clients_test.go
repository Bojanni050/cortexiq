// Modifications copyright (c) 2026 Bojan van der Heide
package clients

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want []App
	}{
		{"", All()},
		{"all", All()},
		{"ALL", All()},
		{"cursor", []App{AppCursor}},
		{"cursor,opencode", []App{AppCursor, AppOpenCode}},
		{" cursor , claude ", []App{AppCursor, AppClaude}},
		{"claude,claude", []App{AppClaude}},
	}
	for _, tc := range cases {
		got, err := Parse(tc.in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.in, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Parse(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseRejectsUnknown(t *testing.T) {
	// A typo must fail loudly: a skipped client looks identical to a connected
	// one until the user wonders why nothing is remembered.
	for _, in := range []string{"vim", "cursor,vim", ","} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) accepted an unknown client", in)
		}
	}
}

func TestTargetValidate(t *testing.T) {
	ok := Target{Name: "cortexiq", URL: "http://x/mcp", Token: "s3cret"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid target rejected: %v", err)
	}
	if err := (Target{Name: "cortexiq", URL: "http://x/mcp", EnvRef: "CORTEXIQ_API_TOKEN"}).Validate(); err != nil {
		t.Fatalf("env reference rejected: %v", err)
	}
	bad := []Target{
		{URL: "http://x/mcp", Token: "t"},
		{Name: "cortexiq", Token: "t"},
		{Name: "cortexiq", URL: "http://x/mcp"},
		{Name: "cortexiq", URL: "http://x/mcp", Token: "t", EnvRef: "V"},
	}
	for i, target := range bad {
		if err := target.Validate(); err == nil {
			t.Errorf("case %d: accepted an incomplete target", i)
		}
	}
}

func TestHeaderValueUsesEachClientsEnvSyntax(t *testing.T) {
	ref := Target{Name: "cortexiq", URL: "u", EnvRef: "MY_TOKEN"}
	cases := map[App]string{
		AppCursor:   "Bearer ${env:MY_TOKEN}",
		AppClaude:   "Bearer ${MY_TOKEN}",
		AppOpenCode: "Bearer {env:MY_TOKEN}",
	}
	for app, want := range cases {
		if got := ref.HeaderValue(app); got != want {
			t.Errorf("%s: got %q, want %q", app, got, want)
		}
	}
	literal := Target{Name: "cortexiq", URL: "u", Token: "abc"}
	if got := literal.HeaderValue(AppCursor); got != "Bearer abc" {
		t.Errorf("literal token: got %q", got)
	}
}

func TestAddArgs(t *testing.T) {
	target := Target{Name: "cortexiq", URL: "http://h:18100/mcp", Token: "abc"}

	claude, err := AddArgs(AppClaude, target, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"mcp", "add", "--transport", "http", "cortexiq", "http://h:18100/mcp", "--header", "Authorization: Bearer abc", "--scope", "user"}
	if !reflect.DeepEqual(claude, want) {
		t.Errorf("claude global:\n got %v\nwant %v", claude, want)
	}

	claudeLocal, err := AddArgs(AppClaude, target, false)
	if err != nil {
		t.Fatal(err)
	}
	if claudeLocal[len(claudeLocal)-1] != "local" {
		t.Errorf("claude local scope: got %v", claudeLocal)
	}

	oc, err := AddArgs(AppOpenCode, target, true)
	if err != nil {
		t.Fatal(err)
	}
	wantOC := []string{"mcp", "add", "cortexiq", "--url", "http://h:18100/mcp", "--header", "Authorization=Bearer abc", "--global"}
	if !reflect.DeepEqual(oc, wantOC) {
		t.Errorf("opencode:\n got %v\nwant %v", oc, wantOC)
	}
	ocLocal, err := AddArgs(AppOpenCode, target, false)
	if err != nil {
		t.Fatal(err)
	}
	if ocLocal[len(ocLocal)-1] == "--global" {
		t.Errorf("project scope must not pass --global: %v", ocLocal)
	}

	if _, err := AddArgs(AppCursor, target, true); err == nil {
		t.Error("cursor has no CLI and must report that")
	}
	if _, err := AddArgs(AppClaude, Target{Name: "cortexiq"}, true); err == nil {
		t.Error("an incomplete target must not reach the client CLI")
	}
}

func TestRemoveArgs(t *testing.T) {
	got, err := RemoveArgs(AppClaude, "cortexiq", true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"mcp", "remove", "cortexiq", "--scope", "user"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if _, err := RemoveArgs(AppOpenCode, "cortexiq", true); !errors.Is(err, ErrNoRemove) {
		t.Errorf("opencode remove: got %v, want ErrNoRemove", err)
	}
	if _, err := RemoveArgs(AppCursor, "cortexiq", true); err == nil {
		t.Error("cursor remove must report that its file is edited instead")
	}
}

func TestMergeServerCreatesDocument(t *testing.T) {
	out, changed, err := MergeServer(AppCursor, nil, "cortexiq", map[string]any{"url": "http://x/mcp"})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("creating an entry must report a change")
	}
	var doc map[string]map[string]map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if doc["mcpServers"]["cortexiq"]["url"] != "http://x/mcp" {
		t.Errorf("entry not written: %s", out)
	}
	if !strings.HasSuffix(string(out), "\n") {
		t.Error("output should end in a newline")
	}
}

func TestMergeServerKeepsEverythingElse(t *testing.T) {
	doc := []byte(`{
  "theme": "dark",
  "mcpServers": {
    "other": {"url": "http://other/sse", "headers": {"X-K": "v"}}
  }
}`)
	out, changed, err := MergeServer(AppCursor, doc, "cortexiq", map[string]any{"url": "http://x/mcp"})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("adding a second server must report a change")
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	if string(parsed["theme"]) != `"dark"` {
		t.Errorf("unrelated key was dropped or changed: %s", out)
	}
	names, err := ServerNames(AppCursor, out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"cortexiq", "other"}) {
		t.Errorf("server names: %v", names)
	}
	var servers map[string]map[string]any
	_ = json.Unmarshal(parsed["mcpServers"], &servers)
	if servers["other"]["url"] != "http://other/sse" {
		t.Errorf("the other server lost its entry: %s", out)
	}
}

func TestMergeServerIsIdempotent(t *testing.T) {
	entry := map[string]any{"url": "http://x/mcp", "headers": map[string]any{"Authorization": "Bearer t"}}
	first, changed, err := MergeServer(AppCursor, nil, "cortexiq", entry)
	if err != nil || !changed {
		t.Fatalf("first merge: changed=%v err=%v", changed, err)
	}
	second, changed, err := MergeServer(AppCursor, first, "cortexiq", entry)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("writing the same entry twice must not rewrite the file")
	}
	if string(second) != string(first) {
		t.Errorf("document changed on a no-op merge:\n%s\n%s", first, second)
	}
}

func TestMergeAndRemoveRefuseComments(t *testing.T) {
	doc := []byte("{\n  // my note\n  \"mcpServers\": {}\n}")
	if _, _, err := MergeServer(AppCursor, doc, "cortexiq", map[string]any{"url": "u"}); !errors.Is(err, ErrComments) {
		t.Errorf("merge: got %v, want ErrComments", err)
	}
	if _, _, err := RemoveServer(AppCursor, doc, "cortexiq"); !errors.Is(err, ErrComments) {
		t.Errorf("remove: got %v, want ErrComments", err)
	}
}

func TestHasComments(t *testing.T) {
	cases := map[string]bool{
		`{"a": 1}`:                           false,
		`{"a": "http://x/y"}`:                false,
		`{"a": "not // a comment"}`:          false,
		`{"a": "escaped \" // inside"}`:      false,
		"{ // line\n}":                       true,
		"{ /* block */ }":                    true,
		`{"a": "x"} /* trailing */`:          true,
		"":                                   false,
		`{"mcp": {"servers": {}},"b": "/*"}`: false,
		`{"a": 1}
// note`: true,
	}
	for doc, want := range cases {
		if got := HasComments([]byte(doc)); got != want {
			t.Errorf("HasComments(%q) = %v, want %v", doc, got, want)
		}
	}
}

func TestRemoveServer(t *testing.T) {
	doc := []byte(`{"mcpServers":{"cortexiq":{"url":"u"},"other":{"url":"o"}}}`)
	out, removed, err := RemoveServer(AppCursor, doc, "cortexiq")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("existing entry must be reported as removed")
	}
	names, err := ServerNames(AppCursor, out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"other"}) {
		t.Errorf("after removal: %v", names)
	}

	same, removed, err := RemoveServer(AppCursor, out, "cortexiq")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Error("a missing entry must not be reported as removed")
	}
	if string(same) != string(out) {
		t.Error("a no-op removal must leave the document untouched")
	}
}

func TestMergeServerRejectsBrokenJSON(t *testing.T) {
	if _, _, err := MergeServer(AppCursor, []byte("{not json"), "cortexiq", map[string]any{"url": "u"}); err == nil {
		t.Error("broken JSON must not be replaced with a fresh document")
	}
	if _, _, err := MergeServer(AppOpenCode, []byte(`{"mcp": 3}`), "cortexiq", map[string]any{"url": "u"}); err == nil {
		t.Error("a non-object mcpServers value must be an error, not a silent overwrite")
	}
}

func TestEntryShapePerClient(t *testing.T) {
	target := Target{Name: "cortexiq", URL: "http://h/mcp", Token: "t"}

	cursor := target.Entry(AppCursor)
	if cursor["url"] != "http://h/mcp" {
		t.Errorf("cursor entry: %v", cursor)
	}
	if _, ok := cursor["type"]; ok {
		t.Error("cursor infers the transport from url; a type field would be ignored")
	}

	oc := target.Entry(AppOpenCode)
	if oc["type"] != "remote" || oc["oauth"] != false {
		t.Errorf("opencode entry: %v", oc)
	}

	claude := target.Entry(AppClaude)
	if claude["url"] != "http://h/mcp" {
		t.Errorf("claude entry: %v", claude)
	}
}

func TestOpenCodeUsesItsOwnServerKey(t *testing.T) {
	// OpenCode V2 nests servers under mcp.servers, which starts as the "mcp"
	// key of the document.
	out, _, err := MergeServer(AppOpenCode, nil, "cortexiq", map[string]any{"type": "remote", "url": "u"})
	if err != nil {
		t.Fatal(err)
	}
	names, err := ServerNames(AppOpenCode, out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"cortexiq"}) {
		t.Errorf("opencode server names: %v", names)
	}
}
