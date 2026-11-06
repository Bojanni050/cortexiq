// Modifications copyright (c) 2026 Bojan van der Heide

package oauth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// authorizeRequest is a validated /oauth/authorize request: everything the
// consent page and the code need, with nothing left to re-parse.
type authorizeRequest struct {
	Client      Client
	RedirectURI string
	State       string
	Challenge   string
	Method      string
	Resource    string
	Scope       string
}

// authorizeFailure distinguishes "tell the client" from "show the page". An
// error may only be redirected once both the client and the redirect URI are
// known to be valid; otherwise the redirect would be an open redirect.
type authorizeFailure struct {
	Code        string
	Description string
	RedirectTo  string
	State       string
}

func (f *authorizeFailure) Error() string { return f.Code + ": " + f.Description }

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		s.authorizePage(w, r, nil)
	case http.MethodPost:
		s.authorizeDecision(w, r)
	default:
		w.Header().Set("Allow", "GET, HEAD, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// authorizePage renders the consent form for a valid request.
func (s *Server) authorizePage(w http.ResponseWriter, r *http.Request, failure *authorizeFailure) {
	req, err := s.parseAuthorize(r.Context(), r.URL.Query())
	if err != nil {
		if failure == nil {
			failure = err
		}
		if failure.RedirectTo != "" {
			redirectError(w, r, failure)
			return
		}
		renderError(w, http.StatusBadRequest, errorData{
			Title:   "This authorization request is not valid",
			Message: failure.Description,
			Detail:  failure.Code,
		})
		return
	}

	csrf := s.csrfToken(w, r)
	writeScope := hasScope(req.Scope, ScopeWrite)
	renderConsent(w, http.StatusOK, consentData{
		ClientName:     displayName(req.Client),
		ClientHost:     redirectHost(req.RedirectURI),
		ClientID:       req.Client.ID,
		RedirectURI:    req.RedirectURI,
		State:          req.State,
		Challenge:      req.Challenge,
		Method:         req.Method,
		Resource:       req.Resource,
		RequestedScope: req.Scope,
		WriteOffered:   writeScope,
		WriteChecked:   writeScope,
		LoggedIn:       s.loggedIn(r),
		CSRF:           csrf,
	})
}

// authorizeDecision handles the consent form: password, scope choice, and either
// a code or access_denied back to the client.
func (s *Server) authorizeDecision(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		renderError(w, http.StatusBadRequest, errorData{
			Title:   "Could not read the form",
			Message: "The authorization request was not readable.",
		})
		return
	}

	req, failure := s.parseAuthorize(r.Context(), r.PostForm)
	if failure != nil {
		if failure.RedirectTo != "" {
			redirectError(w, r, failure)
			return
		}
		renderError(w, http.StatusBadRequest, errorData{
			Title:   "This authorization request is not valid",
			Message: failure.Description,
			Detail:  failure.Code,
		})
		return
	}

	// CSRF: the hidden field must match the cookie, and the origin, when the
	// browser sends one, must be us. SameSite=Lax on the cookie is the third
	// layer, not the only one.
	if !s.checkCSRF(r) {
		renderError(w, http.StatusForbidden, errorData{
			Title:   "This form has expired",
			Message: "Reload the authorization request from the client and try again.",
			Detail:  "csrf",
		})
		return
	}
	if !sameOrigin(r, s.cfg.Issuer) {
		renderError(w, http.StatusForbidden, errorData{
			Title:   "Cross-origin request refused",
			Message: "The consent form must be submitted from CortexIQ itself.",
		})
		return
	}

	csrf := r.PostForm.Get("csrf")
	base := consentData{
		ClientName:     displayName(req.Client),
		ClientHost:     redirectHost(req.RedirectURI),
		ClientID:       req.Client.ID,
		RedirectURI:    req.RedirectURI,
		State:          req.State,
		Challenge:      req.Challenge,
		Method:         req.Method,
		Resource:       req.Resource,
		RequestedScope: req.Scope,
		WriteOffered:   hasScope(req.Scope, ScopeWrite),
		WriteChecked:   r.PostForm.Get("allow_write") == "1",
		LoggedIn:       s.loggedIn(r),
		CSRF:           csrf,
	}

	if r.PostForm.Get("decision") != "approve" {
		redirectCode(w, r, req.RedirectURI, req.State, map[string]string{
			"error":             "access_denied",
			"error_description": "The resource owner denied the request.",
		})
		return
	}

	// Login. A session cookie makes later approvals one click; without one the
	// password is required every time.
	if !s.loggedIn(r) {
		ip := clientIP(r)
		if !s.limiter.allow("login:"+ip, 5, 15*time.Minute, s.now()) {
			base.Error = "Too many attempts. Wait 15 minutes and try again."
			renderConsent(w, http.StatusTooManyRequests, base)
			return
		}
		if !constantTimeEqual(r.PostForm.Get("password"), s.cfg.AdminPassword) {
			s.log.Warn("oauth consent: wrong admin password", "ip", ip, "client", req.Client.ID)
			base.Error = "That password is not correct."
			renderConsent(w, http.StatusUnauthorized, base)
			return
		}
		s.limiter.reset("login:" + ip)
		s.startSession(w)
		base.LoggedIn = true
	}

	// The consent page can only take away write, never add it.
	granted := req.Scope
	if hasScope(granted, ScopeWrite) && r.PostForm.Get("allow_write") != "1" {
		granted = strings.Join([]string{ScopeRead}, " ")
	}

	secret, err := randomSecret(tokenPrefixCode)
	if err != nil {
		s.log.Error("oauth: cannot generate authorization code", "err", err)
		renderError(w, http.StatusInternalServerError, errorData{
			Title:   "Something went wrong",
			Message: "The authorization code could not be created.",
		})
		return
	}
	code := Code{
		Hash:          hashSecret(secret),
		ClientID:      req.Client.ID,
		RedirectURI:   req.RedirectURI,
		Scope:         granted,
		Challenge:     req.Challenge,
		ChallengeKind: req.Method,
		Resource:      req.Resource,
		Principal:     principalName(req.Client),
		ExpiresAt:     s.now().Add(s.cfg.CodeTTL),
	}
	if err := s.store.CreateCode(r.Context(), code); err != nil {
		s.log.Error("oauth: cannot store authorization code", "err", err)
		renderError(w, http.StatusInternalServerError, errorData{
			Title:   "Something went wrong",
			Message: "The authorization code could not be stored.",
		})
		return
	}
	s.store.TouchClient(r.Context(), req.Client.ID)
	s.log.Info("oauth: authorization granted",
		"client", req.Client.ID, "scope", granted, "ip", clientIP(r))

	redirectCode(w, r, req.RedirectURI, req.State, map[string]string{"code": secret})
}

