// Package auth provides the shared Bearer-token check used by the dashboard API
// and the MCP server.
package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// RequireToken wraps next so every request must carry "Authorization: Bearer <token>".
// An empty token disables authentication (the historical behaviour).
func RequireToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	want := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := bearerToken(r)
		// Hash both sides so the comparison is constant-time regardless of length.
		have := sha256.Sum256([]byte(got))
		if !ok || subtle.ConstantTimeCompare(have[:], want[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="stash"`)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized: missing or invalid API token"}` + "\n"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(h[len(prefix):]), true
}
