package auth

import (
	"net/http"
	"net/http/httptest"
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

func TestParseSet(t *testing.T) {
	set, err := ParseSet(`[{"name":"ro","token":"t1","scope":"ro","namespaces":["/projects"]},{"name":"rw","token":"t2"}]`, "")
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
	if !ok || tok.Scope != "ro" || len(tok.Namespaces) != 1 || tok.Namespaces[0] != "/projects" {
		t.Fatalf("ro token lookup: ok=%v tok=%+v", ok, tok)
	}
	if _, ok := set.authenticate("nope"); ok {
		t.Fatal("unknown secret must not authenticate")
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

func TestRequireInjectsPrincipal(t *testing.T) {
	set, err := ParseSet(`[{"name":"reader","token":"ro-token","scope":"ro","namespaces":["/projects"]}]`, "")
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
	if !ok || got.Name != "reader" || !got.ReadOnly || len(got.Namespaces) != 1 {
		t.Fatalf("principal not injected: ok=%v principal=%+v", ok, got)
	}
}
