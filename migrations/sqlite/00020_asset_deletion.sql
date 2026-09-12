-- +goose Up
CREATE TABLE asset_deletions (
 tenant_id TEXT NOT NULL REFERENCES tenants(id),
 asset_id TEXT NOT NULL,
 actor_user_id TEXT NOT NULL,
 deleted_at TEXT NOT NULL,
 PRIMARY KEY (tenant_id, asset_id)
);
CREATE TABLE deleted_lifecycle_requests (
 tenant_id TEXT NOT NULL,
 user_id TEXT NOT NULL,
 request_key TEXT NOT NULL,
 request_hash TEXT NOT NULL,
 PRIMARY KEY (tenant_id, user_id, request_key),
 FOREIGN KEY (tenant_id, user_id) REFERENCES tenant_memberships(tenant_id, user_id)
);
DROP TRIGGER asset_events_no_delete;
-- +goose StatementBegin
CREATE TRIGGER asset_events_no_delete BEFORE DELETE ON asset_events
WHEN NOT EXISTS (SELECT 1 FROM asset_deletions WHERE tenant_id = OLD.tenant_id AND asset_id = OLD.asset_id)
BEGIN SELECT RAISE(ABORT, 'asset events are append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER assets_no_restore BEFORE INSERT ON assets
WHEN EXISTS (SELECT 1 FROM asset_deletions WHERE tenant_id = NEW.tenant_id AND asset_id = NEW.id)
BEGIN SELECT RAISE(ABORT, 'asset permanently deleted'); END;
-- +goose StatementEnd
