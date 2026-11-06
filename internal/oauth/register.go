// Modifications copyright (c) 2026 Bojan van der Heide

package oauth

import (
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// Dynamic client registration (RFC 7591). An MCP client that has no client id
// calls this endpoint, gets one, and immediately starts the authorization flow.
// There is no registration access token: a client that wants to change its
// registration registers again, and the old row is cleaned up by disabling it in
// the admin CLI. That is a deliberate simplification — MCP clients register once
// and never update.

const registerMaxBytes = 32 << 10

type registerRequest struct {
	RedirectURIs            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	Scope                   string   `json:"scope"`
}

func (s *Server) registerClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeOAuthError(w, http.StatusMethodNotAllowed, "invalid_request", "POST required")
		return
	}
	ip := clientIP(r)
	if !s.limiter.allow("register:"+ip, 10, time.Hour, s.now()) {
		writeOAuthError(w, http.StatusTooManyRequests, "invalid_request", "Too many registrations from this address.")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, registerMaxBytes+1))
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "Could not read the request body.")
		return
	}
	if len(body) > registerMaxBytes {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "The request body is too large.")
		return
	}
	var req registerRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "The body is not valid JSON.")
		return
	}
	if len(req.RedirectURIs) == 0 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "At least one redirect_uri is required.")
		return
	}
	for _, uri := range req.RedirectURIs {
		if err := validateRedirectURI(uri); err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", err.Error())
			return
		}
	}
	if req.Scope != "" {
		if _, err := normalizeScope(req.Scope); err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", err.Error())
			return
		}
	}

	// Public clients are the norm for MCP. A confidential client is accepted so
	// an integrator that insists on a secret is not blocked, but the secret is
	// generated here and returned exactly once.
	method := req.TokenEndpointAuthMethod
	if method == "" {
		method = "none"
	}
	if method == "client_secret_basic" {
		method = "client_secret_post"
	}
	if method != "none" && method != "client_secret_post" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata",
			"token_endpoint_auth_method must be none or client_secret_post.")
		return
	}

	clientID, err := randomSecret(tokenPrefixClient)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "Could not generate a client id.")
		return
	}
	client := Client{
		ID:           clientID,
		Name:         cleanName(req.ClientName),
		RedirectURIs: req.RedirectURIs,
		AuthMethod:   method,
		Source:       "dcr",
	}
	var clientSecret string
	if method != "none" {
		clientSecret, err = randomSecret("cxq_s_")
		if err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "Could not generate a client secret.")
			return
		}
		sum := sha256.Sum256([]byte(clientSecret))
		client.SecretHash = sum[:]
	}
	if err := s.store.CreateClient(r.Context(), client); err != nil {
		s.log.Error("oauth: cannot store registered client", "err", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "Could not store the client.")
		return
	}
	s.log.Info("oauth: client registered", "client", client.ID, "name", client.Name, "ip", ip)

	resp := map[string]any{
		"client_id":                  client.ID,
		"client_id_issued_at":        s.now().Unix(),
		"client_name":                client.Name,
		"redirect_uris":              client.RedirectURIs,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": method,
		"scope":                      strings.Join(SupportedScopes, " "),
	}
	if clientSecret != "" {
		resp["client_secret"] = clientSecret
		resp["client_secret_expires_at"] = 0
	}
	writeJSON(w, http.StatusCreated, resp)
}

// cleanName trims a client name and caps its length. The consent page shows it,
// so an unbounded or control-character-laden name is not acceptable.
func cleanName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if len(name) > clientNameLimit {
		name = name[:clientNameLimit]
	}
	if name == "" {
		return "MCP client"
	}
	return name
}
