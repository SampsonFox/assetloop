-- +goose Up
ALTER TABLE product_models
    ADD COLUMN model_3d_store_id TEXT,
    ADD COLUMN model_3d_object_key TEXT,
    ADD COLUMN model_3d_sha256 TEXT,
    ADD COLUMN model_3d_size_bytes BIGINT,
    ADD COLUMN model_3d_source_url TEXT,
    ADD COLUMN model_3d_author TEXT,
    ADD COLUMN model_3d_license TEXT,
    ADD COLUMN model_3d_updated_at TIMESTAMPTZ;

-- +goose Down
SELECT 1;
