-- name: GetModelImage :one
SELECT * FROM model_images WHERE tenant_id=$1 AND model_id=$2 AND active=1;
-- name: GetImageRevision :one
SELECT * FROM model_images WHERE tenant_id=$1 AND id=$2;
-- name: ClearModelImage :exec
UPDATE model_images SET active=0 WHERE tenant_id=$1 AND model_id=$2 AND active=1;
-- name: InsertModelImage :exec
INSERT INTO model_images(id,tenant_id,model_id,store_id,object_key,sha256,content_type,size_bytes,source_url,active)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,1);
