// Modifications copyright (c) 2026 Bojan van der Heide

package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client ID Metadata Documents (draft-ietf-oauth-client-id-metadata-document)
// let a client use its own https URL as its client_id, with the metadata served
// from that URL. Nothing is registered and no secret exists: PKCE plus the
// consent page carry the security, exactly as for a dynamically registered
// public client.
//
// The fetch is the one place where this server makes an outbound request on
// behalf of an unauthenticated caller, so it is deliberately paranoid: https
// only, no redirects, a size and time limit, and a DNS check that refuses to
// reach loopback, link-local or private addresses (SSRF).

const (
	cimdTTL         = time.Hour
	cimdMaxBytes    = 64 << 10
	cimdTimeout     = 5 * time.Second
	cimdMaxURLLen   = 2048
	cimdMediaType   = "application/json"
	cimdAuthMethod  = "none"
	clientIDPrefix  = "https://"
	clientNameLimit = 100
)

type cimdDocument struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

type cimdEntry struct {
	client  Client
	fetched time.Time
}

type cimdCache struct {
	mu      sync.Mutex
	entries map[string]cimdEntry
}

func newCIMDCache() *cimdCache {
	return &cimdCache{entries: make(map[string]cimdEntry)}
}

func (c *cimdCache) get(key string, now time.Time) (Client, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || now.Sub(e.fetched) > cimdTTL {
		return Client{}, false
	}
	return e.client, true
}

func (c *cimdCache) put(key string, cl Client, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) > 5000 {
		for k, e := range c.entries {
			if now.Sub(e.fetched) > cimdTTL {
				delete(c.entries, k)
			}
		}
	}
	c.entries[key] = cimdEntry{client: cl, fetched: now}
}

// resolveClient looks a client_id up in the local table, or fetches its metadata
// document when the id is an https URL.
func (s *Server) resolveClient(ctx context.Context, clientID string) (Client, error) {
	if clientID == "" {
		return Client{}, ErrUnknownClient
	}
	if strings.HasPrefix(clientID, clientIDPrefix) {
		cl, err := s.cimdClient(ctx, clientID)
		if err != nil {
			return Client{}, err
		}
		if cl.Disabled {
			return Client{}, ErrUnknownClient
		}
		return cl, nil
	}
	return s.store.Client(ctx, clientID)
}

// cimdClient fetches and validates a metadata document, with a one-hour cache.
func (s *Server) cimdClient(ctx context.Context, raw string) (Client, error) {
	now := s.now()
	if cl, ok := s.cimd.get(raw, now); ok {
		return cl, nil
	}
	cl, err := s.fetchCIMD(ctx, raw)
	if err != nil {
		return Client{}, err
	}
	// Keep a row so the admin listing and the token table have something to join
	// against. A CIMD client never has a secret.
	cl.Source = "cimd"
	if err := s.store.UpsertClient(ctx, cl); err != nil {
		return Client{}, err
	}
	// The stored row is authoritative for the disabled flag: an admin must be
	// able to block a CIMD client without the cache overriding that decision.
	if stored, err := s.store.Client(ctx, raw); err == nil {
		cl = stored
	}
	s.cimd.put(raw, cl, now)
	return cl, nil
}

