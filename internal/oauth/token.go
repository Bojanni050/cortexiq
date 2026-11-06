// Modifications copyright (c) 2026 Bojan van der Heide

package oauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// tokenResponse is the RFC 6749 §5.1 body. Cache-Control: no-store is set by
// writeJSON; a token response must never be cached.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope"`
}

// token is the token endpoint: authorization_code and refresh_token grants.
func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeOAuthError(w, http.StatusMethodNotAllowed, "invalid_request", "POST required")
		return
	}
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "The request body is not a valid form.")
		return
	}

	clientID := r.PostForm.Get("client_id")
	clientSecret := r.PostForm.Get("client_secret")
	if id, secret, ok := r.BasicAuth(); ok {
		// RFC 6749 §2.3.1: the values in Basic auth are form-urlencoded before
		// base64, so they have to be decoded again.
		if decoded, err := url.QueryUnescape(id); err == nil {
			id = decoded
		}
		if decoded, err := url.QueryUnescape(secret); err == nil {
			secret = decoded
		}
		if clientID != "" && clientID != id {
			writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "client_id does not match the credentials.")
			return
		}
		clientID, clientSecret = id, secret
	}

	client, err := s.resolveClient(r.Context(), clientID)
	if err != nil {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "Unknown client.")
		return
	}
	if client.Disabled {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "This client has been disabled.")
		return
	}
	if !clientAuthenticated(client, clientSecret) {
		w.Header().Set("WWW-Authenticate", `Basic realm="cortexiq"`)
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "Client authentication failed.")
		return
	}

	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		s.tokenFromCode(w, r, client)
	case "refresh_token":
		s.tokenFromRefresh(w, r, client)
	default:
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "Only authorization_code and refresh_token are supported.")
	}
}

// tokenFromCode redeems an authorization code exactly once.
func (s *Server) tokenFromCode(w http.ResponseWriter, r *http.Request, client Client) {
	now := s.now()
	secret := r.PostForm.Get("code")
	if !strings.HasPrefix(secret, tokenPrefixCode) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Unknown or already used authorization code.")
		return
	}
	verifier := r.PostForm.Get("code_verifier")
	if verifier == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "code_verifier is required (PKCE).")
		return
	}

	code, err := s.store.ConsumeCode(r.Context(), hashSecret(secret), now)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Unknown, expired or already used authorization code.")
		return
	}
	if code.ClientID != client.ID {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "The code was issued to another client.")
		return
	}
	// redirect_uri is required when it was part of the authorization request.
	// Comparing it is what stops a code from being replayed to a different
	// callback of the same client.
	if got := r.PostForm.Get("redirect_uri"); got != "" && got != code.RedirectURI {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri does not match the authorization request.")
		return
	}
	if !verifyPKCE(verifier, code.Challenge, code.ChallengeKind) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed.")
		return
	}
	// Audience binding: a token issued for another resource must not be
	// redeemable here, and vice versa.
	if got := strings.TrimRight(r.PostForm.Get("resource"), "/"); got != "" && got != code.Resource {
		writeOAuthError(w, http.StatusBadRequest, "invalid_target", "The requested resource does not match the authorization request.")
		return
	}

	accessSecret, refreshSecret, access, refresh, err := s.newPair(client, code.Scope, code.Resource, code.Principal, "", nil)
	if err != nil {
		s.log.Error("oauth: cannot generate tokens", "err", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "Could not issue tokens.")
		return
	}
	if err := s.store.CreateToken(r.Context(), access); err != nil {
		s.log.Error("oauth: cannot store access token", "err", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "Could not store tokens.")
		return
	}
	if err := s.store.CreateToken(r.Context(), refresh); err != nil {
		s.log.Error("oauth: cannot store refresh token", "err", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "Could not store tokens.")
		return
	}
	s.store.TouchClient(r.Context(), client.ID)
	s.log.Info("oauth: tokens issued", "client", client.ID, "scope", code.Scope, "grant", "authorization_code")

	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  accessSecret,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.cfg.AccessTTL.Seconds()),
		RefreshToken: refreshSecret,
		Scope:        code.Scope,
	})
}

