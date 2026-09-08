-- +goose Up
CREATE TABLE oauth_grants (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    client_id TEXT NOT NULL,
    scope TEXT NOT NULL,
    resource TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, user_id) REFERENCES tenant_memberships(tenant_id, user_id)
);
CREATE INDEX oauth_grants_owner_idx ON oauth_grants(tenant_id, user_id, created_at);
CREATE TABLE oauth_credentials (
    hash TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    grant_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('code', 'access', 'refresh')),
    redirect_uri TEXT NOT NULL DEFAULT '',
    challenge TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL,
    consumed BOOLEAN NOT NULL DEFAULT FALSE,
    FOREIGN KEY (tenant_id, grant_id) REFERENCES oauth_grants(tenant_id, id)
);
CREATE INDEX oauth_credentials_grant_idx ON oauth_credentials(grant_id);
CREATE INDEX oauth_credentials_expiry_idx ON oauth_credentials(expires_at);
