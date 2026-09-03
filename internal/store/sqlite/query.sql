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
       c.icon_key AS category_icon,
       m.id AS model_id, m.name AS model_name, v.id AS variant_id,
       v.name AS variant_name, a.display_name, a.serial_number, a.color,
       a.purchase_channel, a.notes, a.created_at
FROM assets a
JOIN product_variants v ON v.tenant_id = a.tenant_id AND v.id = a.variant_id
JOIN product_models m ON m.tenant_id = v.tenant_id AND m.id = v.model_id
JOIN item_categories c ON c.tenant_id = m.tenant_id AND c.id = m.category_id
WHERE a.tenant_id = ? AND a.id = ?;

-- name: CreateCategory :exec
INSERT INTO item_categories (id, tenant_id, name, icon_key, created_at)
VALUES (?, ?, ?, ?, ?);

-- name: UpdateCategory :execrows
UPDATE item_categories
SET name = ?, icon_key = ?
WHERE tenant_id = ? AND id = ?;

-- name: CreateModel :exec
INSERT INTO product_models (id, tenant_id, category_id, name, created_at)
VALUES (?, ?, ?, ?, ?);

-- name: UpdateModel :execrows
UPDATE product_models
SET category_id = ?, name = ?
WHERE tenant_id = ? AND id = ?;

-- name: CreateVariant :exec
INSERT INTO product_variants (id, tenant_id, model_id, name, created_at)
VALUES (?, ?, ?, ?, ?);

-- name: UpdateVariant :execrows
UPDATE product_variants
SET model_id = ?, name = ?
WHERE tenant_id = ? AND id = ?;

