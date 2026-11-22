-- name: CreateAttachment :one
INSERT INTO attachments (inspection_id, criterion_id, finding_id, storage_key, filename, content_type, size_bytes, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetAttachment :one
SELECT * FROM attachments WHERE id = $1;

-- name: ListAttachmentsByInspection :many
SELECT * FROM attachments WHERE inspection_id = $1 ORDER BY created_at;

-- name: ListAttachmentsByFinding :many
SELECT * FROM attachments WHERE finding_id = $1 ORDER BY created_at;

-- name: DeleteAttachment :exec
DELETE FROM attachments WHERE id = $1;
