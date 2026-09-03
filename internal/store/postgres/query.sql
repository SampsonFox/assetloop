-- name: EnsureTenant :exec
INSERT INTO tenants (id, name, base_currency, created_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO NOTHING;

-- name: EnsureCategory :one
INSERT INTO item_categories (id, tenant_id, name, created_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (tenant_id, name) DO UPDATE SET name = excluded.name
RETURNING id;

-- name: EnsureModel :one
INSERT INTO product_models (id, tenant_id, category_id, name, created_at) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (tenant_id, category_id, name) DO UPDATE SET name = excluded.name
RETURNING id;

-- name: EnsureVariant :one
INSERT INTO product_variants (id, tenant_id, model_id, name, created_at) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (tenant_id, model_id, name) DO UPDATE SET name = excluded.name
RETURNING id;

-- name: CreateAsset :exec
INSERT INTO assets (id, tenant_id, variant_id, display_name, created_at) VALUES ($1, $2, $3, $4, $5);

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
WHERE a.tenant_id = $1 AND a.id = $2;

-- name: CreateCategory :exec
INSERT INTO item_categories (id, tenant_id, name, icon_key, created_at)
VALUES ($1, $2, $3, $4, $5);

-- name: UpdateCategory :execrows
UPDATE item_categories
SET name = $1, icon_key = $2
WHERE tenant_id = $3 AND id = $4;

-- name: CreateModel :exec
INSERT INTO product_models (id, tenant_id, category_id, name, created_at)
VALUES ($1, $2, $3, $4, $5);

-- name: UpdateModel :execrows
UPDATE product_models
SET category_id = $1, name = $2
WHERE tenant_id = $3 AND id = $4;

-- name: CreateVariant :exec
INSERT INTO product_variants (id, tenant_id, model_id, name, created_at)
VALUES ($1, $2, $3, $4, $5);

-- name: UpdateVariant :execrows
UPDATE product_variants
SET model_id = $1, name = $2
WHERE tenant_id = $3 AND id = $4;

-- name: DeleteVariant :execrows
DELETE FROM product_variants AS variant
WHERE variant.tenant_id = $1 AND variant.id = $2
  AND NOT EXISTS (
    SELECT 1 FROM assets
    WHERE assets.tenant_id = variant.tenant_id
      AND assets.variant_id = variant.id
  );

-- name: CreateCatalogAsset :exec
INSERT INTO assets
    (id, tenant_id, variant_id, display_name, serial_number, color, purchase_channel, notes, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: UpdateCatalogAsset :execrows
UPDATE assets
SET variant_id = $1, display_name = $2, serial_number = $3, color = $4, purchase_channel = $5, notes = $6
WHERE tenant_id = $7 AND id = $8;

-- name: ListCategories :many
SELECT id, tenant_id, name, icon_key, created_at
FROM item_categories
WHERE tenant_id = $1
ORDER BY name, id;

-- name: ListModels :many
SELECT m.id, m.tenant_id, m.category_id, c.name AS category_name, c.icon_key AS category_icon, m.name, m.created_at
FROM product_models m
JOIN item_categories c ON c.tenant_id = m.tenant_id AND c.id = m.category_id
WHERE m.tenant_id = $1
ORDER BY c.name, m.name, m.id;

-- name: ListModelsWithVariants :many
WITH filtered_models AS (
    SELECT m.id, m.tenant_id, m.category_id, c.name AS category_name,
           c.icon_key AS category_icon, m.name, m.created_at
    FROM product_models m
    JOIN item_categories c ON c.tenant_id = m.tenant_id AND c.id = m.category_id
    WHERE m.tenant_id = sqlc.arg(tenant_id)
      AND (sqlc.arg(search_query)::text = '' OR m.name ILIKE '%' || sqlc.arg(search_query)::text || '%' OR c.name ILIKE '%' || sqlc.arg(search_query)::text || '%')
      AND (sqlc.arg(category_filter)::text = '' OR m.category_id::text = sqlc.arg(category_filter)::text)
),
paged_models AS (
    SELECT *, COUNT(*) OVER () AS total_count,
           ROW_NUMBER() OVER (ORDER BY
             CASE WHEN sqlc.arg(sort_key)::text = 'category' AND sqlc.arg(sort_direction)::text = 'asc' THEN LOWER(category_name) END ASC,
             CASE WHEN sqlc.arg(sort_key)::text = 'category' AND sqlc.arg(sort_direction)::text = 'desc' THEN LOWER(category_name) END DESC,
             CASE WHEN sqlc.arg(sort_key)::text = 'name' AND sqlc.arg(sort_direction)::text = 'asc' THEN LOWER(name) END ASC,
             CASE WHEN sqlc.arg(sort_key)::text = 'name' AND sqlc.arg(sort_direction)::text = 'desc' THEN LOWER(name) END DESC,
             CASE WHEN sqlc.arg(sort_key)::text = 'created' AND sqlc.arg(sort_direction)::text = 'asc' THEN created_at END ASC,
             CASE WHEN sqlc.arg(sort_key)::text = 'created' AND sqlc.arg(sort_direction)::text = 'desc' THEN created_at END DESC,
             LOWER(category_name), LOWER(name), id) AS page_order
    FROM filtered_models
    LIMIT sqlc.arg(page_size)::bigint OFFSET sqlc.arg(page_offset)::bigint
)
SELECT pm.id, pm.tenant_id, pm.category_id, pm.category_name, pm.category_icon,
       pm.name, pm.created_at, pm.total_count, pm.page_order,
       v.id AS variant_id, v.name AS variant_name, v.created_at AS variant_created_at
FROM paged_models pm
LEFT JOIN product_variants v ON v.tenant_id = pm.tenant_id AND v.model_id = pm.id
ORDER BY pm.page_order, LOWER(v.name), v.id;

-- name: ListVariants :many
SELECT v.id, v.tenant_id, m.category_id, c.name AS category_name,
       c.icon_key AS category_icon, v.model_id, m.name AS model_name, v.name, v.created_at
FROM product_variants v
JOIN product_models m ON m.tenant_id = v.tenant_id AND m.id = v.model_id
JOIN item_categories c ON c.tenant_id = m.tenant_id AND c.id = m.category_id
WHERE v.tenant_id = $1
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
WHERE a.tenant_id = $1
ORDER BY a.created_at DESC, a.id;

-- name: ListAssetsWithSummary :many
WITH effective_events AS (
SELECT e.*
FROM asset_events e
WHERE e.tenant_id = sqlc.arg(tenant_id)
  AND e.event_type != 'void'
  AND NOT EXISTS (
      SELECT 1 FROM asset_events void_event
      WHERE void_event.tenant_id = e.tenant_id AND void_event.voids_event_id = e.id
  )
),
event_rollup AS (
SELECT asset_id,
       SUM(CASE WHEN base_amount_minor < 0 THEN -base_amount_minor ELSE 0 END)::bigint AS expense_minor,
       SUM(CASE WHEN base_amount_minor > 0 THEN base_amount_minor ELSE 0 END)::bigint AS income_minor,
       SUM(base_amount_minor)::bigint AS net_minor,
       BOOL_OR(event_type = 'purchase') AS has_purchase,
       BOOL_OR(event_type = 'sale') AS has_sale
FROM effective_events
GROUP BY asset_id
),
ranked_events AS (
SELECT asset_id, event_type,
       ROW_NUMBER() OVER (PARTITION BY asset_id ORDER BY occurred_at DESC, created_at DESC, id DESC) AS event_rank
FROM effective_events
),
asset_rows AS (
SELECT a.id, a.tenant_id, c.id AS category_id, c.name AS category_name,
       c.icon_key AS category_icon,
       m.id AS model_id, m.name AS model_name, v.id AS variant_id,
       v.name AS variant_name, a.display_name, a.serial_number, a.color,
       a.purchase_channel, a.notes, a.created_at,
       COALESCE(er.expense_minor, 0)::bigint AS expense_minor,
       COALESCE(er.income_minor, 0)::bigint AS income_minor,
       COALESCE(er.net_minor, 0)::bigint AS net_minor,
       COALESCE(er.has_purchase, FALSE) AS has_purchase,
       COALESCE(er.has_sale, FALSE) AS has_sale,
       COALESCE(re.event_type, '') AS latest_event_type,
       t.base_currency::text AS base_currency
FROM assets a
JOIN product_variants v ON v.tenant_id = a.tenant_id AND v.id = a.variant_id
JOIN product_models m ON m.tenant_id = v.tenant_id AND m.id = v.model_id
JOIN item_categories c ON c.tenant_id = m.tenant_id AND c.id = m.category_id
JOIN tenants t ON t.id = a.tenant_id
LEFT JOIN event_rollup er ON er.asset_id = a.id
LEFT JOIN ranked_events re ON re.asset_id = a.id AND re.event_rank = 1
WHERE a.tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(search_query)::text = '' OR (
       a.display_name ILIKE '%' || sqlc.arg(search_query)::text || '%' OR
       a.serial_number ILIKE '%' || sqlc.arg(search_query)::text || '%' OR
       a.color ILIKE '%' || sqlc.arg(search_query)::text || '%' OR
       a.purchase_channel ILIKE '%' || sqlc.arg(search_query)::text || '%' OR
       a.notes ILIKE '%' || sqlc.arg(search_query)::text || '%' OR
       m.name ILIKE '%' || sqlc.arg(search_query)::text || '%' OR
       v.name ILIKE '%' || sqlc.arg(search_query)::text || '%' OR
       c.name ILIKE '%' || sqlc.arg(search_query)::text || '%'
  ))
)
SELECT id, tenant_id, category_id, category_name, category_icon, model_id, model_name,
       variant_id, variant_name, display_name, serial_number, color, purchase_channel, notes, created_at,
       expense_minor, income_minor, net_minor,
       CASE
           WHEN has_sale THEN 'sold'
           WHEN has_purchase AND latest_event_type = 'repair' THEN 'repairing'
           WHEN has_purchase THEN 'active'
           ELSE 'unacquired'
       END AS status,
       base_currency
FROM asset_rows
WHERE sqlc.arg(status_filter)::text IN ('', 'all')
   OR (sqlc.arg(status_filter)::text = 'sold' AND has_sale)
   OR (sqlc.arg(status_filter)::text = 'repairing' AND NOT has_sale AND has_purchase AND latest_event_type = 'repair')
   OR (sqlc.arg(status_filter)::text = 'active' AND NOT has_sale AND has_purchase AND latest_event_type != 'repair')
   OR (sqlc.arg(status_filter)::text = 'unacquired' AND NOT has_sale AND NOT has_purchase)
ORDER BY
  CASE WHEN sqlc.arg(sort_key)::text = 'name' AND sqlc.arg(sort_direction)::text = 'asc' THEN LOWER(display_name) END ASC,
  CASE WHEN sqlc.arg(sort_key)::text = 'name' AND sqlc.arg(sort_direction)::text = 'desc' THEN LOWER(display_name) END DESC,
  CASE WHEN sqlc.arg(sort_key)::text = 'model' AND sqlc.arg(sort_direction)::text = 'asc' THEN LOWER(model_name || ' ' || variant_name) END ASC,
  CASE WHEN sqlc.arg(sort_key)::text = 'model' AND sqlc.arg(sort_direction)::text = 'desc' THEN LOWER(model_name || ' ' || variant_name) END DESC,
  CASE WHEN sqlc.arg(sort_key)::text = 'status' AND sqlc.arg(sort_direction)::text = 'asc' THEN status END ASC,
  CASE WHEN sqlc.arg(sort_key)::text = 'status' AND sqlc.arg(sort_direction)::text = 'desc' THEN status END DESC,
  CASE WHEN sqlc.arg(sort_key)::text = 'net' AND sqlc.arg(sort_direction)::text = 'asc' THEN net_minor END ASC,
  CASE WHEN sqlc.arg(sort_key)::text = 'net' AND sqlc.arg(sort_direction)::text = 'desc' THEN net_minor END DESC,
  CASE WHEN sqlc.arg(sort_key)::text = 'cost' AND sqlc.arg(sort_direction)::text = 'asc' THEN expense_minor END ASC,
  CASE WHEN sqlc.arg(sort_key)::text = 'cost' AND sqlc.arg(sort_direction)::text = 'desc' THEN expense_minor END DESC,
  CASE WHEN sqlc.arg(sort_key)::text = 'created' AND sqlc.arg(sort_direction)::text = 'asc' THEN created_at END ASC,
  CASE WHEN sqlc.arg(sort_key)::text = 'created' AND sqlc.arg(sort_direction)::text = 'desc' THEN created_at END DESC,
  id
LIMIT sqlc.arg(page_size)::bigint OFFSET sqlc.arg(page_offset)::bigint;

-- name: CountAssetsWithSummary :one
WITH effective_events AS (
SELECT e.*
FROM asset_events e
WHERE e.tenant_id = sqlc.arg(tenant_id)
  AND e.event_type != 'void'
  AND NOT EXISTS (
      SELECT 1 FROM asset_events void_event
      WHERE void_event.tenant_id = e.tenant_id AND void_event.voids_event_id = e.id
  )
),
event_rollup AS (
SELECT asset_id,
       BOOL_OR(event_type = 'purchase') AS has_purchase,
       BOOL_OR(event_type = 'sale') AS has_sale
FROM effective_events
GROUP BY asset_id
),
ranked_events AS (
SELECT asset_id, event_type,
       ROW_NUMBER() OVER (PARTITION BY asset_id ORDER BY occurred_at DESC, created_at DESC, id DESC) AS event_rank
FROM effective_events
),
asset_rows AS (
SELECT a.id,
       COALESCE(er.has_purchase, FALSE) AS has_purchase,
       COALESCE(er.has_sale, FALSE) AS has_sale,
       COALESCE(re.event_type, '') AS latest_event_type
FROM assets a
JOIN product_variants v ON v.tenant_id = a.tenant_id AND v.id = a.variant_id
JOIN product_models m ON m.tenant_id = v.tenant_id AND m.id = v.model_id
JOIN item_categories c ON c.tenant_id = m.tenant_id AND c.id = m.category_id
LEFT JOIN event_rollup er ON er.asset_id = a.id
LEFT JOIN ranked_events re ON re.asset_id = a.id AND re.event_rank = 1
WHERE a.tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(search_query)::text = '' OR (
       a.display_name ILIKE '%' || sqlc.arg(search_query)::text || '%' OR
       a.serial_number ILIKE '%' || sqlc.arg(search_query)::text || '%' OR
       a.color ILIKE '%' || sqlc.arg(search_query)::text || '%' OR
       a.purchase_channel ILIKE '%' || sqlc.arg(search_query)::text || '%' OR
       a.notes ILIKE '%' || sqlc.arg(search_query)::text || '%' OR
       m.name ILIKE '%' || sqlc.arg(search_query)::text || '%' OR
       v.name ILIKE '%' || sqlc.arg(search_query)::text || '%' OR
       c.name ILIKE '%' || sqlc.arg(search_query)::text || '%'
  ))
)
SELECT COUNT(*) FROM asset_rows
WHERE sqlc.arg(status_filter)::text IN ('', 'all')
   OR (sqlc.arg(status_filter)::text = 'sold' AND has_sale)
   OR (sqlc.arg(status_filter)::text = 'repairing' AND NOT has_sale AND has_purchase AND latest_event_type = 'repair')
   OR (sqlc.arg(status_filter)::text = 'active' AND NOT has_sale AND has_purchase AND latest_event_type != 'repair')
   OR (sqlc.arg(status_filter)::text = 'unacquired' AND NOT has_sale AND NOT has_purchase);

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: CreateTenant :exec
INSERT INTO tenants (id, name, base_currency, created_at) VALUES ($1, $2, $3, $4);

-- name: CreateUser :exec
INSERT INTO users (id, username, username_normalized, password_hash, locale, theme, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: CreateMembership :exec
INSERT INTO tenant_memberships (tenant_id, user_id, role, created_at)
VALUES ($1, $2, $3, $4);

-- name: FindAccountByUsername :one
SELECT u.id AS user_id, u.username, u.password_hash, u.locale, u.theme,
       tm.tenant_id, tm.role, t.name AS tenant_name
FROM users u
JOIN tenant_memberships tm ON tm.user_id = u.id
JOIN tenants t ON t.id = tm.tenant_id
WHERE u.username_normalized = $1
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
VALUES ($1, $2, $3, $4, $5);

-- name: GetSessionPrincipal :one
SELECT s.tenant_id, s.user_id, u.username, tm.role, t.name AS tenant_name,
       u.locale, u.theme
FROM sessions s
JOIN users u ON u.id = s.user_id
JOIN tenant_memberships tm ON tm.tenant_id = s.tenant_id AND tm.user_id = s.user_id
JOIN tenants t ON t.id = s.tenant_id
WHERE s.token_hash = $1 AND s.expires_at > $2;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: UpdateUserPreferences :exec
UPDATE users SET locale = $1, theme = $2 WHERE id = $3;

-- name: ListMembers :many
SELECT u.id AS user_id, u.username, tm.role, tm.created_at
FROM tenant_memberships tm
JOIN users u ON u.id = tm.user_id
WHERE tm.tenant_id = $1
ORDER BY u.username_normalized;

-- name: ListMembersPage :many
SELECT u.id AS user_id, u.username, tm.role, tm.created_at, COUNT(*) OVER () AS total_count
FROM tenant_memberships tm
JOIN users u ON u.id = tm.user_id
WHERE tm.tenant_id = sqlc.arg(tenant_id)
  AND (sqlc.arg(search_query)::text = '' OR u.username ILIKE '%' || sqlc.arg(search_query)::text || '%')
  AND (sqlc.arg(role_filter)::text = '' OR tm.role = sqlc.arg(role_filter)::text)
ORDER BY
  CASE WHEN sqlc.arg(sort_key)::text = 'username' AND sqlc.arg(sort_direction)::text = 'asc' THEN u.username_normalized END ASC,
  CASE WHEN sqlc.arg(sort_key)::text = 'username' AND sqlc.arg(sort_direction)::text = 'desc' THEN u.username_normalized END DESC,
  CASE WHEN sqlc.arg(sort_key)::text = 'role' AND sqlc.arg(sort_direction)::text = 'asc' THEN tm.role END ASC,
  CASE WHEN sqlc.arg(sort_key)::text = 'role' AND sqlc.arg(sort_direction)::text = 'desc' THEN tm.role END DESC,
  CASE WHEN sqlc.arg(sort_key)::text = 'created' AND sqlc.arg(sort_direction)::text = 'asc' THEN tm.created_at END ASC,
  CASE WHEN sqlc.arg(sort_key)::text = 'created' AND sqlc.arg(sort_direction)::text = 'desc' THEN tm.created_at END DESC,
  u.id
LIMIT sqlc.arg(page_size)::bigint OFFSET sqlc.arg(page_offset)::bigint;

-- name: CreateSecurityAuditEvent :exec
INSERT INTO security_audit_events
    (id, tenant_id, actor_user_id, action, target_user_id, detail, occurred_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetTenantBaseCurrency :one
SELECT base_currency, base_currency_locked
FROM tenants
WHERE id = $1;

-- name: GetPortfolioSummary :one
WITH effective_events AS (
    SELECT e.*
    FROM asset_events e
    WHERE e.tenant_id = sqlc.arg(tenant_id)
      AND e.event_type != 'void'
      AND NOT EXISTS (
          SELECT 1 FROM asset_events void_event
          WHERE void_event.tenant_id = e.tenant_id AND void_event.voids_event_id = e.id
      )
)
SELECT COUNT(DISTINCT a.id)::bigint AS asset_count,
       COALESCE(SUM(CASE WHEN e.base_amount_minor < 0 THEN -e.base_amount_minor ELSE 0 END), 0)::bigint AS expense_minor,
       COALESCE(SUM(CASE WHEN e.base_amount_minor > 0 THEN e.base_amount_minor ELSE 0 END), 0)::bigint AS income_minor,
       COALESCE(SUM(e.base_amount_minor), 0)::bigint AS net_minor,
       t.base_currency::text AS base_currency
FROM tenants t
LEFT JOIN assets a ON a.tenant_id = t.id
LEFT JOIN effective_events e ON e.asset_id = a.id
WHERE t.id = sqlc.arg(tenant_id)
GROUP BY t.base_currency;

-- name: LockTenantBaseCurrency :exec
UPDATE tenants
SET base_currency_locked = TRUE
WHERE id = $1 AND base_currency = $2;

-- name: CreateAssetTransaction :exec
INSERT INTO asset_transactions
    (id, tenant_id, occurred_at, source, external_reference, notes, created_by_user_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: CreateAssetEvent :exec
INSERT INTO asset_events
    (id, tenant_id, asset_id, transaction_id, event_type, base_amount_minor,
     base_currency, original_amount_minor, original_currency, fx_rate_scaled,
     fx_rate_date, fx_rate_source, notes, voids_event_id, replaces_event_id,
     occurred_at, created_by_user_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18);

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
WHERE e.tenant_id = $1 AND e.id = $2;

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
WHERE e.tenant_id = $1 AND e.asset_id = $2
ORDER BY e.occurred_at, e.created_at, e.id;

-- name: ListAssetEventsPage :many
SELECT e.id, e.tenant_id, e.asset_id, e.transaction_id, e.event_type,
       e.base_amount_minor, e.base_currency, e.original_amount_minor,
       e.original_currency, e.fx_rate_scaled, e.fx_rate_date, e.fx_rate_source,
       e.notes, e.voids_event_id, e.replaces_event_id, e.occurred_at,
       e.created_by_user_id, e.created_at,
       EXISTS (
           SELECT 1 FROM asset_events v
           WHERE v.tenant_id = e.tenant_id AND v.voids_event_id = e.id
       ) AS is_voided,
       COUNT(*) OVER ()::bigint AS total_count
FROM asset_events e
WHERE e.tenant_id = sqlc.arg(tenant_id) AND e.asset_id = sqlc.arg(asset_id)
  AND (sqlc.arg(search_query)::text = '' OR e.notes ILIKE '%' || sqlc.arg(search_query)::text || '%' OR COALESCE(e.fx_rate_source, '') ILIKE '%' || sqlc.arg(search_query)::text || '%')
  AND (sqlc.arg(event_type_filter)::text = '' OR e.event_type = sqlc.arg(event_type_filter)::text)
ORDER BY
  CASE WHEN sqlc.arg(sort_key)::text = 'occurred' AND sqlc.arg(sort_direction)::text = 'asc' THEN e.occurred_at END ASC,
  CASE WHEN sqlc.arg(sort_key)::text = 'occurred' AND sqlc.arg(sort_direction)::text = 'desc' THEN e.occurred_at END DESC,
  CASE WHEN sqlc.arg(sort_key)::text = 'amount' AND sqlc.arg(sort_direction)::text = 'asc' THEN e.base_amount_minor END ASC,
  CASE WHEN sqlc.arg(sort_key)::text = 'amount' AND sqlc.arg(sort_direction)::text = 'desc' THEN e.base_amount_minor END DESC,
  CASE WHEN sqlc.arg(sort_key)::text = 'type' AND sqlc.arg(sort_direction)::text = 'asc' THEN e.event_type END ASC,
  CASE WHEN sqlc.arg(sort_key)::text = 'type' AND sqlc.arg(sort_direction)::text = 'desc' THEN e.event_type END DESC,
  e.created_at DESC, e.id DESC
LIMIT sqlc.arg(page_size)::bigint OFFSET sqlc.arg(page_offset)::bigint;

-- name: GetAssetSummary :one
WITH effective_events AS (
    SELECT e.*
    FROM asset_events e
    WHERE e.tenant_id = sqlc.arg(tenant_id) AND e.asset_id = sqlc.arg(asset_id)
      AND e.event_type != 'void'
      AND NOT EXISTS (SELECT 1 FROM asset_events v WHERE v.tenant_id = e.tenant_id AND v.voids_event_id = e.id)
), latest_event AS (
    SELECT event_type FROM effective_events ORDER BY occurred_at DESC, created_at DESC, id DESC LIMIT 1
)
SELECT t.base_currency::text AS base_currency,
       COALESCE(SUM(CASE WHEN e.base_amount_minor < 0 THEN -e.base_amount_minor ELSE 0 END), 0)::bigint AS expense_minor,
       COALESCE(SUM(CASE WHEN e.base_amount_minor > 0 THEN e.base_amount_minor ELSE 0 END), 0)::bigint AS income_minor,
       COALESCE(SUM(e.base_amount_minor), 0)::bigint AS net_minor,
       CASE
         WHEN COALESCE(BOOL_OR(e.event_type = 'sale'), FALSE) THEN 'sold'
         WHEN NOT COALESCE(BOOL_OR(e.event_type = 'purchase'), FALSE) THEN 'unacquired'
         WHEN COALESCE((SELECT event_type FROM latest_event), '') = 'repair' THEN 'repairing'
         ELSE 'active'
       END::text AS status
FROM tenants t
JOIN assets a ON a.tenant_id = t.id AND a.id = sqlc.arg(asset_id)
LEFT JOIN effective_events e ON e.asset_id = a.id
WHERE t.id = sqlc.arg(tenant_id)
GROUP BY t.base_currency;
