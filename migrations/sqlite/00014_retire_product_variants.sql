-- +goose Up
-- Contract phase explicitly approved after specification tag migration 00013.
-- The Go migration verifies all foreign keys before committing the rebuild.
DROP TRIGGER model_3d_pending_references;
DROP TRIGGER model_3d_delete_references;
DROP TABLE legacy_variant_tags;
DROP TABLE legacy_variant_media;

CREATE TABLE assets_next (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, model_id TEXT NOT NULL,
 display_name TEXT NOT NULL, serial_number TEXT NOT NULL DEFAULT '',
 purchase_channel TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL,
 model_3d_resource_id TEXT,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,id,model_id),
 FOREIGN KEY(tenant_id,model_id) REFERENCES product_models(tenant_id,id),
 FOREIGN KEY(tenant_id,model_3d_resource_id) REFERENCES model_3d_resources(tenant_id,id)
);
INSERT INTO assets_next(id,tenant_id,model_id,display_name,serial_number,purchase_channel,notes,created_at,model_3d_resource_id)
 SELECT id,tenant_id,model_id,display_name,serial_number,purchase_channel,notes,created_at,model_3d_resource_id FROM assets;
DROP TABLE assets;
ALTER TABLE assets_next RENAME TO assets;
CREATE UNIQUE INDEX assets_tenant_serial_idx ON assets(tenant_id,serial_number) WHERE serial_number<>'';
CREATE INDEX assets_model_idx ON assets(tenant_id,model_id);
DROP TABLE product_variants;

-- +goose StatementBegin
CREATE TRIGGER assets_resource_insert BEFORE INSERT ON assets
WHEN NEW.model_3d_resource_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM model_3d_resources WHERE tenant_id=NEW.tenant_id AND id=NEW.model_3d_resource_id AND status='ready')
BEGIN SELECT RAISE(ABORT,'3D resource unavailable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER assets_resource_update BEFORE UPDATE ON assets
WHEN NEW.model_3d_resource_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM model_3d_resources WHERE tenant_id=NEW.tenant_id AND id=NEW.model_3d_resource_id AND status='ready')
BEGIN SELECT RAISE(ABORT,'3D resource unavailable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER model_3d_pending_references BEFORE UPDATE OF status ON model_3d_resources
WHEN NEW.status='pending-delete' AND (EXISTS(SELECT 1 FROM product_models WHERE tenant_id=OLD.tenant_id AND model_3d_resource_id=OLD.id) OR EXISTS(SELECT 1 FROM assets WHERE tenant_id=OLD.tenant_id AND model_3d_resource_id=OLD.id) OR EXISTS(SELECT 1 FROM model_appearance_defaults WHERE tenant_id=OLD.tenant_id AND resource_id=OLD.id))
BEGIN SELECT RAISE(ABORT,'3D resource referenced'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER model_3d_delete_references BEFORE DELETE ON model_3d_resources
WHEN EXISTS(SELECT 1 FROM product_models WHERE tenant_id=OLD.tenant_id AND model_3d_resource_id=OLD.id) OR EXISTS(SELECT 1 FROM assets WHERE tenant_id=OLD.tenant_id AND model_3d_resource_id=OLD.id) OR EXISTS(SELECT 1 FROM model_appearance_defaults WHERE tenant_id=OLD.tenant_id AND resource_id=OLD.id)
BEGIN SELECT RAISE(ABORT,'3D resource referenced'); END;
-- +goose StatementEnd
