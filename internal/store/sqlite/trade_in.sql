-- name: ListTradeInLinksForAsset :many
-- Anchored on the requested asset's own current pairing event, so a purged
-- counterpart still projects the surviving relation with its write-time snapshot
-- instead of being dropped by an inner join. The counterpart event id is only an
-- enrichment; the label always prefers the live counterpart asset name or model
-- and falls back to the snapshot taken when the relation was recorded.
SELECT e.trade_in_link_id AS link_id,
       e.trade_in_state AS link_state,
       CAST(CASE WHEN st.system_code = 'trade_in_source' THEN e.id ELSE COALESCE(c.id, '') END AS TEXT) AS source_event_id,
       CAST(CASE WHEN st.system_code = 'trade_in_destination' THEN e.id ELSE COALESCE(c.id, '') END AS TEXT) AS destination_event_id,
       CAST(CASE WHEN st.system_code = 'trade_in_source' THEN e.asset_id ELSE COALESCE(c.asset_id, e.related_asset_id, '') END AS TEXT) AS new_asset_id,
       CAST(CASE WHEN st.system_code = 'trade_in_destination' THEN e.asset_id ELSE COALESCE(c.asset_id, e.related_asset_id, '') END AS TEXT) AS old_asset_id,
       CAST(CASE WHEN st.system_code = 'trade_in_source'
            THEN COALESCE(NULLIF(a.display_name, ''), pm.name, '')
            ELSE COALESCE(NULLIF(ca.display_name, ''), cam.name, e.related_asset_name, '') END AS TEXT) AS new_asset_label,
       CAST(CASE WHEN st.system_code = 'trade_in_source'
            THEN COALESCE(pm.name, e.related_asset_spec, '')
            ELSE COALESCE(cam.name, e.related_asset_spec, '') END AS TEXT) AS new_asset_spec_label,
       CAST(CASE WHEN st.system_code = 'trade_in_destination'
            THEN COALESCE(NULLIF(a.display_name, ''), pm.name, '')
            ELSE COALESCE(NULLIF(ca.display_name, ''), cam.name, e.related_asset_name, '') END AS TEXT) AS old_asset_label,
       CAST(CASE WHEN st.system_code = 'trade_in_destination'
            THEN COALESCE(pm.name, e.related_asset_spec, '')
            ELSE COALESCE(cam.name, e.related_asset_spec, '') END AS TEXT) AS old_asset_spec_label,
       CASE WHEN st.system_code = 'trade_in_source' THEN 0 WHEN ca.id IS NULL THEN 1 ELSE 0 END AS new_asset_deleted,
       CASE WHEN st.system_code = 'trade_in_destination' THEN 0 WHEN ca.id IS NULL THEN 1 ELSE 0 END AS old_asset_deleted
FROM asset_events e
JOIN asset_event_types st ON st.tenant_id = e.tenant_id AND st.id = e.event_type_id
JOIN assets a ON a.tenant_id = e.tenant_id AND a.id = e.asset_id
LEFT JOIN product_models pm ON pm.tenant_id = a.tenant_id AND pm.id = a.model_id
LEFT JOIN asset_events c ON c.tenant_id = e.tenant_id AND c.trade_in_link_id = e.trade_in_link_id AND c.id <> e.id
    AND c.trade_in_state = e.trade_in_state
    AND NOT EXISTS (SELECT 1 FROM asset_events v WHERE v.tenant_id = c.tenant_id AND v.voids_event_id = c.id)
LEFT JOIN assets ca ON ca.tenant_id = e.tenant_id AND ca.id = e.related_asset_id
LEFT JOIN product_models cam ON cam.tenant_id = ca.tenant_id AND cam.id = ca.model_id
WHERE e.tenant_id = sqlc.arg(tenant_id)
  AND e.asset_id = sqlc.arg(asset_id)
  AND e.trade_in_link_id IS NOT NULL
  AND st.system_code IN ('trade_in_source', 'trade_in_destination')
  AND (CAST(sqlc.arg(include_cancelled) AS INTEGER) = 1 OR e.trade_in_state = 'active')
  AND NOT EXISTS (SELECT 1 FROM asset_events v WHERE v.tenant_id = e.tenant_id AND v.voids_event_id = e.id)
ORDER BY e.occurred_at, e.created_at, e.id;