func (s *Server) fetchCIMD(ctx context.Context, raw string) (Client, error) {
	u, err := validateCIMDURL(raw)
	if err != nil {
		return Client{}, err
	}
	if err := ensurePublicHost(ctx, u.Hostname()); err != nil {
		return Client{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return Client{}, fmt.Errorf("client metadata request: %w", err)
	}
	req.Header.Set("Accept", cimdMediaType)
	req.Header.Set("User-Agent", "cortexiq-oauth/1.0")

	client := &http.Client{
		Timeout: cimdTimeout,
		Transport: &http.Transport{
			DialContext: dialPublic,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			// A redirect could point at a private address that passed the DNS
			// check above, so it is refused rather than followed.
			return errors.New("client metadata document must not redirect")
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return Client{}, fmt.Errorf("fetch client metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Client{}, fmt.Errorf("client metadata: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, cimdMaxBytes+1))
	if err != nil {
		return Client{}, fmt.Errorf("read client metadata: %w", err)
	}
	if len(body) > cimdMaxBytes {
		return Client{}, errors.New("client metadata document too large")
	}

	var doc cimdDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return Client{}, fmt.Errorf("parse client metadata: %w", err)
	}
	// The document must name the URL it was fetched from; otherwise a document
	// could claim another client's identity.
	if doc.ClientID != raw {
		return Client{}, errors.New("client metadata: client_id does not match its URL")
	}
	if len(doc.RedirectURIs) == 0 {
		return Client{}, errors.New("client metadata: no redirect_uris")
	}
	for _, uri := range doc.RedirectURIs {
		if err := validateRedirectURI(uri); err != nil {
			return Client{}, fmt.Errorf("client metadata: %w", err)
		}
	}
	if doc.TokenEndpointAuthMethod != "" && doc.TokenEndpointAuthMethod != cimdAuthMethod {
		return Client{}, fmt.Errorf("client metadata: unsupported token_endpoint_auth_method %q", doc.TokenEndpointAuthMethod)
	}

	name := strings.TrimSpace(doc.ClientName)
	if name == "" {
		name = u.Hostname()
	}
	if len(name) > clientNameLimit {
		name = name[:clientNameLimit]
	}
	return Client{
		ID:           raw,
		Name:         name,
		RedirectURIs: doc.RedirectURIs,
		AuthMethod:   cimdAuthMethod,
		Source:       "cimd",
	}, nil
}

// validateCIMDURL enforces the shape of a client id URL.
func validateCIMDURL(raw string) (*url.URL, error) {
	if len(raw) > cimdMaxURLLen {
		return nil, errors.New("client id too long")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("client id: %w", err)
	}
	if u.Scheme != "https" {
		return nil, errors.New("client id must be an https URL")
	}
	if u.Host == "" || u.User != nil || u.Fragment != "" {
		return nil, errors.New("client id must be a plain https URL")
	}
	return u, nil
}

// ensurePublicHost resolves a hostname and refuses anything that is not a
// globally routable address. A name that resolves to several addresses is
// refused when any of them is internal: the HTTP client could pick that one.
func ensurePublicHost(ctx context.Context, host string) error {
	lookupCtx, cancel := context.WithTimeout(ctx, cimdTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil {
		return fmt.Errorf("resolve client id host: %w", err)
	}
	if len(addrs) == 0 {
		return errors.New("resolve client id host: no addresses")
	}
	for _, a := range addrs {
		if !isPublicIP(a.IP) {
			return errors.New("client id host resolves to a non-public address")
		}
	}
	return nil
}

// dialPublic is the transport's dialer. Checking the hostname before the request
// is not enough on its own: between that check and the connect, the name can
// resolve differently (DNS rebinding). The address actually dialed gets the same
// rule, so the only way out is a globally routable address.
func dialPublic(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: cimdTimeout}
	var lastErr error
	for _, a := range addrs {
		if !isPublicIP(a.IP) {
			return nil, errors.New("client metadata host resolves to a non-public address")
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no addresses to dial")
	}
	return nil, lastErr
}

func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return false
	}
	// 100.64.0.0/10 (carrier-grade NAT) is not "private" as far as net.IP is
	// concerned but is just as unreachable from the public internet.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return false
	}
	return true
}

// validateRedirectURI accepts https anywhere and http only on the loopback
// interface, which is what native MCP clients use for their callback listener.
func validateRedirectURI(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid redirect_uri: %w", err)
	}
	if u.Fragment != "" || u.User != nil {
		return errors.New("invalid redirect_uri: no fragment or userinfo")
	}
	switch u.Scheme {
	case "https":
		if u.Host == "" {
			return errors.New("invalid redirect_uri: missing host")
		}
		return nil
	case "http":
		host := u.Hostname()
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			return nil
		}
		return errors.New("invalid redirect_uri: http is only allowed on localhost")
	default:
		// Custom schemes are used by some native clients, but the MCP spec
		// requires https or localhost, and allowing them widens the surface for
		// no client we actually support.
		return fmt.Errorf("invalid redirect_uri: unsupported scheme %q", u.Scheme)
	}
}

// redirectAllowed reports whether uri is registered for the client. Exact string
// matching is the only safe comparison (RFC 6749 §3.1.2.3).
func redirectAllowed(client Client, uri string) bool {
	for _, registered := range client.RedirectURIs {
		if registered == uri {
			return true
		}
	}
	return false
}
