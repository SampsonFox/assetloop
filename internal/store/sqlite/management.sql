-- name: FindManagementRequest :one
SELECT request_hash, result_json FROM management_requests
WHERE tenant_id = sqlc.arg(tenant_id) AND user_id = sqlc.arg(user_id) AND request_key = sqlc.arg(request_key);

-- name: SaveManagementRequest :exec
INSERT INTO management_requests (tenant_id,user_id,request_key,request_hash,result_json)
VALUES (sqlc.arg(tenant_id),sqlc.arg(user_id),sqlc.arg(request_key),sqlc.arg(request_hash),sqlc.arg(result_json));

-- name: AssetDeletionExists :one
SELECT COUNT(*) FROM asset_deletions WHERE tenant_id = sqlc.arg(tenant_id) AND asset_id = sqlc.arg(asset_id);
-- name: MarkAssetDeleted :exec
INSERT INTO asset_deletions (tenant_id, asset_id, actor_user_id, deleted_at) VALUES (sqlc.arg(tenant_id), sqlc.arg(asset_id), sqlc.arg(actor_user_id), sqlc.arg(deleted_at));
-- name: AssetDeletionTransactions :many
SELECT transaction_id FROM asset_events WHERE asset_events.tenant_id = sqlc.arg(tenant_id) AND asset_events.asset_id = sqlc.arg(asset_id)
UNION SELECT confirmed_transaction_id AS transaction_id FROM import_drafts WHERE import_drafts.tenant_id = sqlc.arg(tenant_id) AND asset_id = sqlc.arg(asset_id) AND confirmed_transaction_id IS NOT NULL;
-- name: SaveDeletedLifecycleRequests :exec
INSERT INTO deleted_lifecycle_requests (tenant_id,user_id,request_key,request_hash)
SELECT lifecycle_requests.tenant_id,user_id,request_key,request_hash FROM lifecycle_requests
WHERE lifecycle_requests.tenant_id = sqlc.arg(tenant_id) AND event_id IN (SELECT id FROM asset_events WHERE asset_events.tenant_id = sqlc.arg(tenant_id) AND asset_id = sqlc.arg(asset_id));
-- name: DeletedLifecycleRequestExists :one
SELECT COUNT(*) FROM deleted_lifecycle_requests WHERE tenant_id = sqlc.arg(tenant_id) AND user_id = sqlc.arg(user_id) AND request_key = sqlc.arg(request_key);
-- name: PurgeLifecycleRequests :exec
DELETE FROM lifecycle_requests WHERE lifecycle_requests.tenant_id = sqlc.arg(tenant_id) AND event_id IN (SELECT id FROM asset_events WHERE asset_events.tenant_id = sqlc.arg(tenant_id) AND asset_id = sqlc.arg(asset_id));
-- name: PurgeAssetDrafts :exec
DELETE FROM import_drafts WHERE import_drafts.tenant_id = sqlc.arg(tenant_id) AND asset_id = sqlc.arg(asset_id);
-- name: PurgeAssetTags :exec
DELETE FROM asset_specification_tags WHERE tenant_id = sqlc.arg(tenant_id) AND asset_id = sqlc.arg(asset_id);
-- name: PurgeAssetMarket :exec
DELETE FROM asset_market_bindings WHERE tenant_id = sqlc.arg(tenant_id) AND asset_id = sqlc.arg(asset_id);
-- name: PurgeAssetEvents :exec
DELETE FROM asset_events WHERE tenant_id = sqlc.arg(tenant_id) AND asset_id = sqlc.arg(asset_id);
-- name: PurgeAsset :execrows
DELETE FROM assets WHERE tenant_id = sqlc.arg(tenant_id) AND id = sqlc.arg(asset_id);
-- name: PurgeUnusedTransaction :exec
DELETE FROM asset_transactions WHERE asset_transactions.tenant_id = sqlc.arg(tenant_id) AND asset_transactions.id = sqlc.arg(transaction_id)
AND NOT EXISTS (SELECT 1 FROM asset_events e WHERE e.tenant_id = asset_transactions.tenant_id AND e.transaction_id = asset_transactions.id)
AND NOT EXISTS (SELECT 1 FROM import_drafts d WHERE d.tenant_id = asset_transactions.tenant_id AND d.confirmed_transaction_id = asset_transactions.id);
-- name: AssetManagementReceipts :many
SELECT user_id,request_key,result_json FROM management_requests WHERE tenant_id = sqlc.arg(tenant_id);
-- name: RedactManagementReceipt :exec
UPDATE management_requests SET result_json = '{"asset_deleted":true}' WHERE tenant_id = sqlc.arg(tenant_id) AND user_id = sqlc.arg(user_id) AND request_key = sqlc.arg(request_key);
