-- +goose NO TRANSACTION
-- +goose Up
PRAGMA foreign_keys = OFF;

DROP TRIGGER asset_events_no_update;
DROP TRIGGER asset_events_no_delete;
DROP INDEX asset_events_tenant_asset_time_idx;
DROP INDEX asset_events_void_once_idx;
ALTER TABLE asset_events RENAME TO asset_events_v6;

CREATE TABLE asset_events (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    asset_id TEXT NOT NULL,
    transaction_id TEXT NOT NULL,
    event_type TEXT NOT NULL CHECK (length(trim(event_type)) BETWEEN 1 AND 80),
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
        (event_type = 'void' AND base_amount_minor = 0 AND voids_event_id IS NOT NULL)
        OR (event_type <> 'void' AND voids_event_id IS NULL)
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

INSERT INTO asset_events
SELECT id, tenant_id, asset_id, transaction_id, event_type, base_amount_minor,
       base_currency, original_amount_minor, original_currency, fx_rate_scaled,
       fx_rate_date, fx_rate_source, notes, voids_event_id, replaces_event_id,
       occurred_at, created_by_user_id, created_at
FROM asset_events_v6;

DROP TABLE asset_events_v6;

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

CREATE TABLE asset_event_types (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 80),
    normalized_name TEXT NOT NULL,
    cashflow_direction TEXT NOT NULL CHECK (cashflow_direction IN ('expense', 'income', 'neutral')),
    created_by_user_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, normalized_name),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    FOREIGN KEY (tenant_id, created_by_user_id) REFERENCES tenant_memberships(tenant_id, user_id)
);

CREATE INDEX asset_event_types_tenant_name_idx
    ON asset_event_types(tenant_id, normalized_name);

PRAGMA foreign_keys = ON;
