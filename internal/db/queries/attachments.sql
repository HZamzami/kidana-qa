-- name: CreateAttachment :one
INSERT INTO attachments (inspection_id, criterion_id, finding_id, storage_key, filename, content_type, size_bytes, uploaded_by, purpose)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, sqlc.arg(purpose))
RETURNING *;

-- name: GetAttachment :one
SELECT * FROM attachments WHERE id = $1;

-- name: ListAttachmentsByInspection :many
SELECT * FROM attachments WHERE inspection_id = $1 ORDER BY created_at;

-- name: ListAttachmentsByFinding :many
SELECT * FROM attachments WHERE finding_id = $1 ORDER BY created_at;

-- name: DeleteAttachment :exec
DELETE FROM attachments WHERE id = $1;

-- name: ListAttachmentsByWorkOrder :many
SELECT a.* FROM attachments a
LEFT JOIN inspections i ON i.id = a.inspection_id
LEFT JOIN findings f ON f.id = a.finding_id
WHERE i.work_order_id = $1 OR f.work_order_id = $1;
