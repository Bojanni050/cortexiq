// Modifications copyright (c) 2026 Bojan van der Heide

// Package oauth is CortexIQ's own OAuth 2.1 authorization server for the MCP
// endpoint. CortexIQ is a resource server first (it answers /mcp), but hosted MCP
// clients cannot send a static header, so it also issues the tokens it accepts.
//
// The implementation follows the subset the MCP authorization specification
// requires: RFC 9728 protected-resource metadata, RFC 8414 authorization-server
// metadata, authorization code + PKCE (S256), refresh-token rotation, RFC 8707
// resource indicators and RFC 7591 dynamic client registration, plus Client ID
// Metadata Documents for clients that prefer a URL over registration.
//
// Tokens are opaque and stored as SHA-256 digests. There is no JWT and no signing
// key to rotate: the resource server shares a database with the authorization
// server, so a lookup is cheaper than validating a signature and revocation is
// immediate.
package oauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Bojanni050/cortexiq/internal/auth"
	"github.com/Bojanni050/cortexiq/internal/scope"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Scopes. read is always granted; write is what the consent page asks about, so
// a third-party client (Le Chat, a hosted connector) can be connected read-only
// without inventing a second token type.
const (
	ScopeRead  = "memory:read"
	ScopeWrite = "memory:write"
)

// SupportedScopes is what the metadata advertises and what the consent page
// offers. Order matters: it is the order the page shows.
var SupportedScopes = []string{ScopeRead, ScopeWrite}

// Errors shared by the store and the handlers.
var (
	ErrUnknownClient = errors.New("unknown oauth client")
	ErrInvalidGrant  = errors.New("invalid_grant")
	ErrInvalidToken  = errors.New("invalid_token")
	ErrRefreshReused = errors.New("refresh token reused")
)

// Config is the deployment-specific half of the server.
type Config struct {
	// Issuer is the public origin, e.g. https://cortexiq.example.com. It has no
	// trailing slash and is the value clients discover.
	Issuer string
	// Resource is the canonical MCP endpoint URI the tokens are bound to
	// (RFC 8707). Empty defaults to Issuer + "/mcp".
	Resource string
	// AdminPassword gates the consent page. Empty disables the whole OAuth
	// server, leaving only the static API tokens.
	AdminPassword string
	// SessionSecret signs the consent-session cookie. Empty derives one from
	// AdminPassword, which is stable across restarts and needs no extra config.
	SessionSecret string

	AccessTTL  time.Duration
	RefreshTTL time.Duration
	CodeTTL    time.Duration

	Logger *slog.Logger
}

// Server holds the authorization server. One per process.
type Server struct {
	cfg      Config
	store    *Store
	log      *slog.Logger
	sessions *sessionSigner
	limiter  *limiter
	cimd     *cimdCache
	now      func() time.Time
}

// NewServer builds the server. It never fails: a half-configured deployment
// reports itself disabled through Enabled, so the MCP server keeps running on
// static tokens instead of refusing to start.
func NewServer(cfg Config, pool *pgxpool.Pool) *Server {
	cfg.Issuer = strings.TrimRight(strings.TrimSpace(cfg.Issuer), "/")
	cfg.Resource = strings.TrimRight(strings.TrimSpace(cfg.Resource), "/")
	if cfg.Resource == "" && cfg.Issuer != "" {
		cfg.Resource = cfg.Issuer + "/mcp"
	}
	if cfg.AccessTTL <= 0 {
		cfg.AccessTTL = time.Hour
	}
	if cfg.RefreshTTL <= 0 {
		cfg.RefreshTTL = 30 * 24 * time.Hour
	}
	if cfg.CodeTTL <= 0 {
		cfg.CodeTTL = 60 * time.Second
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		cfg:      cfg,
		store:    NewStore(pool),
		log:      logger,
		sessions: newSessionSigner(cfg.SessionSecret, cfg.AdminPassword),
		limiter:  newLimiter(),
		cimd:     newCIMDCache(),
		now:      time.Now,
	}
}

// Enabled reports whether the OAuth endpoints should exist at all.
func (s *Server) Enabled() bool {
	return s != nil && s.cfg.Issuer != "" && s.cfg.AdminPassword != ""
}

// Store exposes the persistence layer to the admin CLI.
func (s *Server) Store() *Store { return s.store }

// Resource returns the audience tokens are bound to.
func (s *Server) Resource() string { return s.cfg.Resource }

// Issuer returns the public origin.
func (s *Server) Issuer() string { return s.cfg.Issuer }

// MetadataURL is the protected-resource metadata address, used in the
// WWW-Authenticate challenge.
func (s *Server) MetadataURL() string {
	return s.cfg.Issuer + "/.well-known/oauth-protected-resource/mcp"
}