-- name: CreateCatalogAsset :exec
INSERT INTO assets
    (id, tenant_id, variant_id, display_name, serial_number, color, purchase_channel, notes, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateCatalogAsset :execrows
UPDATE assets
SET variant_id = ?, display_name = ?, serial_number = ?, color = ?, purchase_channel = ?, notes = ?
WHERE tenant_id = ? AND id = ?;

-- name: ListCategories :many
SELECT id, tenant_id, name, icon_key, created_at
FROM item_categories
WHERE tenant_id = ?
ORDER BY name, id;

-- name: ListModels :many
SELECT m.id, m.tenant_id, m.category_id, c.name AS category_name, c.icon_key AS category_icon, m.name, m.created_at
FROM product_models m
JOIN item_categories c ON c.tenant_id = m.tenant_id AND c.id = m.category_id
WHERE m.tenant_id = ?
ORDER BY c.name, m.name, m.id;

-- name: ListVariants :many
SELECT v.id, v.tenant_id, m.category_id, c.name AS category_name,
       c.icon_key AS category_icon, v.model_id, m.name AS model_name, v.name, v.created_at
FROM product_variants v
JOIN product_models m ON m.tenant_id = v.tenant_id AND m.id = v.model_id
JOIN item_categories c ON c.tenant_id = m.tenant_id AND c.id = m.category_id
WHERE v.tenant_id = ?
ORDER BY c.name, m.name, v.name, v.id;

-- name: ListAssets :many
SELECT a.id, a.tenant_id, c.id AS category_id, c.name AS category_name,
       c.icon_key AS category_icon,
       m.id AS model_id, m.name AS model_name, v.id AS variant_id,
       v.name AS variant_name, a.display_name, a.serial_number, a.color,
       a.purchase_channel, a.notes, a.created_at
FROM assets a
JOIN product_variants v ON v.tenant_id = a.tenant_id AND v.id = a.variant_id
JOIN product_models m ON m.tenant_id = v.tenant_id AND m.id = v.model_id
JOIN item_categories c ON c.tenant_id = m.tenant_id AND c.id = m.category_id
WHERE a.tenant_id = ?
ORDER BY a.created_at DESC, a.id;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: CreateTenant :exec
INSERT INTO tenants (id, name, base_currency, created_at) VALUES (?, ?, ?, ?);

-- name: CreateUser :exec
INSERT INTO users (id, username, username_normalized, password_hash, locale, theme, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: CreateMembership :exec
INSERT INTO tenant_memberships (tenant_id, user_id, role, created_at)
VALUES (?, ?, ?, ?);

-- name: FindAccountByUsername :one
SELECT u.id AS user_id, u.username, u.password_hash, u.locale, u.theme,
       tm.tenant_id, tm.role, t.name AS tenant_name
FROM users u
JOIN tenant_memberships tm ON tm.user_id = u.id
JOIN tenants t ON t.id = tm.tenant_id
WHERE u.username_normalized = ?
ORDER BY tm.created_at
LIMIT 1;

-- name: FirstPrincipal :one
SELECT tm.tenant_id, u.id AS user_id, u.username, tm.role, t.name AS tenant_name,
       u.locale, u.theme
FROM users u
JOIN tenant_memberships tm ON tm.user_id = u.id
JOIN tenants t ON t.id = tm.tenant_id
ORDER BY u.created_at, tm.created_at
LIMIT 1;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, tenant_id, user_id, expires_at, created_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetSessionPrincipal :one
SELECT s.tenant_id, s.user_id, u.username, tm.role, t.name AS tenant_name,
       u.locale, u.theme
FROM sessions s
JOIN users u ON u.id = s.user_id
JOIN tenant_memberships tm ON tm.tenant_id = s.tenant_id AND tm.user_id = s.user_id
JOIN tenants t ON t.id = s.tenant_id
WHERE s.token_hash = ? AND s.expires_at > ?;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = ?;

-- name: UpdateUserPreferences :exec
UPDATE users SET locale = ?, theme = ? WHERE id = ?;

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

-- name: GetTenantBaseCurrency :one
SELECT base_currency, base_currency_locked
FROM tenants
WHERE id = ?;

-- name: LockTenantBaseCurrency :exec
UPDATE tenants
SET base_currency_locked = 1
WHERE id = ? AND base_currency = ?;

-- name: CreateAssetTransaction :exec
INSERT INTO asset_transactions
    (id, tenant_id, occurred_at, source, external_reference, notes, created_by_user_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: CreateAssetEvent :exec
INSERT INTO asset_events
    (id, tenant_id, asset_id, transaction_id, event_type, base_amount_minor,
     base_currency, original_amount_minor, original_currency, fx_rate_scaled,
     fx_rate_date, fx_rate_source, notes, voids_event_id, replaces_event_id,
     occurred_at, created_by_user_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetAssetEvent :one
SELECT e.id, e.tenant_id, e.asset_id, e.transaction_id, e.event_type,
       e.base_amount_minor, e.base_currency, e.original_amount_minor,
       e.original_currency, e.fx_rate_scaled, e.fx_rate_date, e.fx_rate_source,
       e.notes, e.voids_event_id, e.replaces_event_id, e.occurred_at,
       e.created_by_user_id, e.created_at,
       EXISTS (
           SELECT 1 FROM asset_events v
           WHERE v.tenant_id = e.tenant_id AND v.voids_event_id = e.id
       ) AS is_voided
FROM asset_events e
WHERE e.tenant_id = ? AND e.id = ?;

-- name: ListAssetEvents :many
SELECT e.id, e.tenant_id, e.asset_id, e.transaction_id, e.event_type,
       e.base_amount_minor, e.base_currency, e.original_amount_minor,
       e.original_currency, e.fx_rate_scaled, e.fx_rate_date, e.fx_rate_source,
       e.notes, e.voids_event_id, e.replaces_event_id, e.occurred_at,
       e.created_by_user_id, e.created_at,
       EXISTS (
           SELECT 1 FROM asset_events v
           WHERE v.tenant_id = e.tenant_id AND v.voids_event_id = e.id
       ) AS is_voided
FROM asset_events e
WHERE e.tenant_id = ? AND e.asset_id = ?
ORDER BY e.occurred_at, e.created_at, e.id;
