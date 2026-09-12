-- +goose Up
CREATE TABLE management_requests (
 tenant_id UUID NOT NULL,
 user_id UUID NOT NULL,
 request_key TEXT NOT NULL,
 request_hash TEXT NOT NULL,
 result_json TEXT NOT NULL,
 PRIMARY KEY (tenant_id, user_id, request_key),
 FOREIGN KEY (tenant_id, user_id) REFERENCES tenant_memberships(tenant_id, user_id)
);