// parseAuthorize validates the request parameters shared by the GET and the POST.
func (s *Server) parseAuthorize(ctx context.Context, v url.Values) (authorizeRequest, *authorizeFailure) {
	clientID := v.Get("client_id")
	redirectURI := v.Get("redirect_uri")
	state := v.Get("state")

	client, err := s.resolveClient(ctx, clientID)
	if err != nil {
		return authorizeRequest{}, &authorizeFailure{
			Code:        "invalid_request",
			Description: "Unknown client. Register the client or check its client_id.",
		}
	}
	if client.Disabled {
		return authorizeRequest{}, &authorizeFailure{
			Code:        "unauthorized_client",
			Description: "This client has been disabled.",
		}
	}

	// RFC 6749 allows omitting redirect_uri when exactly one is registered.
	if redirectURI == "" && len(client.RedirectURIs) == 1 {
		redirectURI = client.RedirectURIs[0]
	}
	if redirectURI == "" || !redirectAllowed(client, redirectURI) {
		return authorizeRequest{}, &authorizeFailure{
			Code:        "invalid_request",
			Description: "The redirect URI is not registered for this client.",
		}
	}
	// From here on the client is trusted enough to receive an error.
	fail := func(code, description string) (authorizeRequest, *authorizeFailure) {
		return authorizeRequest{}, &authorizeFailure{
			Code:        code,
			Description: description,
			RedirectTo:  redirectURI,
			State:       state,
		}
	}

	if rt := v.Get("response_type"); rt != "code" {
		return fail("unsupported_response_type", "Only the authorization code flow is supported.")
	}
	challenge := v.Get("code_challenge")
	if challenge == "" {
		return fail("invalid_request", "PKCE is required: send code_challenge.")
	}
	method := v.Get("code_challenge_method")
	if method == "" {
		// RFC 7636 defaults to "plain", which MCP forbids. Treating an omitted
		// method as S256 keeps lenient clients working without accepting plain.
		method = "S256"
	}
	if method != "S256" {
		return fail("invalid_request", "Only code_challenge_method=S256 is supported.")
	}
	if len(challenge) < 43 || len(challenge) > 128 {
		return fail("invalid_request", "The PKCE code_challenge has an invalid length.")
	}

	scope, err := normalizeScope(v.Get("scope"))
	if err != nil {
		return fail("invalid_scope", err.Error())
	}

	resource := strings.TrimRight(v.Get("resource"), "/")
	if resource == "" {
		resource = s.cfg.Resource
	}
	if !s.resourceAllowed(resource) {
		return fail("invalid_target", "The requested resource is not served here.")
	}

	return authorizeRequest{
		Client:      client,
		RedirectURI: redirectURI,
		State:       state,
		Challenge:   challenge,
		Method:      method,
		Resource:    resource,
		Scope:       scope,
	}, nil
}

