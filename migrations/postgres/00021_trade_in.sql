-- +goose Up
-- Widen the immutable system-code domain to include the two neutral pairing
-- types. Existing rows keep their code, name, direction and enabled state. The
-- paired Go migration seeds the built-in rows and replaces the tenant trigger
-- with collision-safe normalized keys, so a pre-existing custom type named
-- "trade_in_source" is never converted, renamed or removed.
ALTER TABLE asset_event_types DROP CONSTRAINT IF EXISTS asset_event_types_system_code_check;
ALTER TABLE asset_event_types ADD CONSTRAINT asset_event_types_system_code_check
    CHECK (system_code IN ('', 'purchase', 'repair', 'sale', 'void', 'trade_in_source', 'trade_in_destination'));

ALTER TABLE asset_events ADD COLUMN related_asset_id UUID;
ALTER TABLE asset_events ADD COLUMN related_asset_name TEXT NOT NULL DEFAULT '';
ALTER TABLE asset_events ADD COLUMN related_asset_spec TEXT NOT NULL DEFAULT '';
ALTER TABLE asset_events ADD COLUMN trade_in_link_id UUID;
ALTER TABLE asset_events ADD COLUMN trade_in_state TEXT NOT NULL DEFAULT '' CHECK (trade_in_state IN ('', 'active', 'cancelled'));
-- Deliberately no foreign key on related_asset_id: purging either endpoint must
-- neither cascade nor be blocked, and the write-time snapshot keeps the label.
CREATE INDEX asset_events_trade_in_idx ON asset_events(tenant_id, trade_in_link_id) WHERE trade_in_link_id IS NOT NULL;
CREATE INDEX asset_events_related_asset_idx ON asset_events(tenant_id, related_asset_id) WHERE related_asset_id IS NOT NULL;
