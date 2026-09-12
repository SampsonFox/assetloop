-- name: OAuthPrincipal :one
SELECT tm.tenant_id, tm.user_id, u.username, tm.role, t.name AS tenant_name,
       u.locale, u.theme, u.accent
FROM tenant_memberships tm
JOIN users u ON u.id = tm.user_id
JOIN tenants t ON t.id = tm.tenant_id
WHERE tm.tenant_id = sqlc.arg(tenant_id) AND tm.user_id = sqlc.arg(user_id);

-- name: PutOAuthGrant :exec
INSERT INTO oauth_grants (id, tenant_id, user_id, client_id, scope, resource, created_at, expires_at, revoked)
VALUES (sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(user_id), sqlc.arg(client_id),
sqlc.arg(scope), sqlc.arg(resource), sqlc.arg(created_at), sqlc.arg(expires_at), sqlc.arg(revoked));

-- name: GetOAuthGrant :one
SELECT * FROM oauth_grants WHERE id = sqlc.arg(id);

-- name: ListOAuthGrants :many
SELECT * FROM oauth_grants WHERE tenant_id = sqlc.arg(tenant_id) AND user_id = sqlc.arg(user_id)
ORDER BY created_at DESC, id;

-- name: RevokeOAuthGrant :exec
UPDATE oauth_grants SET revoked = TRUE WHERE id = sqlc.arg(id);

-- name: PutOAuthCredential :exec
INSERT INTO oauth_credentials (hash, tenant_id, grant_id, kind, redirect_uri, challenge, expires_at, consumed)
VALUES (sqlc.arg(hash), sqlc.arg(tenant_id), sqlc.arg(grant_id), sqlc.arg(kind),
sqlc.arg(redirect_uri), sqlc.arg(challenge), sqlc.arg(expires_at), sqlc.arg(consumed));

-- name: GetOAuthCredential :one
SELECT * FROM oauth_credentials WHERE hash = sqlc.arg(hash);

-- name: ConsumeOAuthCredential :execrows
UPDATE oauth_credentials SET consumed = TRUE WHERE hash = sqlc.arg(hash) AND consumed = FALSE;

-- name: LockOAuthWrites :exec
SELECT pg_advisory_xact_lock(716044113);
