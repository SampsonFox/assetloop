-- +goose Up
CREATE TABLE asset_transactions (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    occurred_at TEXT NOT NULL,
    source TEXT NOT NULL,
    external_reference TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    created_by_user_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    FOREIGN KEY (tenant_id, created_by_user_id) REFERENCES tenant_memberships(tenant_id, user_id)
);

CREATE TABLE asset_events (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    asset_id TEXT NOT NULL,
    transaction_id TEXT NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('purchase', 'repair', 'sale', 'void')),
    base_amount_minor INTEGER NOT NULL,
    base_currency TEXT NOT NULL CHECK (length(base_currency) = 3),
    original_amount_minor INTEGER,
    original_currency TEXT,
    fx_rate_scaled INTEGER,
    fx_rate_date TEXT,
    fx_rate_source TEXT,
    notes TEXT NOT NULL DEFAULT '',
    voids_event_id TEXT,
    replaces_event_id TEXT,
    occurred_at TEXT NOT NULL,
    created_by_user_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    CHECK (
        (event_type IN ('purchase', 'repair') AND base_amount_minor < 0 AND voids_event_id IS NULL)
        OR (event_type = 'sale' AND base_amount_minor > 0 AND voids_event_id IS NULL)
        OR (event_type = 'void' AND base_amount_minor = 0 AND voids_event_id IS NOT NULL)
    ),
    CHECK (
        (original_amount_minor IS NULL AND original_currency IS NULL AND fx_rate_scaled IS NULL AND fx_rate_date IS NULL AND fx_rate_source IS NULL)
        OR (event_type <> 'void' AND original_amount_minor > 0 AND length(original_currency) = 3 AND fx_rate_scaled > 0 AND fx_rate_date IS NOT NULL AND length(trim(fx_rate_source)) > 0)
    ),
    FOREIGN KEY (tenant_id, asset_id) REFERENCES assets(tenant_id, id),
    FOREIGN KEY (tenant_id, transaction_id) REFERENCES asset_transactions(tenant_id, id),
    FOREIGN KEY (tenant_id, created_by_user_id) REFERENCES tenant_memberships(tenant_id, user_id),
    FOREIGN KEY (tenant_id, voids_event_id) REFERENCES asset_events(tenant_id, id),
    FOREIGN KEY (tenant_id, replaces_event_id) REFERENCES asset_events(tenant_id, id)
);

CREATE INDEX asset_events_tenant_asset_time_idx
    ON asset_events(tenant_id, asset_id, occurred_at, created_at);
CREATE UNIQUE INDEX asset_events_void_once_idx
    ON asset_events(tenant_id, voids_event_id)
    WHERE voids_event_id IS NOT NULL;

-- +goose StatementBegin
CREATE TRIGGER asset_events_no_update
BEFORE UPDATE ON asset_events
BEGIN
    SELECT RAISE(ABORT, 'asset events are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER asset_events_no_delete
BEFORE DELETE ON asset_events
BEGIN
    SELECT RAISE(ABORT, 'asset events are append-only');
END;
-- +goose StatementEnd

CREATE TABLE import_drafts (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    asset_id TEXT NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('purchase', 'repair', 'sale')),
    amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
    currency TEXT NOT NULL CHECK (length(currency) = 3),
    occurred_at TEXT NOT NULL,
    source TEXT NOT NULL,
    external_reference TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    raw_text TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('pending', 'confirmed', 'rejected')),
    created_by_user_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    confirmed_transaction_id TEXT,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, asset_id) REFERENCES assets(tenant_id, id),
    FOREIGN KEY (tenant_id, created_by_user_id) REFERENCES tenant_memberships(tenant_id, user_id),
    FOREIGN KEY (tenant_id, confirmed_transaction_id) REFERENCES asset_transactions(tenant_id, id)
);

CREATE INDEX import_drafts_tenant_status_time_idx
    ON import_drafts(tenant_id, status, created_at);

-- +goose StatementBegin
CREATE TRIGGER tenants_base_currency_locked
BEFORE UPDATE OF base_currency ON tenants
WHEN OLD.base_currency_locked = 1 AND NEW.base_currency <> OLD.base_currency
BEGIN
    SELECT RAISE(ABORT, 'base currency is locked after the first monetary record');
END;
-- +goose StatementEnd
