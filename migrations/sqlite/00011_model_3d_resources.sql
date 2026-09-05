-- +goose Up
CREATE TABLE model_3d_resources (
 id TEXT PRIMARY KEY,
 tenant_id TEXT NOT NULL REFERENCES tenants(id),
 name TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'ready' CHECK (status IN ('ready','pending-delete')),
 store_id TEXT NOT NULL,
 object_key TEXT NOT NULL,
 sha256 TEXT NOT NULL,
 size_bytes BIGINT NOT NULL CHECK(size_bytes > 0),
 source_url TEXT NOT NULL DEFAULT '',
 author TEXT NOT NULL DEFAULT '',
 license TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL,
 UNIQUE(tenant_id,id),
 UNIQUE(tenant_id,id,status),
 UNIQUE(store_id,object_key)
);
ALTER TABLE product_models ADD COLUMN model_3d_resource_id TEXT;
ALTER TABLE assets ADD COLUMN model_3d_resource_id TEXT;
CREATE TABLE product_variants_new (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, model_id TEXT NOT NULL,
 name TEXT NOT NULL, created_at TEXT NOT NULL, color TEXT NOT NULL DEFAULT '',
 model_3d_resource_id TEXT,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,model_id,name,color),
 FOREIGN KEY(tenant_id,model_id) REFERENCES product_models(tenant_id,id)
);
INSERT INTO product_variants_new(id,tenant_id,model_id,name,created_at)
 SELECT id,tenant_id,model_id,name,created_at FROM product_variants;
DROP TABLE product_variants;
ALTER TABLE product_variants_new RENAME TO product_variants;
-- Preserve the original variant as the unspecified-color choice and split existing colors.
INSERT INTO product_variants(id,tenant_id,model_id,name,created_at,color)
 SELECT lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-a' || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))),
 v.tenant_id,v.model_id,v.name,v.created_at,a.color
 FROM product_variants v JOIN (SELECT DISTINCT tenant_id,variant_id,trim(color) AS color FROM assets WHERE trim(color) <> '') a
 ON a.tenant_id=v.tenant_id AND a.variant_id=v.id;
UPDATE assets SET variant_id = (
 SELECT v.id FROM product_variants v JOIN product_variants old
 ON v.tenant_id=old.tenant_id AND v.model_id=old.model_id AND v.name=old.name
 WHERE old.tenant_id=assets.tenant_id AND old.id=assets.variant_id AND v.color=trim(assets.color)
) WHERE trim(color) <> '';
-- Keep legacy columns intact for recovery, but reads now use the resource relation.
INSERT INTO model_3d_resources(id,tenant_id,name,store_id,object_key,sha256,size_bytes,source_url,author,license,created_at,updated_at)
 SELECT id,tenant_id,name,model_3d_store_id,model_3d_object_key,model_3d_sha256,model_3d_size_bytes,
 COALESCE(model_3d_source_url,''),COALESCE(model_3d_author,''),COALESCE(model_3d_license,''),
 COALESCE(model_3d_updated_at,created_at),COALESCE(model_3d_updated_at,created_at)
 FROM product_models WHERE model_3d_store_id IS NOT NULL AND model_3d_object_key IS NOT NULL
 AND model_3d_sha256 IS NOT NULL AND model_3d_size_bytes > 0;
UPDATE product_models SET model_3d_resource_id=id WHERE id IN (SELECT id FROM model_3d_resources);
-- +goose StatementBegin
CREATE TRIGGER product_models_resource_insert BEFORE INSERT ON product_models
WHEN NEW.model_3d_resource_id IS NOT NULL AND NOT EXISTS (
 SELECT 1 FROM model_3d_resources r WHERE r.tenant_id=NEW.tenant_id AND r.id=NEW.model_3d_resource_id AND r.status='ready')
