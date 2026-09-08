-- name: GetMarketItem :one
SELECT id, CAST(tenant_id AS TEXT) AS tenant_id, name,provider,keyword,filter_criteria,model_desc,region,external_id,enabled,created_at,last_attempt,last_success,last_error,lease_token,lease_until FROM market_items WHERE CAST(tenant_id AS TEXT)=CAST(sqlc.arg(tenant) AS TEXT) AND id=sqlc.arg(id);
-- name: ListMarketItems :many
SELECT id, CAST(tenant_id AS TEXT) AS tenant_id, name,provider,keyword,filter_criteria,model_desc,region,external_id,enabled,created_at,last_attempt,last_success,last_error,lease_token,lease_until FROM market_items WHERE CAST(tenant_id AS TEXT)=CAST(sqlc.arg(tenant) AS TEXT) ORDER BY name,id;
-- name: ListMarketTenants :many
SELECT DISTINCT CAST(tenant_id AS TEXT) AS tenant FROM market_items ORDER BY tenant;
-- name: CreateMarketItem :exec
INSERT INTO market_items(id,tenant_id,name,provider,keyword,filter_criteria,model_desc,region,external_id,enabled,created_at,last_success) VALUES(sqlc.arg(id),CAST(sqlc.arg(tenant) AS TEXT),sqlc.arg(name),sqlc.arg(provider),sqlc.arg(keyword),sqlc.arg(filter_criteria),sqlc.arg(model_desc),sqlc.arg(region),sqlc.narg(external_id),sqlc.arg(enabled),sqlc.arg(created_at),sqlc.arg(last_success));
-- name: UpdateMarketItem :execrows
UPDATE market_items SET name=sqlc.arg(name),enabled=sqlc.arg(enabled) WHERE CAST(tenant_id AS TEXT)=CAST(sqlc.arg(tenant) AS TEXT) AND id=sqlc.arg(id);
-- name: BindAssetMarket :exec
INSERT INTO asset_market_bindings(tenant_id,asset_id,market_item_id) VALUES(CAST(sqlc.arg(tenant) AS TEXT),CAST(sqlc.arg(asset_id) AS TEXT),sqlc.arg(market_item_id)) ON CONFLICT(tenant_id,asset_id) DO UPDATE SET market_item_id=excluded.market_item_id;
-- name: UnbindAssetMarket :exec
DELETE FROM asset_market_bindings WHERE CAST(tenant_id AS TEXT)=CAST(sqlc.arg(tenant) AS TEXT) AND CAST(asset_id AS TEXT)=CAST(sqlc.arg(asset_id) AS TEXT);
-- name: MarketAssetIDs :many
SELECT CAST(asset_id AS TEXT) AS asset_id FROM asset_market_bindings WHERE CAST(tenant_id AS TEXT)=CAST(sqlc.arg(tenant) AS TEXT) AND market_item_id=sqlc.arg(id) ORDER BY asset_id;
-- name: PutMarketPrice :exec
INSERT INTO market_prices(tenant_id,market_item_id,observation_date,observed_at,max_minor,min_minor,currency,base_currency,base_minor,rate_scaled,rate_date,rate_source,provider,provider_version,provenance,evidence,source_date,sample_count)
VALUES(CAST(sqlc.arg(tenant) AS TEXT),sqlc.arg(market_item_id),sqlc.arg(observation_date),sqlc.arg(observed_at),sqlc.arg(max_minor),sqlc.narg(min_minor),sqlc.arg(currency),sqlc.arg(base_currency),sqlc.narg(base_minor),sqlc.narg(rate_scaled),sqlc.narg(rate_date),sqlc.narg(rate_source),sqlc.arg(provider),sqlc.arg(provider_version),sqlc.arg(provenance),sqlc.arg(evidence),sqlc.narg(source_date),sqlc.narg(sample_count))
ON CONFLICT(tenant_id,market_item_id,observation_date) DO UPDATE SET observed_at=excluded.observed_at,max_minor=excluded.max_minor,min_minor=excluded.min_minor,currency=excluded.currency,base_currency=excluded.base_currency,base_minor=excluded.base_minor,rate_scaled=excluded.rate_scaled,rate_date=excluded.rate_date,rate_source=excluded.rate_source,provider=excluded.provider,provider_version=excluded.provider_version,provenance=excluded.provenance,evidence=excluded.evidence,source_date=excluded.source_date,sample_count=excluded.sample_count
WHERE excluded.observed_at >= market_prices.observed_at;
-- name: ListMarketPrices :many
SELECT CAST(tenant_id AS TEXT) AS tenant_id,market_item_id,observation_date,observed_at,max_minor,min_minor,currency,base_currency,base_minor,rate_scaled,rate_date,rate_source,provider,provider_version,provenance,evidence,source_date,sample_count FROM market_prices WHERE CAST(tenant_id AS TEXT)=CAST(sqlc.arg(tenant) AS TEXT) AND market_item_id=sqlc.arg(id) ORDER BY observation_date DESC;
-- name: ClaimMarketLease :execrows
UPDATE market_items SET lease_token=sqlc.arg(token),lease_until=sqlc.arg(until_time),last_attempt=sqlc.arg(now_time) WHERE CAST(tenant_id AS TEXT)=CAST(sqlc.arg(tenant) AS TEXT) AND id=sqlc.arg(id) AND enabled=1 AND lease_until<=sqlc.arg(now_time);
-- name: FinishMarketLease :execrows
UPDATE market_items SET lease_token='',lease_until=0,last_error=sqlc.arg(last_error),last_success=CASE WHEN sqlc.arg(last_error)='' THEN sqlc.arg(success_time) ELSE last_success END WHERE CAST(tenant_id AS TEXT)=CAST(sqlc.arg(tenant) AS TEXT) AND id=sqlc.arg(id) AND lease_token=sqlc.arg(token);
