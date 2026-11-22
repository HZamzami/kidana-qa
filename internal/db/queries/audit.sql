-- name: InsertAudit :exec
INSERT INTO audit_log (user_id, entity, entity_id, action, details)
VALUES ($1, $2, $3, $4, $5);

-- name: ListAudit :many
SELECT a.*, COALESCE(u.name, '')::text AS user_name
FROM audit_log a
LEFT JOIN users u ON u.id = a.user_id
WHERE a.entity = $1 AND a.entity_id = $2
ORDER BY a.created_at DESC
LIMIT 100;
