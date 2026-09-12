-- +goose Up
CREATE TABLE asset_deletions (
 tenant_id UUID NOT NULL REFERENCES tenants(id),
 asset_id UUID NOT NULL,
 actor_user_id UUID NOT NULL,
 deleted_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY (tenant_id, asset_id)
);
CREATE TABLE deleted_lifecycle_requests (
 tenant_id UUID NOT NULL,
 user_id UUID NOT NULL,
 request_key TEXT NOT NULL,
 request_hash TEXT NOT NULL,
 PRIMARY KEY (tenant_id, user_id, request_key),
 FOREIGN KEY (tenant_id, user_id) REFERENCES tenant_memberships(tenant_id, user_id)
);
-- +goose StatementBegin
CREATE FUNCTION assetloop_allow_whole_asset_delete() RETURNS trigger AS $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM asset_deletions WHERE tenant_id = OLD.tenant_id AND asset_id = OLD.asset_id) THEN
  RAISE EXCEPTION 'asset events are append-only';
 END IF;
 RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
DROP TRIGGER asset_events_no_delete ON asset_events;
CREATE TRIGGER asset_events_no_delete BEFORE DELETE ON asset_events FOR EACH ROW EXECUTE FUNCTION assetloop_allow_whole_asset_delete();
-- +goose StatementBegin
CREATE FUNCTION assetloop_reject_deleted_asset() RETURNS trigger AS $$
BEGIN
 IF EXISTS (SELECT 1 FROM asset_deletions WHERE tenant_id = NEW.tenant_id AND asset_id = NEW.id) THEN
  RAISE EXCEPTION 'asset permanently deleted';
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER assets_no_restore BEFORE INSERT ON assets FOR EACH ROW EXECUTE FUNCTION assetloop_reject_deleted_asset();
