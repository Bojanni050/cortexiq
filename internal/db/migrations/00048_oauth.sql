-- +goose Up
-- OAuth 2.1 authorization server state. CortexIQ is both the resource server
-- (/mcp) and its own authorization server, so a hosted MCP client can register
-- itself and obtain a token without a static API token.
--
-- Every secret is stored as a SHA-256 digest: the plaintext access token, refresh
-- token and authorization code exist only in the response that carried them, so
-- a database read cannot hand out a working credential. Tokens are opaque rather
-- than JWT on purpose: revocation has to be immediate, and there is no second
-- party that needs to validate them offline.

-- One row per client. client_id is text because a Client ID Metadata Document
-- uses the client's own https URL as its identifier; dynamic registration gets a
-- generated "cxq_c_..." id instead.
CREATE TABLE oauth_clients (
    client_id                  TEXT PRIMARY KEY,
    name                       TEXT NOT NULL DEFAULT '',
    redirect_uris              TEXT[] NOT NULL DEFAULT '{}',
    -- "none" for public clients (PKCE only), "client_secret_post" for the rare
    -- confidential client. client_secret_basic is accepted on input and stored
    -- as client_secret_post.
    token_endpoint_auth_method TEXT NOT NULL DEFAULT 'none',
    -- NULL for public clients: there is no secret to compare.
    secret_hash                BYTEA NULL,
    -- How the row came to exist: 'dcr' (RFC 7591), 'cimd' (client id metadata
    -- document) or 'manual' (cortexiq oauth client add).
    source                     TEXT NOT NULL DEFAULT 'dcr',
    disabled                   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at               TIMESTAMPTZ NULL
);

CREATE INDEX oauth_clients_created_idx ON oauth_clients (created_at);

-- An authorization code is short-lived, single-use, and bound to the client,
-- the redirect URI, the PKCE challenge and the audience it was requested for.
CREATE TABLE oauth_codes (
    code_hash             BYTEA PRIMARY KEY,
    client_id             TEXT NOT NULL REFERENCES oauth_clients (client_id) ON DELETE CASCADE,
    redirect_uri          TEXT NOT NULL,
    scope                 TEXT NOT NULL,
    code_challenge        TEXT NOT NULL,
    code_challenge_method TEXT NOT NULL DEFAULT 'S256',
    resource              TEXT NOT NULL,
    principal_name        TEXT NOT NULL,
    expires_at            TIMESTAMPTZ NOT NULL,
    used_at               TIMESTAMPTZ NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX oauth_codes_expires_idx ON oauth_codes (expires_at);

-- Access and refresh tokens share one table: a refresh token is the same kind of
-- opaque secret with a longer life and a link to the token it replaced. family_id
-- groups a whole rotation chain so reuse of an already-rotated refresh token can
-- revoke everything that came out of it.
CREATE TABLE oauth_tokens (
    token_hash     BYTEA PRIMARY KEY,
    kind           TEXT NOT NULL CHECK (kind IN ('access', 'refresh')),
    client_id      TEXT NOT NULL REFERENCES oauth_clients (client_id) ON DELETE CASCADE,
    scope          TEXT NOT NULL,
    resource       TEXT NOT NULL,
    principal_name TEXT NOT NULL,
    family_id      TEXT NOT NULL,
    parent_hash    BYTEA NULL,
    expires_at     TIMESTAMPTZ NOT NULL,
    revoked_at     TIMESTAMPTZ NULL,
    last_used_at   TIMESTAMPTZ NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX oauth_tokens_client_idx ON oauth_tokens (client_id);
CREATE INDEX oauth_tokens_family_idx ON oauth_tokens (family_id);
CREATE INDEX oauth_tokens_expires_idx ON oauth_tokens (expires_at);

-- +goose Down
DROP TABLE oauth_tokens;
DROP TABLE oauth_codes;
DROP TABLE oauth_clients;
