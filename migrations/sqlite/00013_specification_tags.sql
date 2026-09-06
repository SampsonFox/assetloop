-- +goose Up
-- Executed with the shared Go data backfill inside one Goose transaction.

DROP TRIGGER model_3d_pending_references;
DROP TRIGGER model_3d_delete_references;
CREATE TABLE assets_next (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, model_id TEXT NOT NULL, variant_id TEXT,
 display_name TEXT NOT NULL, serial_number TEXT NOT NULL DEFAULT '', color TEXT NOT NULL DEFAULT '',
 purchase_channel TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL,
 model_3d_resource_id TEXT,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,id,model_id),
 FOREIGN KEY(tenant_id,model_id) REFERENCES product_models(tenant_id,id),
 FOREIGN KEY(tenant_id,variant_id) REFERENCES product_variants(tenant_id,id),
 FOREIGN KEY(tenant_id,model_3d_resource_id) REFERENCES model_3d_resources(tenant_id,id)
);
INSERT INTO assets_next SELECT a.id,a.tenant_id,v.model_id,a.variant_id,a.display_name,a.serial_number,a.color,a.purchase_channel,a.notes,a.created_at,a.model_3d_resource_id
 FROM assets a JOIN product_variants v ON v.tenant_id=a.tenant_id AND v.id=a.variant_id;
DROP TABLE assets;
ALTER TABLE assets_next RENAME TO assets;
CREATE UNIQUE INDEX assets_tenant_serial_idx ON assets(tenant_id,serial_number) WHERE serial_number<>'';
CREATE INDEX assets_model_idx ON assets(tenant_id,model_id);

