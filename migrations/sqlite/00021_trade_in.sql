-- +goose Up
-- The system-code check constraint can only be widened by rebuilding the type
-- table. SQLite cannot suspend foreign keys inside a transaction, so
-- internal/store/open.go turns them off outside this transaction for the schema
-- version that runs it, and the paired Go migration verifies foreign keys before
-- commit. Never run this file with NO TRANSACTION: a late failure must roll the
-- whole rebuild and version back.
ALTER TABLE asset_events ADD COLUMN related_asset_id TEXT;
ALTER TABLE asset_events ADD COLUMN related_asset_name TEXT NOT NULL DEFAULT '';
ALTER TABLE asset_events ADD COLUMN related_asset_spec TEXT NOT NULL DEFAULT '';
ALTER TABLE asset_events ADD COLUMN trade_in_link_id TEXT;
ALTER TABLE asset_events ADD COLUMN trade_in_state TEXT NOT NULL DEFAULT '' CHECK (trade_in_state IN ('', 'active', 'cancelled'));
-- Deliberately no foreign key on related_asset_id: purging either endpoint must
-- neither cascade nor be blocked, and the write-time snapshot keeps the label.
CREATE INDEX asset_events_trade_in_idx ON asset_events(tenant_id, trade_in_link_id) WHERE trade_in_link_id IS NOT NULL;
CREATE INDEX asset_events_related_asset_idx ON asset_events(tenant_id, related_asset_id) WHERE related_asset_id IS NOT NULL;

DROP TRIGGER IF EXISTS seed_lifecycle_types;

CREATE TABLE asset_event_types_next (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 80),
    normalized_name TEXT NOT NULL,
    cashflow_direction TEXT NOT NULL CHECK (cashflow_direction IN ('expense', 'income', 'neutral')),
    created_by_user_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    system_code TEXT NOT NULL DEFAULT '' CHECK (system_code IN ('', 'purchase', 'repair', 'sale', 'void', 'trade_in_source', 'trade_in_destination')),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    updated_at TEXT NOT NULL DEFAULT '',
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, normalized_name),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    FOREIGN KEY (tenant_id, created_by_user_id) REFERENCES tenant_memberships(tenant_id, user_id)
);

INSERT INTO asset_event_types_next
    (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, enabled, updated_at)
SELECT id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, enabled, updated_at
FROM asset_event_types;

DROP TABLE asset_event_types;
ALTER TABLE asset_event_types_next RENAME TO asset_event_types;

CREATE INDEX asset_event_types_tenant_name_idx ON asset_event_types(tenant_id, normalized_name);
CREATE UNIQUE INDEX asset_event_types_system_idx ON asset_event_types(tenant_id, system_code) WHERE system_code <> '';
