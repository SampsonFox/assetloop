-- name: EnsureTenant :exec
INSERT INTO tenants (id, name, base_currency, created_at) VALUES (?, ?, ?, ?)
ON CONFLICT (id) DO NOTHING;

-- name: EnsureCategory :one
INSERT INTO item_categories (id, tenant_id, name, created_at) VALUES (?, ?, ?, ?)
ON CONFLICT (tenant_id, name) DO UPDATE SET name = excluded.name
RETURNING id;

-- name: EnsureModel :one
INSERT INTO product_models (id, tenant_id, category_id, name, created_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (tenant_id, category_id, name) DO UPDATE SET name = excluded.name
RETURNING id;

-- name: EnsureVariant :one
INSERT INTO product_variants (id, tenant_id, model_id, name, created_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (tenant_id, model_id, name) DO UPDATE SET name = excluded.name
RETURNING id;

-- name: CreateAsset :exec
INSERT INTO assets (id, tenant_id, variant_id, display_name, created_at) VALUES (?, ?, ?, ?, ?);

-- name: GetAsset :one
SELECT a.id, a.tenant_id, c.id AS category_id, c.name AS category_name,
       m.id AS model_id, m.name AS model_name, v.id AS variant_id,
       v.name AS variant_name, a.display_name, a.created_at
FROM assets a
JOIN product_variants v ON v.tenant_id = a.tenant_id AND v.id = a.variant_id
JOIN product_models m ON m.tenant_id = v.tenant_id AND m.id = v.model_id
JOIN item_categories c ON c.tenant_id = m.tenant_id AND c.id = m.category_id
WHERE a.tenant_id = ? AND a.id = ?;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: CreateTenant :exec
INSERT INTO tenants (id, name, base_currency, created_at) VALUES (?, ?, ?, ?);

-- name: CreateUser :exec
INSERT INTO users (id, username, username_normalized, password_hash, created_at)
VALUES (?, ?, ?, ?, ?);

-- name: CreateMembership :exec
INSERT INTO tenant_memberships (tenant_id, user_id, role, created_at)
VALUES (?, ?, ?, ?);

-- name: FindAccountByUsername :one
SELECT u.id AS user_id, u.username, u.password_hash,
       tm.tenant_id, tm.role, t.name AS tenant_name
FROM users u
JOIN tenant_memberships tm ON tm.user_id = u.id
JOIN tenants t ON t.id = tm.tenant_id
WHERE u.username_normalized = ?
ORDER BY tm.created_at
LIMIT 1;

-- name: FirstPrincipal :one
SELECT tm.tenant_id, u.id AS user_id, u.username, tm.role, t.name AS tenant_name
FROM users u
JOIN tenant_memberships tm ON tm.user_id = u.id
JOIN tenants t ON t.id = tm.tenant_id
ORDER BY u.created_at, tm.created_at
LIMIT 1;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, tenant_id, user_id, expires_at, created_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetSessionPrincipal :one
SELECT s.tenant_id, s.user_id, u.username, tm.role, t.name AS tenant_name
FROM sessions s
JOIN users u ON u.id = s.user_id
JOIN tenant_memberships tm ON tm.tenant_id = s.tenant_id AND tm.user_id = s.user_id
JOIN tenants t ON t.id = s.tenant_id
WHERE s.token_hash = ? AND s.expires_at > ?;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = ?;

-- name: ListMembers :many
SELECT u.id AS user_id, u.username, tm.role, tm.created_at
FROM tenant_memberships tm
JOIN users u ON u.id = tm.user_id
WHERE tm.tenant_id = ?
ORDER BY u.username_normalized;

-- name: CreateSecurityAuditEvent :exec
INSERT INTO security_audit_events
    (id, tenant_id, actor_user_id, action, target_user_id, detail, occurred_at)
VALUES (?, ?, ?, ?, ?, ?, ?);