// Challenge is the 401 WWW-Authenticate value. Without it a hosted client never
// learns where the authorization server is: the spec allows well-known probing,
// but the header is the path every client takes.
func (s *Server) Challenge() string {
	if !s.Enabled() {
		return `Bearer realm="cortexiq"`
	}
	return fmt.Sprintf(`Bearer resource_metadata=%q, scope=%q`,
		s.MetadataURL(), strings.Join(SupportedScopes, " "))
}

// InsufficientScopeChallenge is the 403 value for a read-only token that tried a
// write. The scope list is what the client must ask for next time.
func (s *Server) InsufficientScopeChallenge() string {
	if !s.Enabled() {
		return `Bearer realm="cortexiq", error="insufficient_scope"`
	}
	return fmt.Sprintf(`Bearer error="insufficient_scope", scope=%q, resource_metadata=%q`,
		ScopeWrite, s.MetadataURL())
}

// RequireMCP authenticates the MCP transports only. Everything else on the same
// listener (metadata, /oauth/*) is public by design: those endpoints are how a
// client gets a token in the first place.
//
// The path check is a switch, not a prefix match, so a crafted path cannot be
// public here and something else after ServeMux cleans it.
func (s *Server) RequireMCP(static *auth.Set, next http.Handler) http.Handler {
	staticOn := static.Enabled()
	oauthOn := s.Enabled()
	if !staticOn && !oauthOn {
		return next
	}
	// With OAuth off, a challenge that points at metadata would be a dead end.
	challenge := `Bearer realm="cortexiq"`
	if oauthOn {
		challenge = s.Challenge()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mcp", "/sse", "/message":
		default:
			next.ServeHTTP(w, r)
			return
		}

		secret, ok := auth.BearerToken(r)
		if ok {
			if staticOn {
				if tok, found := static.Authenticate(secret); found {
					ctx := scope.With(r.Context(), scope.Principal{
						Name:     tok.Name,
						ReadOnly: tok.Scope == "ro",
					})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			if oauthOn {
				if p, found := s.principalFor(r.Context(), secret); found {
					next.ServeHTTP(w, r.WithContext(scope.With(r.Context(), p)))
					return
				}
			}
		}
		auth.Unauthorized(w, challenge)
	})
}

// principalFor turns an opaque access token into the same principal the static
// tokens produce, so the brain needs no notion of OAuth.
func (s *Server) principalFor(ctx context.Context, secret string) (scope.Principal, bool) {
	if !strings.HasPrefix(secret, tokenPrefixAccess) {
		return scope.Principal{}, false
	}
	t, err := s.store.LookupAccess(ctx, hashSecret(secret), s.now())
	if err != nil {
		return scope.Principal{}, false
	}
	return scope.Principal{
		Name:     "oauth:" + t.Principal,
		ReadOnly: !hasScope(t.Scope, ScopeWrite),
	}, true
}

// StartCleanup removes expired codes and tokens on a timer. The context ends
// when the server shuts down.
func (s *Server) StartCleanup(ctx context.Context) {
	if !s.Enabled() {
		return
	}
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				codes, tokens, err := s.store.DeleteExpired(ctx, s.now().Add(-7*24*time.Hour))
				if err != nil {
					s.log.Warn("oauth cleanup failed", "err", err)
					continue
				}
				if codes > 0 || tokens > 0 {
					s.log.Info("oauth cleanup", "codes", codes, "tokens", tokens)
				}
			}
		}
	}()
}

// Register mounts the public endpoints on the MCP listener's mux.
func (s *Server) Register(mux *http.ServeMux) {
	if !s.Enabled() {
		return
	}
	// RFC 9728 allows the metadata at the root and at the resource path. Clients
	// try the path form first, so both must answer.
	mux.HandleFunc("/.well-known/oauth-protected-resource", corsPublic(s.protectedResourceMetadata))
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", corsPublic(s.protectedResourceMetadata))
	mux.HandleFunc("/.well-known/oauth-authorization-server", corsPublic(s.authorizationServerMetadata))

	// Registration is public and carries no secret, so a browser-based client may
	// read it cross-origin. The token endpoint deliberately does not: its response
	// holds the tokens.
	mux.HandleFunc("/oauth/register", corsPublic(s.registerClient))
	mux.HandleFunc("/oauth/authorize", s.authorize)
	mux.HandleFunc("/oauth/token", s.token)
	mux.HandleFunc("/oauth/revoke", s.revoke)
}

