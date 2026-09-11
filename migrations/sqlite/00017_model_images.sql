-- +goose Up
CREATE TABLE model_images (
 id TEXT PRIMARY KEY,
 tenant_id TEXT NOT NULL,
 model_id TEXT NOT NULL,
 store_id TEXT NOT NULL,
 object_key TEXT NOT NULL,
 sha256 TEXT NOT NULL,
 content_type TEXT NOT NULL,
 size_bytes BIGINT NOT NULL CHECK(size_bytes > 0),
 source_url TEXT NOT NULL DEFAULT '',
 active INTEGER NOT NULL CHECK(active IN (0,1)),
 FOREIGN KEY(tenant_id,model_id) REFERENCES product_models(tenant_id,id)
);
CREATE UNIQUE INDEX model_image_active ON model_images(tenant_id,model_id) WHERE active=1;