CREATE TABLE specification_tag_types (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(id),
 name TEXT NOT NULL, normalized_name TEXT NOT NULL,
 multiple INTEGER NOT NULL DEFAULT 0, affects_appearance INTEGER NOT NULL DEFAULT 0,
 enabled INTEGER NOT NULL DEFAULT 1, system_code TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,normalized_name)
);
CREATE UNIQUE INDEX specification_type_system_idx ON specification_tag_types(tenant_id,system_code) WHERE system_code<>'';
CREATE TABLE specification_tags (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, type_id TEXT NOT NULL,
 name TEXT NOT NULL, normalized_name TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 1,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,id,type_id), UNIQUE(tenant_id,type_id,normalized_name),
 FOREIGN KEY(tenant_id,type_id) REFERENCES specification_tag_types(tenant_id,id)
);
CREATE TABLE model_allowed_tags (
 tenant_id TEXT NOT NULL, model_id TEXT NOT NULL, tag_id TEXT NOT NULL,
 PRIMARY KEY(tenant_id,model_id,tag_id),
 FOREIGN KEY(tenant_id,model_id) REFERENCES product_models(tenant_id,id),
 FOREIGN KEY(tenant_id,tag_id) REFERENCES specification_tags(tenant_id,id)
);
CREATE TABLE model_appearance_dimensions (
 tenant_id TEXT NOT NULL, model_id TEXT NOT NULL, type_id TEXT NOT NULL,
 affects_appearance INTEGER NOT NULL,
 PRIMARY KEY(tenant_id,model_id,type_id),
 FOREIGN KEY(tenant_id,model_id) REFERENCES product_models(tenant_id,id),
 FOREIGN KEY(tenant_id,type_id) REFERENCES specification_tag_types(tenant_id,id)
);
CREATE TABLE asset_specification_tags (
 tenant_id TEXT NOT NULL, asset_id TEXT NOT NULL, model_id TEXT NOT NULL, tag_id TEXT NOT NULL,
 PRIMARY KEY(tenant_id,asset_id,tag_id),
 FOREIGN KEY(tenant_id,asset_id,model_id) REFERENCES assets(tenant_id,id,model_id),
 FOREIGN KEY(tenant_id,model_id,tag_id) REFERENCES model_allowed_tags(tenant_id,model_id,tag_id)
);
CREATE TABLE resource_specification_tags (
 tenant_id TEXT NOT NULL, resource_id TEXT NOT NULL, tag_id TEXT NOT NULL,
 PRIMARY KEY(tenant_id,resource_id,tag_id),
 FOREIGN KEY(tenant_id,resource_id) REFERENCES model_3d_resources(tenant_id,id) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,tag_id) REFERENCES specification_tags(tenant_id,id)
);
CREATE TABLE resource_categories (
 tenant_id TEXT NOT NULL, resource_id TEXT NOT NULL, category_id TEXT NOT NULL,
 PRIMARY KEY(tenant_id,resource_id,category_id),
 FOREIGN KEY(tenant_id,resource_id) REFERENCES model_3d_resources(tenant_id,id) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,category_id) REFERENCES item_categories(tenant_id,id)
);
CREATE TABLE model_appearance_defaults (
 id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, model_id TEXT NOT NULL, resource_id TEXT NOT NULL,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,id,model_id),
 FOREIGN KEY(tenant_id,model_id) REFERENCES product_models(tenant_id,id),
 FOREIGN KEY(tenant_id,resource_id) REFERENCES model_3d_resources(tenant_id,id)
);
CREATE TABLE model_appearance_conditions (
 tenant_id TEXT NOT NULL, rule_id TEXT NOT NULL, model_id TEXT NOT NULL, tag_id TEXT NOT NULL,
 PRIMARY KEY(tenant_id,rule_id,tag_id),
 FOREIGN KEY(tenant_id,rule_id,model_id) REFERENCES model_appearance_defaults(tenant_id,id,model_id) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,model_id,tag_id) REFERENCES model_allowed_tags(tenant_id,model_id,tag_id)
);
CREATE TABLE legacy_variant_tags (
 tenant_id TEXT NOT NULL, variant_id TEXT NOT NULL, tag_id TEXT NOT NULL,
 PRIMARY KEY(tenant_id,variant_id,tag_id),
 FOREIGN KEY(tenant_id,variant_id) REFERENCES product_variants(tenant_id,id),
 FOREIGN KEY(tenant_id,tag_id) REFERENCES specification_tags(tenant_id,id)
);
CREATE TABLE legacy_variant_media (
 tenant_id TEXT NOT NULL, variant_id TEXT NOT NULL, model_id TEXT NOT NULL, resource_id TEXT NOT NULL,
 reason TEXT NOT NULL, resolved INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(tenant_id,variant_id),
 FOREIGN KEY(tenant_id,variant_id) REFERENCES product_variants(tenant_id,id),
 FOREIGN KEY(tenant_id,model_id) REFERENCES product_models(tenant_id,id)
);
CREATE INDEX specification_tags_type_idx ON specification_tags(tenant_id,type_id,normalized_name);
CREATE INDEX asset_specification_tags_tag_idx ON asset_specification_tags(tenant_id,tag_id,asset_id);
CREATE INDEX resource_specification_tags_tag_idx ON resource_specification_tags(tenant_id,tag_id,resource_id);
CREATE INDEX model_appearance_defaults_model_idx ON model_appearance_defaults(tenant_id,model_id);

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
CREATE TRIGGER model_appearance_defaults_resource_insert BEFORE INSERT ON model_appearance_defaults
WHEN NEW.resource_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM model_3d_resources WHERE tenant_id=NEW.tenant_id AND id=NEW.resource_id AND status='ready')
BEGIN SELECT RAISE(ABORT,'3D resource unavailable'); END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER model_appearance_defaults_resource_update BEFORE UPDATE ON model_appearance_defaults
WHEN NEW.resource_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM model_3d_resources WHERE tenant_id=NEW.tenant_id AND id=NEW.resource_id AND status='ready')
BEGIN SELECT RAISE(ABORT,'3D resource unavailable'); END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER model_3d_pending_references BEFORE UPDATE OF status ON model_3d_resources
WHEN NEW.status='pending-delete' AND (EXISTS(SELECT 1 FROM product_models WHERE tenant_id=OLD.tenant_id AND model_3d_resource_id=OLD.id) OR EXISTS(SELECT 1 FROM assets WHERE tenant_id=OLD.tenant_id AND model_3d_resource_id=OLD.id) OR EXISTS(SELECT 1 FROM model_appearance_defaults WHERE tenant_id=OLD.tenant_id AND resource_id=OLD.id) OR EXISTS(SELECT 1 FROM legacy_variant_media WHERE tenant_id=OLD.tenant_id AND resource_id=OLD.id AND resolved=0))
BEGIN SELECT RAISE(ABORT,'3D resource referenced'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER model_3d_delete_references BEFORE DELETE ON model_3d_resources
WHEN EXISTS(SELECT 1 FROM product_models WHERE tenant_id=OLD.tenant_id AND model_3d_resource_id=OLD.id) OR EXISTS(SELECT 1 FROM assets WHERE tenant_id=OLD.tenant_id AND model_3d_resource_id=OLD.id) OR EXISTS(SELECT 1 FROM model_appearance_defaults WHERE tenant_id=OLD.tenant_id AND resource_id=OLD.id) OR EXISTS(SELECT 1 FROM legacy_variant_media WHERE tenant_id=OLD.tenant_id AND resource_id=OLD.id AND resolved=0)
BEGIN SELECT RAISE(ABORT,'3D resource referenced'); END;
-- +goose StatementEnd
