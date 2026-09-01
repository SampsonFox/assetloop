-- +goose Up
PRAGMA foreign_keys = ON;

CREATE TABLE tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    base_currency TEXT NOT NULL DEFAULT 'CNY',
    base_currency_locked INTEGER NOT NULL DEFAULT 0 CHECK (base_currency_locked IN (0, 1)),
    created_at TEXT NOT NULL
);

CREATE TABLE item_categories (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, name),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);

CREATE TABLE product_models (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    category_id TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, category_id, name),
    FOREIGN KEY (tenant_id, category_id) REFERENCES item_categories(tenant_id, id)
);

CREATE TABLE product_variants (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    model_id TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, model_id, name),
    FOREIGN KEY (tenant_id, model_id) REFERENCES product_models(tenant_id, id)
);

CREATE TABLE assets (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    variant_id TEXT NOT NULL,
    display_name TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, variant_id) REFERENCES product_variants(tenant_id, id)
);

