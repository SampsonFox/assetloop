-- +goose Up
-- Executed with the shared Go data backfill inside one Goose transaction.

ALTER TABLE assets ADD COLUMN model_id UUID;
UPDATE assets a SET model_id=v.model_id FROM product_variants v WHERE v.tenant_id=a.tenant_id AND v.id=a.variant_id;
ALTER TABLE assets ALTER COLUMN model_id SET NOT NULL;
ALTER TABLE assets ALTER COLUMN variant_id DROP NOT NULL;
ALTER TABLE assets ADD UNIQUE(tenant_id,id,model_id);
ALTER TABLE assets ADD FOREIGN KEY(tenant_id,model_id) REFERENCES product_models(tenant_id,id);
CREATE INDEX assets_model_idx ON assets(tenant_id,model_id);

CREATE TABLE specification_tag_types (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES tenants(id),
 name TEXT NOT NULL, normalized_name TEXT NOT NULL,
 multiple BOOLEAN NOT NULL DEFAULT FALSE, affects_appearance BOOLEAN NOT NULL DEFAULT FALSE,
 enabled BOOLEAN NOT NULL DEFAULT TRUE, system_code TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,normalized_name)
);
CREATE UNIQUE INDEX specification_type_system_idx ON specification_tag_types(tenant_id,system_code) WHERE system_code<>'';
CREATE TABLE specification_tags (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL, type_id UUID NOT NULL,
 name TEXT NOT NULL, normalized_name TEXT NOT NULL, enabled BOOLEAN NOT NULL DEFAULT TRUE,
 created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,id,type_id), UNIQUE(tenant_id,type_id,normalized_name),
 FOREIGN KEY(tenant_id,type_id) REFERENCES specification_tag_types(tenant_id,id)
);
CREATE TABLE model_allowed_tags (
 tenant_id UUID NOT NULL, model_id UUID NOT NULL, tag_id UUID NOT NULL,
 PRIMARY KEY(tenant_id,model_id,tag_id),
 FOREIGN KEY(tenant_id,model_id) REFERENCES product_models(tenant_id,id),
 FOREIGN KEY(tenant_id,tag_id) REFERENCES specification_tags(tenant_id,id)
);
CREATE TABLE model_appearance_dimensions (
 tenant_id UUID NOT NULL, model_id UUID NOT NULL, type_id UUID NOT NULL,
 affects_appearance BOOLEAN NOT NULL,
 PRIMARY KEY(tenant_id,model_id,type_id),
 FOREIGN KEY(tenant_id,model_id) REFERENCES product_models(tenant_id,id),
 FOREIGN KEY(tenant_id,type_id) REFERENCES specification_tag_types(tenant_id,id)
);
CREATE TABLE asset_specification_tags (
 tenant_id UUID NOT NULL, asset_id UUID NOT NULL, model_id UUID NOT NULL, tag_id UUID NOT NULL,
 PRIMARY KEY(tenant_id,asset_id,tag_id),
 FOREIGN KEY(tenant_id,asset_id,model_id) REFERENCES assets(tenant_id,id,model_id),
 FOREIGN KEY(tenant_id,model_id,tag_id) REFERENCES model_allowed_tags(tenant_id,model_id,tag_id)
);
CREATE TABLE resource_specification_tags (
 tenant_id UUID NOT NULL, resource_id UUID NOT NULL, tag_id UUID NOT NULL,
 PRIMARY KEY(tenant_id,resource_id,tag_id),
 FOREIGN KEY(tenant_id,resource_id) REFERENCES model_3d_resources(tenant_id,id) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,tag_id) REFERENCES specification_tags(tenant_id,id)
);
CREATE TABLE resource_categories (
 tenant_id UUID NOT NULL, resource_id UUID NOT NULL, category_id UUID NOT NULL,
 PRIMARY KEY(tenant_id,resource_id,category_id),
 FOREIGN KEY(tenant_id,resource_id) REFERENCES model_3d_resources(tenant_id,id) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,category_id) REFERENCES item_categories(tenant_id,id)
);
CREATE TABLE model_appearance_defaults (
 id UUID PRIMARY KEY, tenant_id UUID NOT NULL, model_id UUID NOT NULL, resource_id UUID NOT NULL,
 created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,id,model_id),
 FOREIGN KEY(tenant_id,model_id) REFERENCES product_models(tenant_id,id),
 FOREIGN KEY(tenant_id,resource_id) REFERENCES model_3d_resources(tenant_id,id)
);
CREATE TABLE model_appearance_conditions (
 tenant_id UUID NOT NULL, rule_id UUID NOT NULL, model_id UUID NOT NULL, tag_id UUID NOT NULL,
 PRIMARY KEY(tenant_id,rule_id,tag_id),
 FOREIGN KEY(tenant_id,rule_id,model_id) REFERENCES model_appearance_defaults(tenant_id,id,model_id) ON DELETE CASCADE,
 FOREIGN KEY(tenant_id,model_id,tag_id) REFERENCES model_allowed_tags(tenant_id,model_id,tag_id)
);
CREATE TABLE legacy_variant_tags (
 tenant_id UUID NOT NULL, variant_id UUID NOT NULL, tag_id UUID NOT NULL,
 PRIMARY KEY(tenant_id,variant_id,tag_id),
 FOREIGN KEY(tenant_id,variant_id) REFERENCES product_variants(tenant_id,id),
 FOREIGN KEY(tenant_id,tag_id) REFERENCES specification_tags(tenant_id,id)
);
CREATE TABLE legacy_variant_media (
 tenant_id UUID NOT NULL, variant_id UUID NOT NULL, model_id UUID NOT NULL, resource_id UUID NOT NULL,
 reason TEXT NOT NULL, resolved BOOLEAN NOT NULL DEFAULT FALSE,
 PRIMARY KEY(tenant_id,variant_id),
 FOREIGN KEY(tenant_id,variant_id) REFERENCES product_variants(tenant_id,id),
 FOREIGN KEY(tenant_id,model_id) REFERENCES product_models(tenant_id,id)
);
CREATE INDEX specification_tags_type_idx ON specification_tags(tenant_id,type_id,normalized_name);
CREATE INDEX asset_specification_tags_tag_idx ON asset_specification_tags(tenant_id,tag_id,asset_id);
CREATE INDEX resource_specification_tags_tag_idx ON resource_specification_tags(tenant_id,tag_id,resource_id);
CREATE INDEX model_appearance_defaults_model_idx ON model_appearance_defaults(tenant_id,model_id);

