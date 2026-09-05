-- +goose Up
ALTER TABLE product_models ADD COLUMN model_3d_store_id TEXT;
ALTER TABLE product_models ADD COLUMN model_3d_object_key TEXT;
ALTER TABLE product_models ADD COLUMN model_3d_sha256 TEXT;
ALTER TABLE product_models ADD COLUMN model_3d_size_bytes INTEGER;
ALTER TABLE product_models ADD COLUMN model_3d_source_url TEXT;
ALTER TABLE product_models ADD COLUMN model_3d_author TEXT;
ALTER TABLE product_models ADD COLUMN model_3d_license TEXT;
ALTER TABLE product_models ADD COLUMN model_3d_updated_at TEXT;

-- +goose Down
SELECT 1;
