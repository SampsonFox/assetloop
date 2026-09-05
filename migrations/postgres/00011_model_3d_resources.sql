-- +goose Up
CREATE TABLE model_3d_resources (
 id UUID PRIMARY KEY,
 tenant_id UUID NOT NULL REFERENCES tenants(id),
 name TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'ready' CHECK (status IN ('ready','pending-delete')),
 store_id TEXT NOT NULL,
 object_key TEXT NOT NULL,
 sha256 TEXT NOT NULL,
 size_bytes BIGINT NOT NULL CHECK(size_bytes > 0),
 source_url TEXT NOT NULL DEFAULT '',
 author TEXT NOT NULL DEFAULT '',
 license TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL,
 UNIQUE(tenant_id,id),
 UNIQUE(tenant_id,id,status),
 UNIQUE(store_id,object_key)
);
ALTER TABLE product_models ADD COLUMN model_3d_resource_id UUID;
ALTER TABLE assets ADD COLUMN model_3d_resource_id UUID;
ALTER TABLE product_variants ADD COLUMN color TEXT NOT NULL DEFAULT '';
ALTER TABLE product_variants ADD COLUMN model_3d_resource_id UUID;
ALTER TABLE product_variants DROP CONSTRAINT product_variants_tenant_id_model_id_name_key;
ALTER TABLE product_variants ADD UNIQUE(tenant_id,model_id,name,color);
-- Preserve the original variant as the unspecified-color choice and split existing colors.
INSERT INTO product_variants(id,tenant_id,model_id,name,created_at,color)
 SELECT md5(v.id::text || ':' || a.color)::uuid,
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
ALTER TABLE product_models ADD COLUMN model_3d_resource_status TEXT NOT NULL DEFAULT 'ready' CHECK(model_3d_resource_status='ready');
ALTER TABLE product_models ADD FOREIGN KEY(tenant_id,model_3d_resource_id,model_3d_resource_status) REFERENCES model_3d_resources(tenant_id,id,status);
ALTER TABLE product_variants ADD COLUMN model_3d_resource_status TEXT NOT NULL DEFAULT 'ready' CHECK(model_3d_resource_status='ready');
ALTER TABLE product_variants ADD FOREIGN KEY(tenant_id,model_3d_resource_id,model_3d_resource_status) REFERENCES model_3d_resources(tenant_id,id,status);
ALTER TABLE assets ADD COLUMN model_3d_resource_status TEXT NOT NULL DEFAULT 'ready' CHECK(model_3d_resource_status='ready');
ALTER TABLE assets ADD FOREIGN KEY(tenant_id,model_3d_resource_id,model_3d_resource_status) REFERENCES model_3d_resources(tenant_id,id,status);
-- +goose StatementBegin
CREATE FUNCTION protect_model_3d_bytes() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.tenant_id,NEW.id,NEW.store_id,NEW.object_key,NEW.sha256,NEW.size_bytes,NEW.created_at) IS DISTINCT FROM
 (OLD.tenant_id,OLD.id,OLD.store_id,OLD.object_key,OLD.sha256,OLD.size_bytes,OLD.created_at)
 OR (OLD.status='pending-delete' AND NEW.status <> OLD.status) THEN
 RAISE EXCEPTION 'immutable 3D resource';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER model_3d_immutable BEFORE UPDATE ON model_3d_resources FOR EACH ROW EXECUTE FUNCTION protect_model_3d_bytes();
