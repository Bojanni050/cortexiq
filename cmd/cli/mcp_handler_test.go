package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/Bojanni050/cortexiq/internal/auth"
)

const testTokenSet = `[
	{"name": "rw", "token": "rw-secret", "scope": "rw"},
	{"name": "ro", "token": "ro-secret", "scope": "ro"}
]`

// testMCPHandler wires the same stack mcpServeCmd serves: auth, the read-only
// gate, and both transports.
func testMCPHandler(t *testing.T) http.Handler {
	t.Helper()
	mcpServer := server.NewMCPServer("cortexiq-test", "0.0.0", server.WithToolCapabilities(false))
	set, err := auth.ParseSet(testTokenSet, "")
	if err != nil {
		t.Fatalf("ParseSet: %v", err)
	}
	sseServer := server.NewSSEServer(mcpServer)
	streamableServer := server.NewStreamableHTTPServer(mcpServer)
	return auth.Require(set, readOnlyMCP(mcpHandler(sseServer, streamableServer)))
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
	// A mutating tool is rejected before it reaches the server.
	if rec := postJSON(t, h, "/mcp", "ro-secret", toolCallBody("remember")); rec.Code != http.StatusForbidden {
		t.Fatalf("remember with ro token: status = %d, want 403", rec.Code)
	}
	// A read-only tool passes the gate and fails later only because the test
	// server registers no tools.
	if rec := postJSON(t, h, "/mcp", "ro-secret", toolCallBody("recall")); rec.Code == http.StatusForbidden {
		t.Fatalf("recall with ro token was blocked by the read-only gate")
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
