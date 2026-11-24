-- name: CreateFinding :one
INSERT INTO findings (work_order_id, inspection_id, criterion_id, severity, description,
                      corrective_action, owner, due_date, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetFinding :one
SELECT f.*, w.ref AS wo_ref, COALESCE(c.code, '')::text AS criterion_code, COALESCE(c.label, '')::text AS criterion_label, COALESCE(c.label_en, '')::text AS criterion_label_en
FROM findings f
JOIN work_orders w ON w.id = f.work_order_id
LEFT JOIN template_criteria c ON c.id = f.criterion_id
WHERE f.id = $1;

-- name: ListFindingsByWorkOrder :many
SELECT f.*, COALESCE(c.code, '')::text AS criterion_code, COALESCE(c.label, '')::text AS criterion_label, COALESCE(c.label_en, '')::text AS criterion_label_en,
       COALESCE(i.stage::text, '')::text AS inspection_stage, COALESCE(i.attempt, 0)::int AS inspection_attempt
FROM findings f
LEFT JOIN template_criteria c ON c.id = f.criterion_id
LEFT JOIN inspections i ON i.id = f.inspection_id
WHERE f.work_order_id = $1
ORDER BY f.status <> 'closed' DESC, f.created_at DESC;

-- name: ListFindingsByInspection :many
SELECT f.*, COALESCE(c.code, '')::text AS criterion_code, COALESCE(c.label, '')::text AS criterion_label, COALESCE(c.label_en, '')::text AS criterion_label_en
FROM findings f
LEFT JOIN template_criteria c ON c.id = f.criterion_id
WHERE f.inspection_id = $1
ORDER BY f.created_at;

-- name: CountFindingsByInspection :one
SELECT count(*)::int FROM findings WHERE inspection_id = $1;

-- name: ListFindings :many
SELECT f.*, w.ref AS wo_ref, s.name AS site_name, s.name_en AS site_name_en, COALESCE(ct.name, '')::text AS contractor_name, COALESCE(ct.name_en, '')::text AS contractor_name_en,
       COALESCE(c.code, '')::text AS criterion_code, COALESCE(c.label, '')::text AS criterion_label, COALESCE(c.label_en, '')::text AS criterion_label_en
FROM findings f
JOIN work_orders w ON w.id = f.work_order_id
JOIN sites s ON s.id = w.site_id
LEFT JOIN contractors ct ON ct.id = w.contractor_id
LEFT JOIN template_criteria c ON c.id = f.criterion_id
WHERE (sqlc.narg(status)::finding_status IS NULL OR f.status = sqlc.narg(status))
  AND (NOT sqlc.arg(open_only)::boolean OR f.status <> 'closed')
  AND (NOT sqlc.arg(overdue_only)::boolean OR (f.status <> 'closed' AND f.due_date < CURRENT_DATE))
  AND (sqlc.narg(site_id)::bigint IS NULL OR w.site_id = sqlc.narg(site_id))
  AND (sqlc.narg(contractor_id)::bigint IS NULL OR w.contractor_id = sqlc.narg(contractor_id))
  AND (sqlc.narg(q)::text IS NULL
       OR w.ref ILIKE '%' || sqlc.narg(q) || '%'
       OR f.description ILIKE '%' || sqlc.narg(q) || '%'
       OR f.owner ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY (f.status <> 'closed') DESC, f.due_date NULLS LAST, f.created_at DESC
LIMIT 500;

-- name: UpdateFinding :one
UPDATE findings SET description = $2, corrective_action = $3, owner = $4, due_date = $5,
                    severity = $6, status = $7, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CloseFinding :exec
UPDATE findings SET status = 'closed', closure_note = $2, closed_at = now(), closed_by = $3, updated_at = now()
WHERE id = $1;

-- name: ReopenFinding :exec
UPDATE findings SET status = 'open', closed_at = NULL, closed_by = NULL, updated_at = now()
WHERE id = $1;

-- name: DeleteFinding :exec
DELETE FROM findings WHERE id = $1;

-- Findings of several work orders, for printing: open first, then by date.
-- name: ListFindingsForWorkOrders :many
SELECT f.id, f.work_order_id, f.severity, f.description, f.corrective_action, f.owner, f.due_date,
       f.status, f.closed_at, f.created_at,
       COALESCE(c.code, '')::text AS criterion_code, COALESCE(c.label, '')::text AS criterion_label, COALESCE(c.label_en, '')::text AS criterion_label_en,
       COALESCE(i.stage::text, '')::text AS inspection_stage
FROM findings f
LEFT JOIN template_criteria c ON c.id = f.criterion_id
LEFT JOIN inspections i ON i.id = f.inspection_id
WHERE f.work_order_id = ANY(sqlc.arg(ids)::bigint[])
ORDER BY f.work_order_id, f.status = 'closed', f.created_at;
