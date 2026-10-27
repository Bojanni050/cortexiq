package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bojanni050/cortexiq/internal/scope"
)

// A read-only token used to be refused reflect, because the allowlist named the
// read tools it knew about and reflect was not among them. The refusal arrived as
// a 403 on what its caller saw as a read, so the caller had to guess. reflect
// writes nothing, so it belongs on this side of the gate.
func TestReadOnlyTokenMayReflectButNotWrite(t *testing.T) {
	cases := []struct {
		tool     string
		readOnly bool
		want     int
		why      string
	}{
		{"recall", true, http.StatusOK, "a read"},
		{"reflect", true, http.StatusOK, "a read that costs a model call"},
		{"query_facts", true, http.StatusOK, "a read"},
		{"remember", true, http.StatusForbidden, "a write"},
		{"init", true, http.StatusForbidden, "creates the /self scaffold"},
		{"create_goal", true, http.StatusForbidden, "a write"},
		{"remember", false, http.StatusOK, "a write, for a token that may write"},
		{"init", false, http.StatusOK, "creates namespaces, for a token that may write"},
	}

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			reached := false
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })
			handler := readOnlyMCP(next)

			body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tc.tool + `"}}`
			r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
			if tc.readOnly {
				r = r.WithContext(scope.With(context.Background(), scope.Principal{Name: "agent", ReadOnly: true}))
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)

			if w.Code != tc.want {
				t.Fatalf("%s with readOnly=%v = %d, want %d (%s)", tc.tool, tc.readOnly, w.Code, tc.want, tc.why)
			}
			if tc.want == http.StatusForbidden {
				if reached {
					t.Fatalf("%s reached the server, so the body was executed and only the reply was refused", tc.tool)
				}
				var refusal struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &refusal); err != nil {
					t.Fatalf("refusal is not JSON: %v (%q)", err, w.Body.String())
				}
				if !strings.Contains(refusal.Error, "read-only") {
					t.Fatalf("refusal = %q, want it to name the read-only token", refusal.Error)
				}
			}
			if tc.want == http.StatusOK && !reached {
				t.Fatalf("%s did not reach the server", tc.tool)
			}
		})
	}
}

// tools/list and initialize are how a client learns what exists, so a read-only
// token must be able to run them; only tools/call is inspected.
func TestReadOnlyTokenMayListTools(t *testing.T) {
	reached := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })
	handler := readOnlyMCP(next)

	for _, body := range []string{`{"method":"tools/list"}`, `{"method":"initialize"}`, `{"method":"ping"}`} {
		r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		r = r.WithContext(scope.With(context.Background(), scope.Principal{Name: "agent", ReadOnly: true}))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusOK || !reached {
			t.Fatalf("%s = %d reached=%v, want 200 and pass through", body, w.Code, reached)
		}
		reached = false
	}
}
