-- +goose Up
ALTER TABLE assets ADD COLUMN serial_number TEXT NOT NULL DEFAULT '';
ALTER TABLE assets ADD COLUMN color TEXT NOT NULL DEFAULT '';
ALTER TABLE assets ADD COLUMN purchase_channel TEXT NOT NULL DEFAULT '';
ALTER TABLE assets ADD COLUMN notes TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX assets_tenant_serial_idx
    ON assets(tenant_id, serial_number)
    WHERE serial_number <> '';
