-- name: FindManagementRequest :one
SELECT request_hash, result_json FROM management_requests
WHERE tenant_id = sqlc.arg(tenant_id) AND user_id = sqlc.arg(user_id) AND request_key = sqlc.arg(request_key);

-- name: SaveManagementRequest :exec
INSERT INTO management_requests (tenant_id,user_id,request_key,request_hash,result_json)
VALUES (sqlc.arg(tenant_id),sqlc.arg(user_id),sqlc.arg(request_key),sqlc.arg(request_hash),sqlc.arg(result_json));
