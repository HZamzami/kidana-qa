-- name: CreateFinding :one
INSERT INTO findings (work_order_id, inspection_id, criterion_id, severity, description,
                      corrective_action, owner, due_date, created_by, owner_user_id, repeat_of)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, sqlc.narg(owner_user_id), sqlc.narg(repeat_of))
RETURNING *;

-- name: GetFinding :one
SELECT f.*, w.ref AS wo_ref, w.contractor_id AS wo_contractor_id,
       COALESCE(c.code, '')::text AS criterion_code, COALESCE(c.label, '')::text AS criterion_label, COALESCE(c.label_en, '')::text AS criterion_label_en,
       COALESCE(ou.name, '')::text AS owner_user_name, COALESCE(ru.name, '')::text AS responded_by_name,
       COALESCE(i.stage::text, '')::text AS inspection_stage
FROM findings f
JOIN work_orders w ON w.id = f.work_order_id
LEFT JOIN template_criteria c ON c.id = f.criterion_id
LEFT JOIN users ou ON ou.id = f.owner_user_id
LEFT JOIN users ru ON ru.id = f.responded_by
LEFT JOIN inspections i ON i.id = f.inspection_id
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
SELECT f.*, w.ref AS wo_ref, COALESCE(ou.name, '')::text AS owner_user_name,
       COALESCE(i.stage::text, '')::text AS inspection_stage, s.name AS site_name, s.name_en AS site_name_en, COALESCE(ct.name, '')::text AS contractor_name, COALESCE(ct.name_en, '')::text AS contractor_name_en,
       COALESCE(c.code, '')::text AS criterion_code, COALESCE(c.label, '')::text AS criterion_label, COALESCE(c.label_en, '')::text AS criterion_label_en
FROM findings f
JOIN work_orders w ON w.id = f.work_order_id
JOIN sites s ON s.id = w.site_id
LEFT JOIN contractors ct ON ct.id = w.contractor_id
LEFT JOIN template_criteria c ON c.id = f.criterion_id
LEFT JOIN users ou ON ou.id = f.owner_user_id
LEFT JOIN inspections i ON i.id = f.inspection_id
WHERE (sqlc.narg(status)::finding_status IS NULL OR f.status = sqlc.narg(status))
  AND (NOT sqlc.arg(open_only)::boolean OR f.status <> 'closed')
  AND (NOT sqlc.arg(overdue_only)::boolean OR (f.status <> 'closed' AND f.due_date < CURRENT_DATE))
  AND (NOT sqlc.arg(repeat_only)::boolean OR f.repeat_of IS NOT NULL)
  AND (sqlc.narg(site_id)::bigint IS NULL OR w.site_id = sqlc.narg(site_id))
  AND (sqlc.narg(contractor_id)::bigint IS NULL OR w.contractor_id = sqlc.narg(contractor_id))
  AND (sqlc.narg(system_id)::bigint IS NULL OR w.system_id = sqlc.narg(system_id))
  AND (sqlc.narg(asset_group_id)::bigint IS NULL OR w.asset_group_id = sqlc.narg(asset_group_id))
  AND (sqlc.narg(severity)::finding_severity IS NULL OR f.severity = sqlc.narg(severity))
  AND (sqlc.narg(stage)::inspection_stage IS NULL OR i.stage = sqlc.narg(stage))
  AND (sqlc.narg(owner_user_id)::bigint IS NULL OR f.owner_user_id = sqlc.narg(owner_user_id))
  AND (sqlc.narg(q)::text IS NULL
       OR w.ref ILIKE '%' || sqlc.narg(q) || '%'
       OR f.description ILIKE '%' || sqlc.narg(q) || '%'
       OR f.owner ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY (f.status <> 'closed') DESC, f.due_date NULLS LAST, f.created_at DESC
LIMIT 500;

-- name: UpdateFinding :one
UPDATE findings SET description = $2, corrective_action = $3, owner = $4, due_date = $5,
                    severity = $6, status = $7, owner_user_id = sqlc.narg(owner_user_id),
                    root_cause_category = sqlc.narg(root_cause_category), root_cause = sqlc.arg(root_cause),
                    updated_at = now()
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

-- The contractor's answer: what was done (photos are attachments).
-- name: RespondFinding :execrows
UPDATE findings SET status = 'responded', response_note = $2, responded_by = $3, responded_at = now(),
                    root_cause_category = COALESCE(sqlc.narg(root_cause_category), root_cause_category),
                    root_cause = CASE WHEN sqlc.arg(root_cause)::text = '' THEN root_cause ELSE sqlc.arg(root_cause)::text END,
                    rejection_note = '', updated_at = now()
WHERE id = $1 AND status IN ('open', 'in_progress');

-- The inspector does not accept the answer: back to open, with the reason.
-- name: RejectFindingResponse :execrows
UPDATE findings SET status = 'open', rejection_note = $2, updated_at = now()
WHERE id = $1 AND status = 'responded';

-- Responses waiting for an inspector: the one who raised the finding, or everyone when NULL.
-- name: ListFindingsToVerify :many
SELECT f.id, f.description, f.response_note, f.responded_at, w.ref AS wo_ref, w.id AS work_order_id
FROM findings f JOIN work_orders w ON w.id = f.work_order_id
WHERE f.status = 'responded' AND (sqlc.narg(created_by)::bigint IS NULL OR f.created_by = sqlc.narg(created_by))
ORDER BY f.responded_at;

-- An earlier finding on the same asset and criterion within the window.
-- name: FindRepeatOf :one
SELECT f.id FROM findings f JOIN work_orders w ON w.id = f.work_order_id
WHERE w.asset <> '' AND lower(w.asset) = lower(sqlc.arg(asset)::text) AND f.criterion_id = sqlc.arg(criterion_id)
  AND f.work_order_id <> sqlc.arg(work_order_id) AND f.created_at >= sqlc.arg(since)::timestamptz
ORDER BY f.created_at DESC LIMIT 1;

-- Bulk changes from the findings list. NULL leaves a field as it is.
-- name: BulkUpdateFindings :execrows
UPDATE findings SET
    due_date = COALESCE(sqlc.narg(due_date), due_date),
    owner = COALESCE(sqlc.narg(owner), owner),
    status = CASE WHEN sqlc.arg(mark_in_progress)::boolean AND status = 'open' THEN 'in_progress'::finding_status ELSE status END,
    updated_at = now()
WHERE id = ANY(sqlc.arg(ids)::bigint[]) AND status <> 'closed';
