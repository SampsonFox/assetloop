-- +goose Up
CREATE TABLE market_items (
 id TEXT PRIMARY KEY, tenant_id UUID NOT NULL REFERENCES tenants(id),
 name TEXT NOT NULL, provider TEXT NOT NULL, keyword TEXT NOT NULL, filter_criteria TEXT NOT NULL,
 model_desc TEXT NOT NULL, region TEXT NOT NULL DEFAULT 'CN', external_id TEXT,
 enabled BIGINT NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)), created_at BIGINT NOT NULL,
 last_attempt BIGINT NOT NULL DEFAULT 0, last_success BIGINT NOT NULL DEFAULT 0,
 last_error TEXT NOT NULL DEFAULT '', lease_token TEXT NOT NULL DEFAULT '', lease_until BIGINT NOT NULL DEFAULT 0,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,provider,keyword,filter_criteria,region)
);
CREATE TABLE market_prices (
 tenant_id UUID NOT NULL REFERENCES tenants(id), market_item_id TEXT NOT NULL,
 observation_date TEXT NOT NULL, observed_at BIGINT NOT NULL,
 max_minor BIGINT NOT NULL CHECK(max_minor > 0), min_minor BIGINT CHECK(min_minor > 0 AND min_minor <= max_minor),
 currency TEXT NOT NULL, base_currency TEXT NOT NULL, base_minor BIGINT,
 rate_scaled BIGINT, rate_date TEXT, rate_source TEXT,
 provider TEXT NOT NULL, provider_version TEXT NOT NULL, provenance TEXT NOT NULL,
 evidence TEXT NOT NULL, source_date TEXT, sample_count BIGINT CHECK(sample_count >= 0),
 PRIMARY KEY(tenant_id,market_item_id,observation_date),
 FOREIGN KEY(tenant_id,market_item_id) REFERENCES market_items(tenant_id,id),
 CHECK((base_minor IS NULL AND rate_scaled IS NULL AND rate_date IS NULL AND rate_source IS NULL) OR
       (base_minor IS NOT NULL AND rate_scaled > 0 AND rate_date IS NOT NULL AND rate_source IS NOT NULL))
);
CREATE TABLE asset_market_bindings (
 tenant_id UUID NOT NULL REFERENCES tenants(id), asset_id UUID NOT NULL, market_item_id TEXT NOT NULL,
 PRIMARY KEY(tenant_id,asset_id),
 FOREIGN KEY(tenant_id,asset_id) REFERENCES assets(tenant_id,id),
 FOREIGN KEY(tenant_id,market_item_id) REFERENCES market_items(tenant_id,id)
);
CREATE INDEX market_bindings_item ON asset_market_bindings(tenant_id,market_item_id);
CREATE INDEX market_prices_pending_fx ON market_prices(tenant_id,observation_date) WHERE base_minor IS NULL;