// tokenFromRefresh rotates a refresh token. Reuse of an already rotated token
// revokes the whole family (see Store.RotateRefresh).
func (s *Server) tokenFromRefresh(w http.ResponseWriter, r *http.Request, client Client) {
	now := s.now()
	secret := r.PostForm.Get("refresh_token")
	if !strings.HasPrefix(secret, tokenPrefixRefresh) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Unknown refresh token.")
		return
	}
	old, err := s.store.LookupRefresh(r.Context(), hashSecret(secret), now)
	if errors.Is(err, ErrInvalidGrant) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Unknown or expired refresh token.")
		return
	}
	if err != nil {
		s.log.Error("oauth: refresh lookup failed", "err", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "Could not read the refresh token.")
		return
	}
	if old.ClientID != client.ID {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "The refresh token belongs to another client.")
		return
	}
	resource := old.Resource
	if got := strings.TrimRight(r.PostForm.Get("resource"), "/"); got != "" {
		if got != old.Resource {
			writeOAuthError(w, http.StatusBadRequest, "invalid_target", "The refresh token was issued for another resource.")
			return
		}
		resource = got
	}

	accessSecret, refreshSecret, access, refresh, err := s.newPair(client, old.Scope, resource, old.Principal, old.FamilyID, old.Hash)
	if err != nil {
		s.log.Error("oauth: cannot generate tokens", "err", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "Could not issue tokens.")
		return
	}
	err = s.store.RotateRefresh(r.Context(), old.Hash, access, refresh, now)
	if errors.Is(err, ErrRefreshReused) {
		s.log.Warn("oauth: refresh token reuse detected; family revoked", "client", client.ID, "family", old.FamilyID)
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "The refresh token was already used. Re-authorize the client.")
		return
	}
	if errors.Is(err, ErrInvalidGrant) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Unknown or expired refresh token.")
		return
	}
	if err != nil {
		s.log.Error("oauth: refresh rotation failed", "err", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "Could not rotate the refresh token.")
		return
	}
	s.log.Info("oauth: tokens rotated", "client", client.ID, "scope", old.Scope)

	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  accessSecret,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.cfg.AccessTTL.Seconds()),
		RefreshToken: refreshSecret,
		Scope:        old.Scope,
	})
}

// revoke implements RFC 7009. It answers 200 even for an unknown token, because
// a different answer would tell an attacker whether a token exists.
func (s *Server) revoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeOAuthError(w, http.StatusMethodNotAllowed, "invalid_request", "POST required")
		return
	}
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "The request body is not a valid form.")
		return
	}
	if clientID := r.PostForm.Get("client_id"); clientID != "" {
		client, err := s.resolveClient(r.Context(), clientID)
		if err != nil {
			writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "Unknown client.")
			return
		}
		if !clientAuthenticated(client, r.PostForm.Get("client_secret")) {
			writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "Client authentication failed.")
			return
		}
	}
	secret := r.PostForm.Get("token")
	if secret != "" {
		if err := s.store.Revoke(r.Context(), hashSecret(secret), s.now()); err != nil {
			s.log.Warn("oauth: revoke failed", "err", err)
		}
	}
	w.WriteHeader(http.StatusOK)
}

// newPair mints one access token and one refresh token. family is empty for a
// fresh authorization and the existing family on rotation.
func (s *Server) newPair(client Client, scope, resource, principal, family string, parent []byte) (string, string, Token, Token, error) {
	now := s.now()
	if family == "" {
		id, err := newID("cxq_f_")
		if err != nil {
			return "", "", Token{}, Token{}, err
		}
		family = id
	}
	accessSecret, err := randomSecret(tokenPrefixAccess)
	if err != nil {
		return "", "", Token{}, Token{}, err
	}
	refreshSecret, err := randomSecret(tokenPrefixRefresh)
	if err != nil {
		return "", "", Token{}, Token{}, err
	}
	access := Token{
		Hash:      hashSecret(accessSecret),
		Kind:      "access",
		ClientID:  client.ID,
		Scope:     scope,
		Resource:  resource,
		Principal: principal,
		FamilyID:  family,
		ExpiresAt: now.Add(s.cfg.AccessTTL),
	}
	refresh := Token{
		Hash:       hashSecret(refreshSecret),
		Kind:       "refresh",
		ClientID:   client.ID,
		Scope:      scope,
		Resource:   resource,
		Principal:  principal,
		FamilyID:   family,
		ParentHash: parent,
		ExpiresAt:  now.Add(s.cfg.RefreshTTL),
	}
	return accessSecret, refreshSecret, access, refresh, nil
}

// clientAuthenticated checks the client secret when there is one. Public clients
// have none: PKCE is their proof.
func clientAuthenticated(client Client, secret string) bool {
	if len(client.SecretHash) == 0 {
		return true
	}
	if secret == "" {
		return false
	}
	return hmac.Equal(hashSecret(secret), client.SecretHash)
}

// verifyPKCE implements RFC 7636 S256 only. "plain" is refused even if it were
// somehow stored, because it offers no protection at all.
func verifyPKCE(verifier, challenge, method string) bool {
	if method != "S256" {
		return false
	}
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return hmac.Equal([]byte(computed), []byte(challenge))
}

// refreshTTL is exposed for the CLI output.
func (s *Server) refreshTTL() time.Duration { return s.cfg.RefreshTTL }
