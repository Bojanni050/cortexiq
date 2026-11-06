// Modifications copyright (c) 2026 Bojan van der Heide
package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Bojanni050/cortexiq/internal/auth"
	"github.com/Bojanni050/cortexiq/internal/scope"
)

// testServer builds a server without a database: metadata, PKCE and the
// unauthenticated path all work on configuration alone.
func testServer() *Server {
	return NewServer(Config{
		Issuer:        "https://cortexiq.example.com",
		Resource:      "https://cortexiq.example.com/mcp",
		AdminPassword: "correct horse battery staple",
		SessionSecret: "test-session-secret",
	}, nil)
}

func pkcePair(verifier string) (string, string) {
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func TestVerifyPKCE(t *testing.T) {
	verifier, challenge := pkcePair("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUV")
	tests := []struct {
		name      string
		verifier  string
		challenge string
		method    string
		want      bool
	}{
		{"valid s256", verifier, challenge, "S256", true},
		{"wrong verifier", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", challenge, "S256", false},
		{"plain refused", verifier, verifier, "plain", false},
		{"empty method refused", verifier, challenge, "", false},
		{"short verifier refused", "too-short", challenge, "S256", false},
		{"long verifier refused", strings.Repeat("a", 129), challenge, "S256", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := verifyPKCE(tc.verifier, tc.challenge, tc.method); got != tc.want {
				t.Fatalf("verifyPKCE() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNormalizeScope(t *testing.T) {
	tests := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{"", "memory:read memory:write", false},
		{"memory:read", "memory:read", false},
		{"memory:write", "memory:read memory:write", false},
		{"memory:write memory:read", "memory:read memory:write", false},
		{"memory:read memory:read", "memory:read", false},
		{"memory:admin", "", true},
		{"memory:read bogus", "", true},
	}
	for _, tc := range tests {
		got, err := normalizeScope(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizeScope(%q) = %q, want error", tc.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeScope(%q): %v", tc.raw, err)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizeScope(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestValidateRedirectURI(t *testing.T) {
	ok := []string{
		"https://example.com/callback",
		"https://example.com:8443/cb?x=1",
		"http://localhost:3000/callback",
		"http://127.0.0.1:8080/cb",
		"http://[::1]:9000/cb",
	}
	for _, raw := range ok {
		if err := validateRedirectURI(raw); err != nil {
			t.Errorf("validateRedirectURI(%q) = %v, want nil", raw, err)
		}
	}
	bad := []string{
		"",
		"http://example.com/callback",
		"ftp://example.com/cb",
		"https://example.com/cb#fragment",
		"https://user:pass@example.com/cb",
		"javascript:alert(1)",
		"https:///nohost",
	}
	for _, raw := range bad {
		if err := validateRedirectURI(raw); err == nil {
			t.Errorf("validateRedirectURI(%q) = nil, want error", raw)
		}
	}
}

func TestRedirectAllowed(t *testing.T) {
	client := Client{RedirectURIs: []string{"https://example.com/cb", "http://localhost:1234/cb"}}
	if !redirectAllowed(client, "https://example.com/cb") {
		t.Fatal("exact match should be allowed")
	}
	if redirectAllowed(client, "https://example.com/cb/") {
		t.Fatal("a near match must not be allowed: exact matching is the point")
	}
	if redirectAllowed(client, "https://evil.example.com/cb") {
		t.Fatal("another host must not be allowed")
	}
}

func TestSessionSigner(t *testing.T) {
	s := newSessionSigner("", "admin-password")
	now := time.Now()
	value := s.issue(now.Add(time.Hour))
	if !s.valid(value, now) {
		t.Fatal("freshly issued session should be valid")
	}
	if s.valid(value, now.Add(2*time.Hour)) {
		t.Fatal("expired session should be invalid")
	}
	if s.valid(value+"x", now) {
		t.Fatal("tampered signature should be invalid")
	}
	if s.valid("", now) {
		t.Fatal("empty value should be invalid")
	}
	other := newSessionSigner("", "another-password")
	if other.valid(value, now) {
		t.Fatal("a cookie signed with another key must not verify")
	}
}

func TestMetadata(t *testing.T) {
	s := testServer()

	rec := httptest.NewRecorder()
	s.protectedResourceMetadata(rec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var prm map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &prm); err != nil {
		t.Fatal(err)
	}
	if prm["resource"] != "https://cortexiq.example.com/mcp" {
		t.Fatalf("resource = %v", prm["resource"])
	}
	servers, _ := prm["authorization_servers"].([]any)
	if len(servers) != 1 || servers[0] != "https://cortexiq.example.com" {
		t.Fatalf("authorization_servers = %v", prm["authorization_servers"])
	}

	rec = httptest.NewRecorder()
	s.authorizationServerMetadata(rec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var as map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &as); err != nil {
		t.Fatal(err)
	}
	if as["issuer"] != "https://cortexiq.example.com" {
		t.Fatalf("issuer = %v", as["issuer"])
	}
	if as["client_id_metadata_document_supported"] != true {
		t.Fatal("CIMD must be advertised")
	}
	methods, _ := as["code_challenge_methods_supported"].([]any)
	if len(methods) != 1 || methods[0] != "S256" {
		t.Fatalf("code_challenge_methods_supported = %v", as["code_challenge_methods_supported"])
	}
}

func TestChallenge(t *testing.T) {
	s := testServer()
	if !strings.Contains(s.Challenge(), `resource_metadata="https://cortexiq.example.com/.well-known/oauth-protected-resource/mcp"`) {
		t.Fatalf("challenge = %q", s.Challenge())
	}
	if !strings.Contains(s.Challenge(), `scope="memory:read memory:write"`) {
		t.Fatalf("challenge = %q", s.Challenge())
	}
	if !strings.Contains(s.InsufficientScopeChallenge(), `error="insufficient_scope"`) {
		t.Fatalf("challenge = %q", s.InsufficientScopeChallenge())
	}
}

// An MCP request without a token must be answered with a challenge that points
// at the metadata, because that is where a hosted client starts the OAuth flow.
func TestRequireMCPUnauthorized(t *testing.T) {
	s := testServer()
	static, err := auth.ParseSet(`[{"name":"app","token":"static-secret","scope":"rw"}]`, "")
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler must not run without a token")
	})
	h := s.RequireMCP(static, next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); !strings.Contains(got, "resource_metadata=") {
		t.Fatalf("WWW-Authenticate = %q", got)
	}
}

// Discovery must stay reachable without a token, or no client can authenticate.
func TestRequireMCPLeavesDiscoveryOpen(t *testing.T) {
	s := testServer()
	static, err := auth.ParseSet(`[{"name":"app","token":"static-secret","scope":"rw"}]`, "")
	if err != nil {
		t.Fatal(err)
	}
	called := false
	h := s.RequireMCP(static, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))
	if !called {
		t.Fatal("discovery must not require a token")
	}
}

// A static token still works exactly as before; the OAuth layer only adds a
// second way in.
func TestRequireMCPStaticToken(t *testing.T) {
	s := testServer()
	static, err := auth.ParseSet(`[{"name":"app","token":"static-secret","scope":"ro"}]`, "")
	if err != nil {
		t.Fatal(err)
	}
	var principal string
	var readOnly bool
	h := s.RequireMCP(static, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := scope.From(r.Context())
		principal, readOnly = p.Name, p.ReadOnly
	}))
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer static-secret")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if principal != "app" || !readOnly {
		t.Fatalf("principal = %q readOnly = %v", principal, readOnly)
	}
}
