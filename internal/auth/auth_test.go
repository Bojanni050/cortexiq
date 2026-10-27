package auth

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/scope"
)

func TestRequireToken(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })

	tests := []struct {
		name   string
		token  string
		header string
		want   int
	}{
		{"auth disabled", "", "", http.StatusNoContent},
		{"missing header", "s3cret", "", http.StatusUnauthorized},
		{"wrong token", "s3cret", "Bearer nope", http.StatusUnauthorized},
		{"wrong scheme", "s3cret", "Basic s3cret", http.StatusUnauthorized},
		{"empty bearer", "s3cret", "Bearer ", http.StatusUnauthorized},
		{"valid", "s3cret", "Bearer s3cret", http.StatusNoContent},
		{"scheme is case-insensitive", "s3cret", "bearer s3cret", http.StatusNoContent},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/facts", nil)
		if tt.header != "" {
			req.Header.Set("Authorization", tt.header)
		}
		rec := httptest.NewRecorder()
		RequireToken(tt.token, ok).ServeHTTP(rec, req)
		if rec.Code != tt.want {
			t.Errorf("%s: got %d, want %d", tt.name, rec.Code, tt.want)
		}
	}
}

func TestParseSetRejectsBadNamespace(t *testing.T) {
	bad := []string{
		`projects/x`,         // no leading slash
		`/`,                  // the root is what a caller already gets
		`/Projects/X`,        // segments are lowercase slugs
		`/projects//x`,       // empty segment
		`/projects/x y`,      // whitespace
		`/projects/../other`, // traversal
	}
	for _, ns := range bad {
		spec := `[{"name":"a","token":"t","namespace":` + strconv.Quote(ns) + `}]`
		if _, err := ParseSet(spec, ""); err == nil {
			t.Errorf("namespace %q must be rejected at start-up", ns)
		}
	}
	if _, err := ParseSet(`[{"name":"a","token":"t","namespace":"/projects/cortexiq"}]`, ""); err != nil {
		t.Fatalf("a valid namespace must parse: %v", err)
	}
}

// The token's namespace must reach the principal: that is the only path from
// configuration to where a write actually lands.
func TestRequireCarriesNamespace(t *testing.T) {
	set, err := ParseSet(`[{"name":"opencode","token":"t1","namespace":"/projects/cortexiq"}]`, "")
	if err != nil {
		t.Fatalf("ParseSet: %v", err)
	}
	var got scope.Principal
	handler := Require(set, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = scope.From(r.Context())
	}))
	req := httptest.NewRequest(http.MethodPost, "/sse", nil)
	req.Header.Set("Authorization", "Bearer t1")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if got.Namespace != "/projects/cortexiq" {
		t.Fatalf("principal namespace = %q, want /projects/cortexiq", got.Namespace)
	}
}

func TestParseSet(t *testing.T) {
	set, err := ParseSet(`[{"name":"ro","token":"t1","scope":"ro"},{"name":"rw","token":"t2"}]`, "")
	if err != nil {
		t.Fatalf("ParseSet: %v", err)
	}
	if !set.Enabled() {
		t.Fatal("expected set to be enabled")
	}
	tok, ok := set.authenticate("t2")
	if !ok || tok.Name != "rw" || tok.Scope != "" {
		t.Fatalf("rw token lookup: ok=%v tok=%+v", ok, tok)
	}
	tok, ok = set.authenticate("t1")
	if !ok || tok.Scope != "ro" {
		t.Fatalf("ro token lookup: ok=%v tok=%+v", ok, tok)
	}
	if _, ok := set.authenticate("nope"); ok {
		t.Fatal("unknown secret must not authenticate")
	}
	if names := set.LegacyNamespaces(); len(names) != 0 {
		t.Fatalf("LegacyNamespaces() = %v, want none", names)
	}
}

// The namespace allowlist is gone. A config that still carries it must keep
// working, so that nobody is locked out by an old .env, but the field is
// reported instead of silently ignored.
func TestParseSetReportsLegacyNamespaces(t *testing.T) {
	set, err := ParseSet(`[{"name":"agent","token":"t1","scope":"ro","namespaces":["/projects"]},{"token":"t2"}]`, "")
	if err != nil {
		t.Fatalf("ParseSet: %v", err)
	}
	names := set.LegacyNamespaces()
	if len(names) != 1 || names[0] != "agent" {
		t.Fatalf("LegacyNamespaces() = %v, want [agent]", names)
	}
}

func TestParseSetFallbackAndErrors(t *testing.T) {
	set, err := ParseSet("", "legacy")
	if err != nil || !set.Enabled() {
		t.Fatalf("fallback token: err=%v enabled=%v", err, set.Enabled())
	}
	if _, err := ParseSet("not json", ""); err == nil {
		t.Fatal("expected a JSON error")
	}
	if _, err := ParseSet(`[{"name":"x","token":""}]`, ""); err == nil {
		t.Fatal("expected an empty-secret error")
	}
	if _, err := ParseSet(`[{"name":"x","token":"y","scope":"admin"}]`, ""); err == nil {
		t.Fatal("expected a scope error")
	}
	if set, _ := ParseSet("", ""); set.Enabled() {
		t.Fatal("no tokens must disable auth")
	}
}

// The single token is added to the list, not replaced by it: setting both must
// never lock a listed caller out.
func TestParseSetFallbackAddsToList(t *testing.T) {
	set, err := ParseSet(`[{"name":"agent","token":"t1","scope":"ro"}]`, "master")
	if err != nil {
		t.Fatalf("ParseSet: %v", err)
	}
	if len(set.tokens) != 2 {
		t.Fatalf("got %d tokens, want 2", len(set.tokens))
	}
	tok, ok := set.authenticate("master")
	if !ok || tok.Name != "default" || tok.Scope != "" {
		t.Fatalf("fallback token lookup: ok=%v tok=%+v", ok, tok)
	}
	if tok, _ := set.authenticate("t1"); tok.Scope != "ro" {
		t.Fatalf("listed token lost its scope: %+v", tok)
	}

	// The same secret must not be listed twice: the last match wins, so a
	// duplicate full-access entry would undo the scope of the listed one.
	same, err := ParseSet(`[{"name":"agent","token":"master","scope":"ro"}]`, "master")
	if err != nil {
		t.Fatalf("ParseSet: %v", err)
	}
	if len(same.tokens) != 1 {
		t.Fatalf("got %d tokens, want 1", len(same.tokens))
	}
	if tok, _ := same.authenticate("master"); tok.Name != "agent" || tok.Scope != "ro" {
		t.Fatalf("duplicate secret widened the scope: %+v", tok)
	}
}

func TestRequireInjectsPrincipal(t *testing.T) {
	set, err := ParseSet(`[{"name":"reader","token":"ro-token","scope":"ro"}]`, "")
	if err != nil {
		t.Fatalf("ParseSet: %v", err)
	}

	var got scope.Principal
	var ok bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok = scope.From(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/facts", nil)
	req.Header.Set("Authorization", "Bearer ro-token")
	rec := httptest.NewRecorder()
	Require(set, inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	if !ok || got.Name != "reader" || !got.ReadOnly {
		t.Fatalf("principal not injected: ok=%v principal=%+v", ok, got)
	}
}
