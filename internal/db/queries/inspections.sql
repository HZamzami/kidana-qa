-- name: NextInspectionAttempt :one
SELECT (COALESCE(max(attempt), 0) + 1)::int FROM inspections WHERE work_order_id = $1 AND stage = $2;

-- name: CreateInspection :one
INSERT INTO inspections (work_order_id, stage, attempt, template_id, inspector_id, inspected_on)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- Creates one empty item per criterion of the inspection's template.
-- name: CreateInspectionItems :exec
INSERT INTO inspection_items (inspection_id, criterion_id)
SELECT i.id, c.id FROM inspections i
JOIN template_criteria c ON c.template_id = i.template_id
WHERE i.id = $1;

-- name: GetInspection :one
SELECT i.*, w.ref AS wo_ref, u.name AS inspector_name, t.name AS template_name
FROM inspections i
JOIN work_orders w ON w.id = i.work_order_id
JOIN users u ON u.id = i.inspector_id
JOIN checklist_templates t ON t.id = i.template_id
WHERE i.id = $1;

-- name: ListInspectionsByWorkOrder :many
SELECT i.*, u.name AS inspector_name,
       (SELECT count(*) FROM findings f WHERE f.inspection_id = i.id)::int AS findings_count
FROM inspections i
JOIN users u ON u.id = i.inspector_id
WHERE i.work_order_id = $1
ORDER BY i.created_at, i.id;

-- name: ListInspectionItems :many
SELECT it.inspection_id, it.criterion_id, it.rating, it.note,
       c.code, c.label, c.label_en, c.description, c.description_en, c.weight, c.is_critical, c.sort
FROM inspection_items it
JOIN template_criteria c ON c.id = it.criterion_id
WHERE it.inspection_id = $1
ORDER BY c.sort, c.code;

-- name: SetItemRating :execrows
UPDATE inspection_items SET rating = $3
WHERE inspection_id = $1 AND criterion_id = $2;

-- name: SetItemNote :execrows
UPDATE inspection_items SET note = $3
WHERE inspection_id = $1 AND criterion_id = $2;

-- name: UpdateInspectionHeader :exec
UPDATE inspections SET notes = $2, inspected_on = $3, updated_at = now()
WHERE id = $1 AND status = 'draft';

-- name: SubmitInspection :exec
UPDATE inspections
SET status = 'submitted', submitted_at = now(), score = $2, passed = $3, critical_failed = $4, updated_at = now()
WHERE id = $1 AND status = 'draft';

-- Re-opens a submitted inspection for correction (admin only).
-- name: ReopenInspection :exec
UPDATE inspections SET status = 'draft', submitted_at = NULL, score = NULL, passed = NULL,
                       critical_failed = false, updated_at = now()
WHERE id = $1;

-- name: DeleteDraftInspection :execrows
DELETE FROM inspections WHERE id = $1 AND status = 'draft';

-- name: AcceptanceFacts :one
SELECT
    w.status,
    EXISTS (SELECT 1 FROM inspections i WHERE i.work_order_id = w.id AND i.status = 'submitted')::boolean AS has_submitted,
    EXISTS (SELECT 1 FROM inspections i WHERE i.work_order_id = w.id AND i.stage = 'after' AND i.status = 'submitted')::boolean AS has_submitted_after,
    COALESCE((SELECT i.passed FROM inspections i
              WHERE i.work_order_id = w.id AND i.stage = 'after' AND i.status = 'submitted'
              ORDER BY i.attempt DESC LIMIT 1), false)::boolean AS latest_after_passed,
    (SELECT count(*) FROM findings f WHERE f.work_order_id = w.id AND f.status <> 'closed')::int AS open_findings
FROM work_orders w
WHERE w.id = $1;


-- Submitted inspections of several work orders, for printing.
-- name: ListSubmittedInspectionsForWorkOrders :many
SELECT i.id, i.work_order_id, i.stage, i.attempt, i.inspected_on, i.score, i.passed, i.critical_failed, i.notes,
       u.name AS inspector_name
FROM inspections i
JOIN users u ON u.id = i.inspector_id
WHERE i.work_order_id = ANY(sqlc.arg(ids)::bigint[]) AND i.status = 'submitted'
ORDER BY i.work_order_id, i.stage, i.attempt;

-- Rated items of several inspections with their criteria, for printing.
-- name: ListItemsForPrint :many
SELECT it.inspection_id, it.rating, it.note, c.code, c.label, c.label_en, c.weight, c.is_critical, c.sort
FROM inspection_items it
JOIN template_criteria c ON c.id = it.criterion_id
WHERE it.inspection_id = ANY(sqlc.arg(ids)::bigint[])
ORDER BY it.inspection_id, c.sort, c.code;

-- Where the inspector was when the inspection started (set once).
-- name: SetInspectionLocation :exec
UPDATE inspections SET lat = $2, lng = $3, accuracy = $4, located_at = now()
WHERE id = $1 AND status = 'draft' AND lat IS NULL;
