-- +goose Up
ALTER TABLE asset_events DROP CONSTRAINT asset_events_event_type_check;
ALTER TABLE asset_events DROP CONSTRAINT asset_events_check;
ALTER TABLE asset_events
    ADD CONSTRAINT asset_events_event_type_valid CHECK (length(trim(event_type)) BETWEEN 1 AND 80),
    ADD CONSTRAINT asset_events_amount_shape_valid CHECK (
        (event_type = 'void' AND base_amount_minor = 0 AND voids_event_id IS NOT NULL)
        OR (event_type <> 'void' AND voids_event_id IS NULL)
    );

CREATE TABLE asset_event_types (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 80),
    normalized_name TEXT NOT NULL,
    cashflow_direction TEXT NOT NULL CHECK (cashflow_direction IN ('expense', 'income', 'neutral')),
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, normalized_name),
    FOREIGN KEY (tenant_id, created_by_user_id) REFERENCES tenant_memberships(tenant_id, user_id)
);

CREATE INDEX asset_event_types_tenant_name_idx
    ON asset_event_types(tenant_id, normalized_name);
