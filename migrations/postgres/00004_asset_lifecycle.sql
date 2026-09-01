-- +goose Up
CREATE TABLE asset_transactions (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    occurred_at TIMESTAMPTZ NOT NULL,
    source TEXT NOT NULL,
    external_reference TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, created_by_user_id) REFERENCES tenant_memberships(tenant_id, user_id)
);

CREATE TABLE asset_events (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    asset_id UUID NOT NULL,
    transaction_id UUID NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('purchase', 'repair', 'sale', 'void')),
    base_amount_minor BIGINT NOT NULL,
    base_currency CHAR(3) NOT NULL,
    original_amount_minor BIGINT,
    original_currency CHAR(3),
    fx_rate_scaled BIGINT,
    fx_rate_date DATE,
    fx_rate_source TEXT,
    notes TEXT NOT NULL DEFAULT '',
    voids_event_id UUID,
    replaces_event_id UUID,
    occurred_at TIMESTAMPTZ NOT NULL,
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, id),
    CHECK (
        (event_type IN ('purchase', 'repair') AND base_amount_minor < 0 AND voids_event_id IS NULL)
        OR (event_type = 'sale' AND base_amount_minor > 0 AND voids_event_id IS NULL)
        OR (event_type = 'void' AND base_amount_minor = 0 AND voids_event_id IS NOT NULL)
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

CREATE INDEX asset_events_tenant_asset_time_idx
    ON asset_events(tenant_id, asset_id, occurred_at, created_at);
CREATE UNIQUE INDEX asset_events_void_once_idx
    ON asset_events(tenant_id, voids_event_id)
    WHERE voids_event_id IS NOT NULL;

CREATE FUNCTION assetloop_reject_asset_event_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'asset events are append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER asset_events_no_update
BEFORE UPDATE ON asset_events
FOR EACH ROW EXECUTE FUNCTION assetloop_reject_asset_event_mutation();

CREATE TRIGGER asset_events_no_delete
BEFORE DELETE ON asset_events
FOR EACH ROW EXECUTE FUNCTION assetloop_reject_asset_event_mutation();

CREATE TABLE import_drafts (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    asset_id UUID NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('purchase', 'repair', 'sale')),
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency CHAR(3) NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    source TEXT NOT NULL,
    external_reference TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    raw_text TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('pending', 'confirmed', 'rejected')),
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    confirmed_transaction_id UUID,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, asset_id) REFERENCES assets(tenant_id, id),
    FOREIGN KEY (tenant_id, created_by_user_id) REFERENCES tenant_memberships(tenant_id, user_id),
    FOREIGN KEY (tenant_id, confirmed_transaction_id) REFERENCES asset_transactions(tenant_id, id)
);

CREATE INDEX import_drafts_tenant_status_time_idx
    ON import_drafts(tenant_id, status, created_at);

CREATE FUNCTION assetloop_guard_base_currency() RETURNS trigger AS $$
BEGIN
    IF OLD.base_currency_locked AND NEW.base_currency <> OLD.base_currency THEN
        RAISE EXCEPTION 'base currency is locked after the first monetary record';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tenants_base_currency_locked
BEFORE UPDATE OF base_currency ON tenants
FOR EACH ROW EXECUTE FUNCTION assetloop_guard_base_currency();
