-- +goose Up
ALTER TABLE asset_event_types ADD COLUMN system_code TEXT NOT NULL DEFAULT '' CHECK (system_code IN ('', 'purchase', 'repair', 'sale', 'void'));
ALTER TABLE asset_event_types ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1));
ALTER TABLE asset_event_types ADD COLUMN updated_at TEXT NOT NULL DEFAULT '';
UPDATE asset_event_types SET updated_at = created_at;
CREATE UNIQUE INDEX asset_event_types_system_idx ON asset_event_types(tenant_id, system_code) WHERE system_code <> '';

INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, updated_at)
SELECT lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-8' || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))), m.tenant_id, 'purchase', 'purchase', 'expense', m.user_id, m.created_at, 'purchase', m.created_at
FROM tenant_memberships m WHERE m.user_id = (SELECT MIN(user_id) FROM tenant_memberships WHERE tenant_id = m.tenant_id);

INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, updated_at)
SELECT lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-8' || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))), m.tenant_id, 'repair', 'repair', 'expense', m.user_id, m.created_at, 'repair', m.created_at
FROM tenant_memberships m WHERE m.user_id = (SELECT MIN(user_id) FROM tenant_memberships WHERE tenant_id = m.tenant_id);

INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, updated_at)
SELECT lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-8' || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))), m.tenant_id, 'sale', 'sale', 'income', m.user_id, m.created_at, 'sale', m.created_at
FROM tenant_memberships m WHERE m.user_id = (SELECT MIN(user_id) FROM tenant_memberships WHERE tenant_id = m.tenant_id);

INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, updated_at)
SELECT lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-8' || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))), m.tenant_id, 'void', 'void', 'neutral', m.user_id, m.created_at, 'void', m.created_at
FROM tenant_memberships m WHERE m.user_id = (SELECT MIN(user_id) FROM tenant_memberships WHERE tenant_id = m.tenant_id);

-- +goose StatementBegin
CREATE TRIGGER seed_lifecycle_types AFTER INSERT ON tenant_memberships
WHEN NOT EXISTS (SELECT 1 FROM asset_event_types WHERE tenant_id = NEW.tenant_id AND system_code = 'purchase')
BEGIN
INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, updated_at) VALUES (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-8' || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))), NEW.tenant_id, 'purchase', 'purchase', 'expense', NEW.user_id, NEW.created_at, 'purchase', NEW.created_at);
INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, updated_at) VALUES (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-8' || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))), NEW.tenant_id, 'repair', 'repair', 'expense', NEW.user_id, NEW.created_at, 'repair', NEW.created_at);
INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, updated_at) VALUES (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-8' || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))), NEW.tenant_id, 'sale', 'sale', 'income', NEW.user_id, NEW.created_at, 'sale', NEW.created_at);
INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, updated_at) VALUES (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-8' || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))), NEW.tenant_id, 'void', 'void', 'neutral', NEW.user_id, NEW.created_at, 'void', NEW.created_at);
END;
-- +goose StatementEnd

CREATE TABLE asset_events_next (
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
    event_type_id TEXT NOT NULL,
    FOREIGN KEY (tenant_id, event_type_id) REFERENCES asset_event_types(tenant_id, id),
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

INSERT INTO asset_events_next (id, tenant_id, asset_id, transaction_id, event_type, base_amount_minor, base_currency, original_amount_minor, original_currency, fx_rate_scaled, fx_rate_date, fx_rate_source, notes, voids_event_id, replaces_event_id, occurred_at, created_by_user_id, created_at, event_type_id)
SELECT e.id, e.tenant_id, e.asset_id, e.transaction_id, e.event_type, e.base_amount_minor, e.base_currency, e.original_amount_minor, e.original_currency, e.fx_rate_scaled, e.fx_rate_date, e.fx_rate_source, e.notes, e.voids_event_id, e.replaces_event_id, e.occurred_at, e.created_by_user_id, e.created_at,
       (SELECT t.id FROM asset_event_types t WHERE t.tenant_id = e.tenant_id AND (t.name = trim(e.event_type) OR t.normalized_name = lower(trim(e.event_type))))
FROM asset_events e;
DROP TRIGGER asset_events_no_update;
DROP TRIGGER asset_events_no_delete;
DROP TABLE asset_events;
ALTER TABLE asset_events_next RENAME TO asset_events;
CREATE INDEX asset_events_tenant_asset_time_idx ON asset_events(tenant_id, asset_id, occurred_at, created_at);
CREATE UNIQUE INDEX asset_events_void_once_idx ON asset_events(tenant_id, voids_event_id) WHERE voids_event_id IS NOT NULL;
CREATE INDEX asset_events_type_idx ON asset_events(tenant_id, event_type_id);
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
