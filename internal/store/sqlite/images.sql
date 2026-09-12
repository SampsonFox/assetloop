-- name: GetModelImage :one
SELECT * FROM model_images WHERE tenant_id=? AND model_id=? AND active=1;
-- name: GetImageRevision :one
SELECT * FROM model_images WHERE tenant_id=? AND id=?;
-- name: ClearModelImage :exec
UPDATE model_images SET active=0 WHERE tenant_id=? AND model_id=? AND active=1;
-- name: InsertModelImage :exec
INSERT INTO model_images(id,tenant_id,model_id,store_id,object_key,sha256,content_type,size_bytes,source_url,active)
VALUES (?,?,?,?,?,?,?,?,?,1);