BEGIN SELECT RAISE(ABORT,'3D resource unavailable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER product_models_resource_update BEFORE UPDATE ON product_models
WHEN NEW.model_3d_resource_id IS NOT NULL AND NOT EXISTS (
 SELECT 1 FROM model_3d_resources r WHERE r.tenant_id=NEW.tenant_id AND r.id=NEW.model_3d_resource_id AND r.status='ready')
BEGIN SELECT RAISE(ABORT,'3D resource unavailable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER product_variants_resource_insert BEFORE INSERT ON product_variants
WHEN NEW.model_3d_resource_id IS NOT NULL AND NOT EXISTS (
 SELECT 1 FROM model_3d_resources r WHERE r.tenant_id=NEW.tenant_id AND r.id=NEW.model_3d_resource_id AND r.status='ready')
BEGIN SELECT RAISE(ABORT,'3D resource unavailable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER product_variants_resource_update BEFORE UPDATE ON product_variants
WHEN NEW.model_3d_resource_id IS NOT NULL AND NOT EXISTS (
 SELECT 1 FROM model_3d_resources r WHERE r.tenant_id=NEW.tenant_id AND r.id=NEW.model_3d_resource_id AND r.status='ready')
BEGIN SELECT RAISE(ABORT,'3D resource unavailable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER assets_resource_insert BEFORE INSERT ON assets
WHEN NEW.model_3d_resource_id IS NOT NULL AND NOT EXISTS (
 SELECT 1 FROM model_3d_resources r WHERE r.tenant_id=NEW.tenant_id AND r.id=NEW.model_3d_resource_id AND r.status='ready')
BEGIN SELECT RAISE(ABORT,'3D resource unavailable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER assets_resource_update BEFORE UPDATE ON assets
WHEN NEW.model_3d_resource_id IS NOT NULL AND NOT EXISTS (
 SELECT 1 FROM model_3d_resources r WHERE r.tenant_id=NEW.tenant_id AND r.id=NEW.model_3d_resource_id AND r.status='ready')
BEGIN SELECT RAISE(ABORT,'3D resource unavailable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER model_3d_pending_references BEFORE UPDATE OF status ON model_3d_resources
WHEN NEW.status='pending-delete' AND (EXISTS(SELECT 1 FROM product_models WHERE tenant_id=OLD.tenant_id AND model_3d_resource_id=OLD.id) OR EXISTS(SELECT 1 FROM product_variants WHERE tenant_id=OLD.tenant_id AND model_3d_resource_id=OLD.id) OR EXISTS(SELECT 1 FROM assets WHERE tenant_id=OLD.tenant_id AND model_3d_resource_id=OLD.id))
BEGIN SELECT RAISE(ABORT,'3D resource referenced'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER model_3d_delete_references BEFORE DELETE ON model_3d_resources
WHEN EXISTS(SELECT 1 FROM product_models WHERE tenant_id=OLD.tenant_id AND model_3d_resource_id=OLD.id) OR EXISTS(SELECT 1 FROM product_variants WHERE tenant_id=OLD.tenant_id AND model_3d_resource_id=OLD.id) OR EXISTS(SELECT 1 FROM assets WHERE tenant_id=OLD.tenant_id AND model_3d_resource_id=OLD.id)
BEGIN SELECT RAISE(ABORT,'3D resource referenced'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER model_3d_immutable BEFORE UPDATE ON model_3d_resources
WHEN NEW.tenant_id<>OLD.tenant_id OR NEW.id<>OLD.id OR NEW.store_id<>OLD.store_id OR NEW.object_key<>OLD.object_key
 OR NEW.sha256<>OLD.sha256 OR NEW.size_bytes<>OLD.size_bytes OR NEW.created_at<>OLD.created_at
 OR (OLD.status='pending-delete' AND NEW.status<>OLD.status)
BEGIN SELECT RAISE(ABORT,'immutable 3D resource'); END;
-- +goose StatementEnd
-- Abort before Goose commits the schema version if any rebuilt relationship is invalid.
CREATE TEMP TABLE migration11_fk_check (violations INTEGER CHECK(violations=0));
INSERT INTO migration11_fk_check SELECT COUNT(*) FROM pragma_foreign_key_check;
DROP TABLE migration11_fk_check;