// corsPublic allows cross-origin reads of endpoints that carry no credentials.
func corsPublic(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

// protectedResourceMetadata is the document a client reads after a 401.
func (s *Server) protectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 s.cfg.Resource,
		"authorization_servers":    []string{s.cfg.Issuer},
		"scopes_supported":         SupportedScopes,
		"bearer_methods_supported": []string{"header"},
	})
}

// authorizationServerMetadata is RFC 8414. code_challenge_methods_supported must
// be present or an MCP client is required to refuse the flow.
func (s *Server) authorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                s.cfg.Issuer,
		"authorization_endpoint":                s.cfg.Issuer + "/oauth/authorize",
		"token_endpoint":                        s.cfg.Issuer + "/oauth/token",
		"registration_endpoint":                 s.cfg.Issuer + "/oauth/register",
		"revocation_endpoint":                   s.cfg.Issuer + "/oauth/revoke",
		"scopes_supported":                      SupportedScopes,
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none", "client_secret_post"},
		"client_id_metadata_document_supported": true,
	})
}

// Token prefixes. Recognising the kind before a database round trip is cheap and
// keeps a stray static token from being looked up in the wrong table.
const (
	tokenPrefixAccess  = "cxq_at_"
	tokenPrefixRefresh = "cxq_rt_"
	tokenPrefixCode    = "cxq_ac_"
	tokenPrefixClient  = "cxq_c_"
)

// hasScope reports whether a space-separated scope string contains want.
func hasScope(granted, want string) bool {
	for _, s := range strings.Fields(granted) {
		if s == want {
			return true
		}
	}
	return false
}

// normalizeScope validates a requested scope string and returns it in canonical
// order. An empty request means "everything the resource advertises": MCP clients
// that find no scope hint request all supported scopes, and refusing them would
// break the flow for no security gain (the consent page still asks).
func normalizeScope(raw string) (string, error) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return strings.Join(SupportedScopes, " "), nil
	}
	seen := map[string]bool{}
	for _, s := range fields {
		if !knownScope(s) {
			return "", fmt.Errorf("unknown scope %q", s)
		}
		seen[s] = true
	}
	// memory:write implies read; a client that asks for write alone still needs
	// to read what it writes. Emit in the advertised order so the consent page
	// and the token response agree no matter how the client ordered its request.
	var out []string
	for _, s := range SupportedScopes {
		if seen[s] || (s == ScopeRead && seen[ScopeWrite]) {
			out = append(out, s)
		}
	}
	return strings.Join(out, " "), nil
}

func knownScope(s string) bool {
	for _, known := range SupportedScopes {
		if s == known {
			return true
		}
	}
	return false
}

// isLoopback reports whether the request came from the machine itself, which is
// how the reverse proxy reaches us.
func isLoopback(remoteAddr string) bool {
	host := remoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

// clientIP is the address used for rate limiting. X-Forwarded-For is only
// trusted when the direct peer is the local reverse proxy: otherwise any caller
// could pick its own bucket.
func clientIP(r *http.Request) string {
	if isLoopback(r.RemoteAddr) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i > 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}

// writeJSON writes a JSON body. Cache-Control: no-store is required on token
// responses and harmless on metadata.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// oauthError is the RFC 6749 error body.
type oauthError struct {
	Code        string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

func writeOAuthError(w http.ResponseWriter, status int, code, description string) {
	writeJSON(w, status, oauthError{Code: code, Description: description})
}

// sessionSigner produces and checks the consent-page cookie. It is deliberately
// not a JWT: the only claims are "this browser logged in" and "until when".
type sessionSigner struct {
	key []byte
}

const (
	sessionCookie = "cxq_oauth_session"
	csrfCookie    = "cxq_oauth_csrf"
	sessionTTL    = 8 * time.Hour
)

func newSessionSigner(secret, password string) *sessionSigner {
	material := secret
	if material == "" {
		material = password
	}
	sum := sha256.Sum256([]byte("cortexiq oauth session\x00" + material))
	return &sessionSigner{key: sum[:]}
}

func (s *sessionSigner) sign(payload string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// issue returns a cookie value valid until expiry.
func (s *sessionSigner) issue(expiry time.Time) string {
	payload := fmt.Sprintf("%d", expiry.Unix())
	return payload + "." + s.sign(payload)
}

// valid checks the signature and the expiry.
func (s *sessionSigner) valid(value string, now time.Time) bool {
	payload, sig, ok := strings.Cut(value, ".")
	if !ok {
		return false
	}
	if !hmac.Equal([]byte(sig), []byte(s.sign(payload))) {
		return false
	}
	var unix int64
	if _, err := fmt.Sscanf(payload, "%d", &unix); err != nil {
		return false
	}
	return time.Unix(unix, 0).After(now)
}
