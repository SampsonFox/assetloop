-- +goose Up
-- Retain migrated tags, explicit item bindings, resource metadata and GLB keys.
DROP TRIGGER model_3d_legacy_references ON model_3d_resources;
DROP FUNCTION protect_legacy_variant_media();
DROP TABLE legacy_variant_tags;
DROP TABLE legacy_variant_media;
ALTER TABLE assets DROP COLUMN variant_id;
ALTER TABLE assets DROP COLUMN color;
DROP TABLE product_variants;
