-- +goose Up
CREATE TABLE lifecycle_requests (
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    request_key TEXT NOT NULL CHECK (length(request_key) BETWEEN 1 AND 128),
    request_hash TEXT NOT NULL CHECK (length(request_hash) = 64),
    event_id UUID NOT NULL,
    PRIMARY KEY (tenant_id, user_id, request_key),
    FOREIGN KEY (tenant_id, user_id) REFERENCES tenant_memberships(tenant_id, user_id),
    FOREIGN KEY (tenant_id, event_id) REFERENCES asset_events(tenant_id, id)
);
