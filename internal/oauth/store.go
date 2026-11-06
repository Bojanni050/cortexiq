// Modifications copyright (c) 2026 Bojan van der Heide

package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists the three OAuth tables. Secrets arrive already hashed: nothing
// in this file ever sees a plaintext token.
type Store struct {
	pool *pgxpool.Pool
}

// Client is one registered OAuth client.
type Client struct {
	ID           string
	Name         string
	RedirectURIs []string
	AuthMethod   string
	SecretHash   []byte
	Source       string
	Disabled     bool
}

// Code is one outstanding authorization code.
type Code struct {
	Hash          []byte
	ClientID      string
	RedirectURI   string
	Scope         string
	Challenge     string
	ChallengeKind string
	Resource      string
	Principal     string
	ExpiresAt     time.Time
}

// Token is one access or refresh token.
type Token struct {
	Hash       []byte
	Kind       string
	ClientID   string
	Scope      string
	Resource   string
	Principal  string
	FamilyID   string
	ParentHash []byte
	ExpiresAt  time.Time
}

// TokenInfo is a token row as the admin CLI shows it, without the hash.
type TokenInfo struct {
	ClientID   string
	Kind       string
	Scope      string
	Resource   string
	Principal  string
	FamilyID   string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

// NewStore wraps a pool. It does not create anything: the migration does.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// hashSecret is the only transformation applied to a secret before storage.
func hashSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// randomSecret returns a URL-safe secret with the given prefix. The prefix is
// cosmetic: it makes a leaked string recognisable in a log or a paste, and it
// lets the resource server reject obvious non-tokens before touching the DB.
func randomSecret(prefix string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// newID returns a random identifier for a client or a token family.
func newID(prefix string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// NewClientCredentials generates the identifiers for a manually registered
// client. A confidential client gets a secret, returned once, plus the digest
// that is stored; a public client authenticates with PKCE only and gets neither.
func NewClientCredentials(authMethod string) (id, secret string, hash []byte, err error) {
	id, err = randomSecret("cxq_c_")
	if err != nil {
		return "", "", nil, err
	}
	if authMethod == "" || authMethod == "none" {
		return id, "", nil, nil
	}
	secret, err = randomSecret("cxq_s_")
	if err != nil {
		return "", "", nil, err
	}
	return id, secret, hashSecret(secret), nil
}

// CreateClient inserts a client. A duplicate id is an error rather than an
// update: a CIMD document that changes its own id must not silently take over
// another client.
func (s *Store) CreateClient(ctx context.Context, c Client) error {
	if c.AuthMethod == "" {
		c.AuthMethod = "none"
	}
	if c.Source == "" {
		c.Source = "dcr"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO oauth_clients (client_id, name, redirect_uris, token_endpoint_auth_method, secret_hash, source)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		c.ID, c.Name, c.RedirectURIs, c.AuthMethod, c.SecretHash, c.Source)
	if err != nil {
		return fmt.Errorf("insert oauth client: %w", err)
	}
	return nil
}

// UpsertClient records a client that is validated elsewhere (a CIMD document).
// The id is the URL, so a re-fetch updates the metadata instead of colliding.
func (s *Store) UpsertClient(ctx context.Context, c Client) error {
	if c.AuthMethod == "" {
		c.AuthMethod = "none"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO oauth_clients (client_id, name, redirect_uris, token_endpoint_auth_method, source, last_seen_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (client_id) DO UPDATE
		SET name = EXCLUDED.name,
		    redirect_uris = EXCLUDED.redirect_uris,
		    token_endpoint_auth_method = EXCLUDED.token_endpoint_auth_method,
		    last_seen_at = now()`,
		c.ID, c.Name, c.RedirectURIs, c.AuthMethod, c.Source)
	if err != nil {
		return fmt.Errorf("upsert oauth client: %w", err)
	}
	return nil
}

// Client returns one client by id. A missing row is reported as ErrUnknownClient.
func (s *Store) Client(ctx context.Context, id string) (Client, error) {
	var c Client
	err := s.pool.QueryRow(ctx, `
		SELECT client_id, name, redirect_uris, token_endpoint_auth_method, secret_hash, source, disabled
		FROM oauth_clients WHERE client_id = $1`, id).
		Scan(&c.ID, &c.Name, &c.RedirectURIs, &c.AuthMethod, &c.SecretHash, &c.Source, &c.Disabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return Client{}, ErrUnknownClient
	}
	if err != nil {
		return Client{}, fmt.Errorf("select oauth client: %w", err)
	}
	return c, nil
}

// TouchClient records that a client was used, for the admin listing.
func (s *Store) TouchClient(ctx context.Context, id string) {
	_, _ = s.pool.Exec(ctx, `UPDATE oauth_clients SET last_seen_at = now() WHERE client_id = $1`, id)
}

// ListClients returns every client, newest first.
func (s *Store) ListClients(ctx context.Context) ([]Client, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT client_id, name, redirect_uris, token_endpoint_auth_method, source, disabled
		FROM oauth_clients ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list oauth clients: %w", err)
	}
	defer rows.Close()
	var out []Client
	for rows.Next() {
		var c Client
		if err := rows.Scan(&c.ID, &c.Name, &c.RedirectURIs, &c.AuthMethod, &c.Source, &c.Disabled); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetClientDisabled enables or disables a client. Disabling keeps the row and
// its tokens, so the audit trail survives; the token check joins on disabled.
func (s *Store) SetClientDisabled(ctx context.Context, id string, disabled bool) error {
	tag, err := s.pool.Exec(ctx, `UPDATE oauth_clients SET disabled = $2 WHERE client_id = $1`, id, disabled)
	if err != nil {
		return fmt.Errorf("update oauth client: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUnknownClient
	}
	return nil
}

// CreateCode stores a new authorization code.
func (s *Store) CreateCode(ctx context.Context, c Code) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO oauth_codes
		    (code_hash, client_id, redirect_uri, scope, code_challenge, code_challenge_method, resource, principal_name, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		c.Hash, c.ClientID, c.RedirectURI, c.Scope, c.Challenge, c.ChallengeKind, c.Resource, c.Principal, c.ExpiresAt)
	if err != nil {
		return fmt.Errorf("insert oauth code: %w", err)
	}
	return nil
}

// ConsumeCode marks a code used and returns it, in one statement. Two concurrent
// redemptions therefore cannot both succeed: the second finds used_at set and
// gets ErrInvalidGrant. An expired code is deleted instead of returned.
func (s *Store) ConsumeCode(ctx context.Context, hash []byte, now time.Time) (Code, error) {
	var c Code
	err := s.pool.QueryRow(ctx, `
		UPDATE oauth_codes SET used_at = $2
		WHERE code_hash = $1 AND used_at IS NULL AND expires_at > $2
		RETURNING client_id, redirect_uri, scope, code_challenge, code_challenge_method, resource, principal_name, expires_at`,
		hash, now).
		Scan(&c.ClientID, &c.RedirectURI, &c.Scope, &c.Challenge, &c.ChallengeKind, &c.Resource, &c.Principal, &c.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Code{}, ErrInvalidGrant
	}
	if err != nil {
		return Code{}, fmt.Errorf("consume oauth code: %w", err)
	}
	c.Hash = hash
	return c, nil
}

// CreateToken stores an access or refresh token.
func (s *Store) CreateToken(ctx context.Context, t Token) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO oauth_tokens
		    (token_hash, kind, client_id, scope, resource, principal_name, family_id, parent_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		t.Hash, t.Kind, t.ClientID, t.Scope, t.Resource, t.Principal, t.FamilyID, t.ParentHash, t.ExpiresAt)
	if err != nil {
		return fmt.Errorf("insert oauth token: %w", err)
	}
	return nil
}

// LookupAccess finds a live access token and returns the principal it stands
// for. A disabled client invalidates its tokens immediately, without a write.
func (s *Store) LookupAccess(ctx context.Context, hash []byte, now time.Time) (Token, error) {
	var t Token
	err := s.pool.QueryRow(ctx, `
		SELECT t.client_id, t.scope, t.resource, t.principal_name, t.family_id
		FROM oauth_tokens t
		JOIN oauth_clients c ON c.client_id = t.client_id
		WHERE t.token_hash = $1 AND t.kind = 'access'
		  AND t.revoked_at IS NULL AND t.expires_at > $2 AND c.disabled = FALSE`,
		hash, now).
		Scan(&t.ClientID, &t.Scope, &t.Resource, &t.Principal, &t.FamilyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Token{}, ErrInvalidToken
	}
	if err != nil {
		return Token{}, fmt.Errorf("lookup access token: %w", err)
	}
	t.Hash = hash
	return t, nil
}

// LookupRefresh finds a refresh token, including one that was already rotated:
// the rotation path has to see revoked_at to detect reuse. An expired token is
// reported as ErrInvalidGrant.
func (s *Store) LookupRefresh(ctx context.Context, hash []byte, now time.Time) (Token, error) {
	var t Token
	var revokedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT client_id, scope, resource, principal_name, family_id, parent_hash, expires_at, revoked_at
		FROM oauth_tokens
		WHERE token_hash = $1 AND kind = 'refresh'`, hash).
		Scan(&t.ClientID, &t.Scope, &t.Resource, &t.Principal, &t.FamilyID, &t.ParentHash, &t.ExpiresAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Token{}, ErrInvalidGrant
	}
	if err != nil {
		return Token{}, fmt.Errorf("lookup refresh token: %w", err)
	}
	if !t.ExpiresAt.After(now) {
		return Token{}, ErrInvalidGrant
	}
	t.Hash = hash
	return t, nil
}

// RotateRefresh atomically consumes a refresh token and stores its successor.
//
// A refresh token that was already rotated (revoked_at set) means either a bug or
// a stolen copy being replayed. Either way the whole family is revoked: the
// legitimate client will notice at its next refresh and re-authorize, while the
// attacker's copy is worthless.
func (s *Store) RotateRefresh(ctx context.Context, oldHash []byte, access, refresh Token, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin refresh rotation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var family string
	var expires time.Time
	var revokedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT family_id, expires_at, revoked_at
		FROM oauth_tokens
		WHERE token_hash = $1 AND kind = 'refresh'
		FOR UPDATE`, oldHash).Scan(&family, &expires, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidGrant
	}
	if err != nil {
		return fmt.Errorf("select refresh token: %w", err)
	}
	if revokedAt != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE oauth_tokens SET revoked_at = $2
			WHERE family_id = $1 AND revoked_at IS NULL`, family, now); err != nil {
			return fmt.Errorf("revoke token family: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return ErrRefreshReused
	}
	if !expires.After(now) {
		return ErrInvalidGrant
	}

	// The caller built the pair without knowing the family (it comes from the
	// row), so it is filled in here, inside the transaction.
	access.FamilyID, refresh.FamilyID = family, family
	access.ParentHash, refresh.ParentHash = oldHash, oldHash
	for _, t := range []Token{access, refresh} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO oauth_tokens
			    (token_hash, kind, client_id, scope, resource, principal_name, family_id, parent_hash, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			t.Hash, t.Kind, t.ClientID, t.Scope, t.Resource, t.Principal, t.FamilyID, t.ParentHash, t.ExpiresAt); err != nil {
			return fmt.Errorf("insert rotated %s token: %w", t.Kind, err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE oauth_tokens SET revoked_at = $2, last_used_at = $2 WHERE token_hash = $1`, oldHash, now); err != nil {
		return fmt.Errorf("revoke old refresh token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit refresh rotation: %w", err)
	}
	return nil
}

// Revoke revokes one token by hash. Revoking a refresh token revokes its whole
// family: the refresh token is the long-lived credential, so leaving its
// predecessors or successors alive would defeat the revocation.
func (s *Store) Revoke(ctx context.Context, hash []byte, now time.Time) error {
	var family string
	var kind string
	err := s.pool.QueryRow(ctx, `
		UPDATE oauth_tokens SET revoked_at = $2
		WHERE token_hash = $1 AND revoked_at IS NULL
		RETURNING family_id, kind`, hash, now).Scan(&family, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		// Unknown or already revoked: RFC 7009 wants the same answer.
		return nil
	}
	if err != nil {
		return fmt.Errorf("revoke oauth token: %w", err)
	}
	if kind == "refresh" {
		if _, err := s.pool.Exec(ctx, `
			UPDATE oauth_tokens SET revoked_at = $2
			WHERE family_id = $1 AND revoked_at IS NULL`, family, now); err != nil {
			return fmt.Errorf("revoke token family: %w", err)
		}
	}
	return nil
}

// RevokeClient revokes every live token of one client.
func (s *Store) RevokeClient(ctx context.Context, clientID string, now time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE oauth_tokens SET revoked_at = $2
		WHERE client_id = $1 AND revoked_at IS NULL`, clientID, now)
	if err != nil {
		return 0, fmt.Errorf("revoke client tokens: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ListTokens returns the live and recently expired tokens for the admin CLI.
func (s *Store) ListTokens(ctx context.Context, limit int) ([]TokenInfo, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT client_id, kind, scope, resource, principal_name, family_id, expires_at, revoked_at, last_used_at, created_at
		FROM oauth_tokens
		ORDER BY created_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list oauth tokens: %w", err)
	}
	defer rows.Close()
	var out []TokenInfo
	for rows.Next() {
		var t TokenInfo
		if err := rows.Scan(&t.ClientID, &t.Kind, &t.Scope, &t.Resource, &t.Principal, &t.FamilyID,
			&t.ExpiresAt, &t.RevokedAt, &t.LastUsedAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteExpired removes codes and tokens that expired before the cutoff. It runs
// on a timer; keeping a week of expired rows leaves the audit trail readable
// while stopping the table from growing without bound.
func (s *Store) DeleteExpired(ctx context.Context, cutoff time.Time) (int64, int64, error) {
	codes, err := s.pool.Exec(ctx, `DELETE FROM oauth_codes WHERE expires_at < $1`, cutoff)
	if err != nil {
		return 0, 0, fmt.Errorf("delete expired codes: %w", err)
	}
	tokens, err := s.pool.Exec(ctx, `DELETE FROM oauth_tokens WHERE expires_at < $1`, cutoff)
	if err != nil {
		return 0, 0, fmt.Errorf("delete expired tokens: %w", err)
	}
	return codes.RowsAffected(), tokens.RowsAffected(), nil
}