ALTER TABLE model_appearance_defaults ADD COLUMN resource_status TEXT NOT NULL DEFAULT 'ready' CHECK(resource_status='ready');
ALTER TABLE model_appearance_defaults ADD FOREIGN KEY(tenant_id,resource_id,resource_status) REFERENCES model_3d_resources(tenant_id,id,status);
-- Legacy pointers are evidence, not live bindings. Unresolved mappings are guarded below.
-- +goose StatementBegin
DO $$ DECLARE constraint_name TEXT; BEGIN
 FOR constraint_name IN SELECT conname FROM pg_constraint WHERE conrelid='product_variants'::regclass AND confrelid='model_3d_resources'::regclass LOOP
 EXECUTE format('ALTER TABLE product_variants DROP CONSTRAINT %I',constraint_name);
 END LOOP;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION protect_legacy_variant_media() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (TG_OP='DELETE' OR NEW.status='pending-delete') AND EXISTS(SELECT 1 FROM legacy_variant_media WHERE tenant_id=OLD.tenant_id AND resource_id=OLD.id AND NOT resolved) THEN
 RAISE EXCEPTION '3D resource referenced by unresolved migration';
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER model_3d_legacy_references BEFORE UPDATE OR DELETE ON model_3d_resources FOR EACH ROW EXECUTE FUNCTION protect_legacy_variant_media();
