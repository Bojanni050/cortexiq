package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/Bojanni050/cortexiq/internal/auth"
	"github.com/Bojanni050/cortexiq/internal/oauth"
)

const testTokenSet = `[
	{"name": "rw", "token": "rw-secret", "scope": "rw"},
	{"name": "ro", "token": "ro-secret", "scope": "ro"}
]`

// testMCPHandler wires the same stack mcpServeCmd serves: the OAuth-or-static
// check, the read-only gate, and both transports. The OAuth server has no
// database here: the static-token path never touches it.
func testMCPHandler(t *testing.T) http.Handler {
	t.Helper()
	mcpServer := server.NewMCPServer("cortexiq-test", "0.0.0", server.WithToolCapabilities(false))
	set, err := auth.ParseSet(testTokenSet, "")
	if err != nil {
		t.Fatalf("ParseSet: %v", err)
	}
	oauthSrv := oauth.NewServer(oauth.Config{
		Issuer:        "https://cortexiq.test",
		Resource:      "https://cortexiq.test/mcp",
		AdminPassword: "test-password",
	}, nil)
	sseServer := server.NewSSEServer(mcpServer)
	streamableServer := server.NewStreamableHTTPServer(mcpServer)
	gate := readOnlyMCP(oauthSrv.InsufficientScopeChallenge(), mcpHandler(sseServer, streamableServer, oauthSrv))
	return oauthSrv.RequireMCP(set, gate)
}

func initializeBody() string {
	return `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`
}

func toolCallBody(name string) string {
	return `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"` + name + `","arguments":{}}}`
}

func postJSON(t *testing.T, h http.Handler, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestStreamableHTTPInitialize(t *testing.T) {
	h := testMCPHandler(t)
	rec := postJSON(t, h, "/mcp", "rw-secret", initializeBody())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "cortexiq-test") {
		t.Fatalf("response does not carry the server info: %s", rec.Body.String())
	}
}

func TestStreamableHTTPRequiresToken(t *testing.T) {
	h := testMCPHandler(t)
	rec := postJSON(t, h, "/mcp", "", initializeBody())
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestStreamableHTTPReadOnlyGate(t *testing.T) {
	h := testMCPHandler(t)
	// A mutating tool is rejected before it reaches the server, with the MCP
	// challenge that tells an OAuth client it needs memory:write.
	rec := postJSON(t, h, "/mcp", "ro-secret", toolCallBody("remember"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("remember with ro token: status = %d, want 403", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); !strings.Contains(got, `error="insufficient_scope"`) {
		t.Fatalf("WWW-Authenticate = %q, want insufficient_scope", got)
	}
	// A read-only tool passes the gate and fails later only because the test
	// server registers no tools.
	if rec := postJSON(t, h, "/mcp", "ro-secret", toolCallBody("recall")); rec.Code == http.StatusForbidden {
		t.Fatalf("recall with ro token was blocked by the read-only gate")
	}
}

// A client that gets a 401 must be able to read the discovery documents on the
// same origin without a token; that is the whole entry point of the OAuth flow.
func TestDiscoveryServedWithoutToken(t *testing.T) {
	h := testMCPHandler(t)
	for _, path := range []string{
		"/.well-known/oauth-protected-resource",
		"/.well-known/oauth-protected-resource/mcp",
		"/.well-known/oauth-authorization-server",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", path, rec.Code)
		}
	}
}

// The reasoner cost counters live in this process, so the listener has to
// expose them. They stay behind the same Bearer check as the transports.
func TestMetricsServedOnTheMCPListener(t *testing.T) {
	h := testMCPHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("metrics without a token: status = %d, want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer rw-secret")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics with a token: status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "# HELP") {
		t.Fatalf("metrics body does not look like a Prometheus exposition")
	}
}

func TestSSETransportStillServed(t *testing.T) {
	srv := httptest.NewServer(testMCPHandler(t))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/sse", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer rw-secret")
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /sse: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	buf := make([]byte, 256)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "event: endpoint") {
		t.Fatalf("first SSE event = %q, want the endpoint event", string(buf[:n]))
	}
}
