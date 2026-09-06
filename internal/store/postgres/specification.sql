-- name: SpecificationTypes :many
SELECT CAST(id AS TEXT) AS id,name,normalized_name,multiple,affects_appearance,enabled,system_code,created_at,updated_at
FROM specification_tag_types WHERE tenant_id=sqlc.arg(tenant_id) ORDER BY normalized_name,id;
-- name: SpecificationTags :many
SELECT CAST(id AS TEXT) AS id,CAST(type_id AS TEXT) AS type_id,name,normalized_name,enabled,created_at,updated_at
FROM specification_tags WHERE tenant_id=sqlc.arg(tenant_id) ORDER BY type_id,normalized_name,id;
-- name: SpecificationLinks :many
SELECT CAST('model' AS TEXT) AS kind,CAST(mt.model_id AS TEXT) AS target_id,CAST(mt.model_id AS TEXT) AS model_id,CAST(mt.tag_id AS TEXT) AS tag_id FROM model_allowed_tags mt WHERE mt.tenant_id=sqlc.arg(tenant_id)
UNION ALL SELECT 'asset',CAST(at.asset_id AS TEXT),CAST(at.model_id AS TEXT),CAST(at.tag_id AS TEXT) FROM asset_specification_tags at WHERE at.tenant_id=sqlc.arg(tenant_id)
UNION ALL SELECT 'resource',CAST(rt.resource_id AS TEXT),'',CAST(rt.tag_id AS TEXT) FROM resource_specification_tags rt WHERE rt.tenant_id=sqlc.arg(tenant_id)
UNION ALL SELECT 'resource-category',CAST(rc.resource_id AS TEXT),'',CAST(rc.category_id AS TEXT) FROM resource_categories rc WHERE rc.tenant_id=sqlc.arg(tenant_id)
UNION ALL SELECT 'appearance',CAST(ac.rule_id AS TEXT),CAST(ac.model_id AS TEXT),CAST(ac.tag_id AS TEXT) FROM model_appearance_conditions ac WHERE ac.tenant_id=sqlc.arg(tenant_id)
UNION ALL SELECT 'legacy',CAST(l.variant_id AS TEXT),CAST(v.model_id AS TEXT),CAST(l.tag_id AS TEXT) FROM legacy_variant_tags l JOIN product_variants v ON v.tenant_id=l.tenant_id AND v.id=l.variant_id WHERE l.tenant_id=sqlc.arg(tenant_id);
-- name: SpecificationDimensions :many
SELECT CAST(model_id AS TEXT) AS model_id,CAST(type_id AS TEXT) AS type_id,affects_appearance FROM model_appearance_dimensions WHERE tenant_id=sqlc.arg(tenant_id);
-- name: SpecificationDefaults :many
SELECT CAST(id AS TEXT) AS id,CAST(model_id AS TEXT) AS model_id,CAST(resource_id AS TEXT) AS resource_id FROM model_appearance_defaults WHERE tenant_id=sqlc.arg(tenant_id) ORDER BY id;
-- name: SpecificationLegacyMedia :many
SELECT CAST(variant_id AS TEXT) AS variant_id,CAST(model_id AS TEXT) AS model_id,CAST(resource_id AS TEXT) AS resource_id,reason,resolved FROM legacy_variant_media WHERE tenant_id=sqlc.arg(tenant_id);
-- name: SpecificationCategories :many
SELECT CAST(id AS TEXT) AS id FROM item_categories WHERE tenant_id=sqlc.arg(tenant_id);
-- name: PutSpecificationType :exec
INSERT INTO specification_tag_types(id,tenant_id,name,normalized_name,multiple,affects_appearance,enabled,system_code,created_at,updated_at)
VALUES(sqlc.arg(id),sqlc.arg(tenant_id),sqlc.arg(name),sqlc.arg(normalized_name),sqlc.arg(multiple),sqlc.arg(affects_appearance),sqlc.arg(enabled),sqlc.arg(system_code),sqlc.arg(created_at),sqlc.arg(updated_at))
ON CONFLICT(tenant_id,id) DO UPDATE SET name=excluded.name,normalized_name=excluded.normalized_name,multiple=excluded.multiple,affects_appearance=excluded.affects_appearance,enabled=excluded.enabled,updated_at=excluded.updated_at;
-- name: PutSpecificationTag :exec
INSERT INTO specification_tags(id,tenant_id,type_id,name,normalized_name,enabled,created_at,updated_at)
VALUES(sqlc.arg(id),sqlc.arg(tenant_id),sqlc.arg(type_id),sqlc.arg(name),sqlc.arg(normalized_name),sqlc.arg(enabled),sqlc.arg(created_at),sqlc.arg(updated_at))
ON CONFLICT(tenant_id,id) DO UPDATE SET name=excluded.name,normalized_name=excluded.normalized_name,enabled=excluded.enabled,updated_at=excluded.updated_at;
-- name: AddModelAllowedTag :exec
INSERT INTO model_allowed_tags(tenant_id,model_id,tag_id) VALUES(sqlc.arg(tenant_id),sqlc.arg(model_id),sqlc.arg(tag_id)) ON CONFLICT DO NOTHING;
-- name: RemoveModelAllowedTag :exec
DELETE FROM model_allowed_tags WHERE tenant_id=sqlc.arg(tenant_id) AND model_id=sqlc.arg(model_id) AND tag_id=sqlc.arg(tag_id);
-- name: PutAppearanceDimension :exec
INSERT INTO model_appearance_dimensions(tenant_id,model_id,type_id,affects_appearance) VALUES(sqlc.arg(tenant_id),sqlc.arg(model_id),sqlc.arg(type_id),sqlc.arg(affects_appearance));
-- name: ClearAppearanceDimensions :exec
DELETE FROM model_appearance_dimensions WHERE tenant_id=sqlc.arg(tenant_id) AND model_id=sqlc.arg(model_id);
-- name: PutAppearanceDefault :exec
INSERT INTO model_appearance_defaults(id,tenant_id,model_id,resource_id,created_at,updated_at)
VALUES(sqlc.arg(id),sqlc.arg(tenant_id),sqlc.arg(model_id),sqlc.arg(resource_id),sqlc.arg(created_at),sqlc.arg(updated_at))
ON CONFLICT(tenant_id,id) DO UPDATE SET resource_id=excluded.resource_id,updated_at=excluded.updated_at;
-- name: DeleteAppearanceDefault :exec
DELETE FROM model_appearance_defaults WHERE tenant_id=sqlc.arg(tenant_id) AND id=sqlc.arg(id);
-- name: ClearAppearanceConditions :exec
DELETE FROM model_appearance_conditions WHERE tenant_id=sqlc.arg(tenant_id) AND rule_id=sqlc.arg(rule_id);
-- name: AddAppearanceCondition :exec
INSERT INTO model_appearance_conditions(tenant_id,rule_id,model_id,tag_id) VALUES(sqlc.arg(tenant_id),sqlc.arg(rule_id),sqlc.arg(model_id),sqlc.arg(tag_id));
-- name: ResolveLegacyMedia :execrows
UPDATE legacy_variant_media SET resolved=TRUE WHERE tenant_id=sqlc.arg(tenant_id) AND variant_id=sqlc.arg(variant_id);
-- name: ClearAssetSpecificationTags :exec
DELETE FROM asset_specification_tags WHERE tenant_id=sqlc.arg(tenant_id) AND asset_id=sqlc.arg(asset_id);
-- name: AddAssetSpecificationTag :exec
INSERT INTO asset_specification_tags(tenant_id,asset_id,model_id,tag_id) VALUES(sqlc.arg(tenant_id),sqlc.arg(asset_id),sqlc.arg(model_id),sqlc.arg(tag_id));
-- name: CreateSelectedAsset :exec
INSERT INTO assets(id,tenant_id,model_id,display_name,serial_number,purchase_channel,notes,created_at)
VALUES(sqlc.arg(id),sqlc.arg(tenant_id),sqlc.arg(model_id),sqlc.arg(display_name),sqlc.arg(serial_number),sqlc.arg(purchase_channel),sqlc.arg(notes),sqlc.arg(created_at));
-- name: UpdateSelectedAsset :execrows
UPDATE assets SET model_id=sqlc.arg(model_id),display_name=sqlc.arg(display_name),serial_number=sqlc.arg(serial_number),purchase_channel=sqlc.arg(purchase_channel),notes=sqlc.arg(notes)
WHERE tenant_id=sqlc.arg(tenant_id) AND id=sqlc.arg(id);
-- name: ClearResourceSpecificationTags :exec
DELETE FROM resource_specification_tags WHERE tenant_id=sqlc.arg(tenant_id) AND resource_id=sqlc.arg(resource_id);
-- name: AddResourceSpecificationTag :exec
INSERT INTO resource_specification_tags(tenant_id,resource_id,tag_id) VALUES(sqlc.arg(tenant_id),sqlc.arg(resource_id),sqlc.arg(tag_id));
-- name: ClearResourceCategories :exec
DELETE FROM resource_categories WHERE tenant_id=sqlc.arg(tenant_id) AND resource_id=sqlc.arg(resource_id);
-- name: AddResourceCategory :exec
INSERT INTO resource_categories(tenant_id,resource_id,category_id) VALUES(sqlc.arg(tenant_id),sqlc.arg(resource_id),sqlc.arg(category_id));
