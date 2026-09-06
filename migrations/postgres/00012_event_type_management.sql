-- +goose Up
ALTER TABLE asset_event_types ADD COLUMN system_code TEXT NOT NULL DEFAULT '' CHECK (system_code IN ('', 'purchase', 'repair', 'sale', 'void'));
ALTER TABLE asset_event_types ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE asset_event_types ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
UPDATE asset_event_types SET updated_at = created_at;
CREATE UNIQUE INDEX asset_event_types_system_idx ON asset_event_types(tenant_id, system_code) WHERE system_code <> '';

INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, updated_at)
SELECT gen_random_uuid(), m.tenant_id, 'purchase', 'purchase', 'expense', m.user_id, m.created_at, 'purchase', m.created_at
FROM tenant_memberships m WHERE m.user_id::text = (SELECT MIN(user_id::text) FROM tenant_memberships WHERE tenant_id = m.tenant_id);

INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, updated_at)
SELECT gen_random_uuid(), m.tenant_id, 'repair', 'repair', 'expense', m.user_id, m.created_at, 'repair', m.created_at
FROM tenant_memberships m WHERE m.user_id::text = (SELECT MIN(user_id::text) FROM tenant_memberships WHERE tenant_id = m.tenant_id);

INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, updated_at)
SELECT gen_random_uuid(), m.tenant_id, 'sale', 'sale', 'income', m.user_id, m.created_at, 'sale', m.created_at
FROM tenant_memberships m WHERE m.user_id::text = (SELECT MIN(user_id::text) FROM tenant_memberships WHERE tenant_id = m.tenant_id);

INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, updated_at)
SELECT gen_random_uuid(), m.tenant_id, 'void', 'void', 'neutral', m.user_id, m.created_at, 'void', m.created_at
FROM tenant_memberships m WHERE m.user_id::text = (SELECT MIN(user_id::text) FROM tenant_memberships WHERE tenant_id = m.tenant_id);

-- +goose StatementBegin
CREATE FUNCTION assetloop_seed_lifecycle_types() RETURNS trigger AS $$
BEGIN
INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, updated_at) VALUES (gen_random_uuid(), NEW.tenant_id, 'purchase', 'purchase', 'expense', NEW.user_id, NEW.created_at, 'purchase', NEW.created_at) ON CONFLICT (tenant_id, normalized_name) DO NOTHING;
INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, updated_at) VALUES (gen_random_uuid(), NEW.tenant_id, 'repair', 'repair', 'expense', NEW.user_id, NEW.created_at, 'repair', NEW.created_at) ON CONFLICT (tenant_id, normalized_name) DO NOTHING;
INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, updated_at) VALUES (gen_random_uuid(), NEW.tenant_id, 'sale', 'sale', 'income', NEW.user_id, NEW.created_at, 'sale', NEW.created_at) ON CONFLICT (tenant_id, normalized_name) DO NOTHING;
INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, updated_at) VALUES (gen_random_uuid(), NEW.tenant_id, 'void', 'void', 'neutral', NEW.user_id, NEW.created_at, 'void', NEW.created_at) ON CONFLICT (tenant_id, normalized_name) DO NOTHING;
RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER seed_lifecycle_types AFTER INSERT ON tenant_memberships FOR EACH ROW EXECUTE FUNCTION assetloop_seed_lifecycle_types();

ALTER TABLE asset_events ADD COLUMN event_type_id UUID;
DROP TRIGGER asset_events_no_update ON asset_events;
UPDATE asset_events e SET event_type_id = (SELECT t.id FROM asset_event_types t WHERE t.tenant_id = e.tenant_id AND (t.name = trim(e.event_type) OR t.normalized_name = lower(trim(e.event_type))));
ALTER TABLE asset_events ALTER COLUMN event_type_id SET NOT NULL;
ALTER TABLE asset_events ADD CONSTRAINT asset_events_type_fk FOREIGN KEY (tenant_id, event_type_id) REFERENCES asset_event_types(tenant_id, id);
CREATE INDEX asset_events_type_idx ON asset_events(tenant_id, event_type_id);
CREATE TRIGGER asset_events_no_update BEFORE UPDATE ON asset_events FOR EACH ROW EXECUTE FUNCTION assetloop_reject_asset_event_mutation();
