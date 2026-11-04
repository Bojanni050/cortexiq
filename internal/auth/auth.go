// Package auth provides the shared Bearer-token check used by the dashboard API
// and the MCP server. A token is either read-write or read-only; the resulting
// principal is put on the request context for the brain to enforce.
package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Bojanni050/cortexiq/internal/scope"
)

// Token is one API token definition. Tokens are configured as a JSON array in
// CORTEXIQ_API_TOKENS.
type Token struct {
	Name   string `json:"name"`
	Secret string `json:"token"`
	Scope  string `json:"scope"` // "rw" (default) or "ro"
	// LegacyNamespaces parses the removed namespace allowlist, which is no
	// longer enforced. It is kept so start-up can point at a config that still
	// carries it instead of dropping the restriction silently. Omitted from
	// JSON when absent, so a freshly generated token line does not advertise a
	// field with no meaning.
	LegacyNamespaces []string `json:"namespaces,omitempty"`
}

// Set is an immutable collection of tokens.
type Set struct {
	tokens []Token
	legacy []string
}

// ParseSet builds the token set from CORTEXIQ_API_TOKENS (a JSON array of Token)
// plus CORTEXIQ_API_TOKEN, if set, as one extra full-access token. The two add
// up, so a dashboard token listed in the array cannot be lost by setting it.
// Both empty disables authentication (the historical behaviour).
func ParseSet(spec, fallback string) (*Set, error) {
	var tokens []Token
	var legacy []string
	if strings.TrimSpace(spec) != "" {
		if err := json.Unmarshal([]byte(spec), &tokens); err != nil {
			return nil, fmt.Errorf("CORTEXIQ_API_TOKENS is not a JSON array of tokens: %w", err)
		}
	}
	// The single token is appended rather than skipped, unless the same secret is
	// already in the list: the last entry with a matching secret wins, so a
	// duplicate would silently widen that token's scope.
	if fallback != "" {
		duplicate := false
		for _, t := range tokens {
			if t.Secret == fallback {
				duplicate = true
				break
			}
		}
		if !duplicate {
			tokens = append(tokens, Token{Name: "default", Secret: fallback})
		}
	}
	for i, t := range tokens {
		if t.Secret == "" {
			return nil, fmt.Errorf("token %d (%q) has an empty secret", i, t.Name)
		}
		switch t.Scope {
		case "", "rw", "ro":
		default:
			return nil, fmt.Errorf("token %d (%q) has invalid scope %q (want rw or ro)", i, t.Name, t.Scope)
		}
		if len(t.LegacyNamespaces) > 0 {
			name := t.Name
			if name == "" {
				name = fmt.Sprintf("token %d", i)
			}
			legacy = append(legacy, name)
		}
	}
	return &Set{tokens: tokens, legacy: legacy}, nil
}

// Enabled reports whether any token is configured.
func (s *Set) Enabled() bool { return s != nil && len(s.tokens) > 0 }

// LegacyNamespaces lists the tokens whose definition still carries the removed
// "namespaces" allowlist. Callers log it, so a config that looks restricted but
// is not gets noticed.
func (s *Set) LegacyNamespaces() []string {
	if s == nil {
		return nil
	}
	return s.legacy
}

// authenticate finds the token whose secret matches. Every token is compared in
// constant time and the loop never exits early, so lookup does not reveal which
// token (if any) matched through timing.
func (s *Set) authenticate(secret string) (Token, bool) {
	have := sha256.Sum256([]byte(secret))
	var found Token
	matched := 0
	for _, t := range s.tokens {
		want := sha256.Sum256([]byte(t.Secret))
		if subtle.ConstantTimeCompare(have[:], want[:]) == 1 {
			found = t
			matched = 1
		}
	}
	return found, matched == 1
}

// Require wraps next so every request must carry a valid "Authorization: Bearer"
// header. An empty set disables authentication. On success the principal is put
// on the request context.
func Require(set *Set, next http.Handler) http.Handler {
	if !set.Enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret, ok := bearerToken(r)
		if !ok {
			unauthorized(w)
			return
		}
		token, ok := set.authenticate(secret)
		if !ok {
			unauthorized(w)
			return
		}
		ctx := scope.With(r.Context(), scope.Principal{
			Name:     token.Name,
			ReadOnly: token.Scope == "ro",
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireToken is the single-token convenience wrapper.
func RequireToken(token string, next http.Handler) http.Handler {
	set, _ := ParseSet("", token)
	return Require(set, next)
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="cortexiq"`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized: missing or invalid API token"}` + "\n"))
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(h[len(prefix):]), true
}