// resourceAllowed accepts the configured resource and the issuer origin itself.
// The second value exists for SSE clients, which connect to the origin rather
// than to /mcp; both are the same server, so both are a valid audience.
func (s *Server) resourceAllowed(resource string) bool {
	return resource == s.cfg.Resource || resource == s.cfg.Issuer
}

// loggedIn reports whether the browser carries a valid consent session.
func (s *Server) loggedIn(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	return s.sessions.valid(c.Value, s.now())
}

// startSession sets the signed consent cookie.
func (s *Server) startSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    s.sessions.issue(s.now().Add(sessionTTL)),
		Path:     "/oauth",
		HttpOnly: true,
		Secure:   strings.HasPrefix(s.cfg.Issuer, "https://"),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

// csrfToken returns the double-submit value, creating the cookie when needed.
func (s *Server) csrfToken(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil && c.Value != "" {
		return c.Value
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	value := base64.RawURLEncoding.EncodeToString(buf)
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookie,
		Value:    value,
		Path:     "/oauth",
		HttpOnly: true,
		Secure:   strings.HasPrefix(s.cfg.Issuer, "https://"),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	return value
}

// checkCSRF compares the hidden field with the cookie in constant time.
func (s *Server) checkCSRF(r *http.Request) bool {
	c, err := r.Cookie(csrfCookie)
	if err != nil || c.Value == "" {
		return false
	}
	form := r.PostForm.Get("csrf")
	if form == "" {
		return false
	}
	return hmac.Equal([]byte(form), []byte(c.Value))
}

// sameOrigin checks the Origin header when present. A browser sends it on form
// posts; a missing header is allowed because some privacy settings strip it and
// the CSRF cookie already covers that case.
func sameOrigin(r *http.Request, issuer string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(issuer)
	if err != nil {
		return false
	}
	return origin == u.Scheme+"://"+u.Host
}

// constantTimeEqual compares two secrets without leaking their length through
// timing: both are hashed first, then compared with hmac.Equal.
func constantTimeEqual(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return hmac.Equal(ha[:], hb[:])
}

func displayName(c Client) string {
	if strings.TrimSpace(c.Name) != "" {
		return c.Name
	}
	return c.ID
}

func principalName(c Client) string {
	name := displayName(c)
	if len(name) > 60 {
		name = name[:60]
	}
	return name
}

func redirectHost(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Host == "" {
		return uri
	}
	return u.Host
}

// redirectCode sends the browser back to the client with query parameters added
// to whatever the redirect URI already carried.
func redirectCode(w http.ResponseWriter, r *http.Request, uri, state string, params map[string]string) {
	target, err := url.Parse(uri)
	if err != nil {
		http.Error(w, "invalid redirect", http.StatusInternalServerError)
		return
	}
	q := target.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	if state != "" {
		q.Set("state", state)
	}
	target.RawQuery = q.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func redirectError(w http.ResponseWriter, r *http.Request, f *authorizeFailure) {
	redirectCode(w, r, f.RedirectTo, f.State, map[string]string{
		"error":             f.Code,
		"error_description": f.Description,
	})
}
