-- Submitted inspections joined with their work order, for the dashboard,
-- Excel export and printed report. Aggregation happens in Go (internal/domain).
-- name: ListSubmittedInspections :many
SELECT i.id, i.stage, i.attempt, i.inspected_on, i.score, i.passed, i.critical_failed, i.notes,
       u.name AS inspector_name, t.name AS template_name,
       w.id AS work_order_id, w.ref, w.type, w.asset, w.location, w.technician, w.priority,
       w.planned_date, w.actual_date, w.status AS wo_status,
       s.name AS site_name, sy.name AS system_name, COALESCE(c.name, '')::text AS contractor_name
FROM inspections i
JOIN work_orders w ON w.id = i.work_order_id
JOIN users u ON u.id = i.inspector_id
JOIN checklist_templates t ON t.id = i.template_id
JOIN sites s ON s.id = w.site_id
JOIN systems sy ON sy.id = w.system_id
LEFT JOIN contractors c ON c.id = w.contractor_id
WHERE i.status = 'submitted'
  AND (sqlc.narg(stage)::inspection_stage IS NULL OR i.stage = sqlc.narg(stage))
  AND (sqlc.narg(site_id)::bigint IS NULL OR w.site_id = sqlc.narg(site_id))
  AND (sqlc.narg(system_id)::bigint IS NULL OR w.system_id = sqlc.narg(system_id))
  AND (sqlc.narg(contractor_id)::bigint IS NULL OR w.contractor_id = sqlc.narg(contractor_id))
  AND (sqlc.narg(type)::wo_type IS NULL OR w.type = sqlc.narg(type))
  AND (sqlc.narg(date_from)::date IS NULL OR i.inspected_on >= sqlc.narg(date_from))
  AND (sqlc.narg(date_to)::date IS NULL OR i.inspected_on <= sqlc.narg(date_to))
ORDER BY i.inspected_on DESC, i.id DESC;

-- Ratings of the given inspections, for per-criterion compliance.
-- name: ListItemsForInspections :many
SELECT it.inspection_id, it.rating, c.code, c.label, c.weight, c.is_critical, c.sort, t.stage
FROM inspection_items it
JOIN template_criteria c ON c.id = it.criterion_id
JOIN checklist_templates t ON t.id = c.template_id
WHERE it.inspection_id = ANY(sqlc.arg(ids)::bigint[]);

-- Findings in the dashboard scope (by work orders of the given inspections' filters).
-- name: FindingStats :one
SELECT
    count(*) FILTER (WHERE f.status <> 'closed')::int AS open_count,
    count(*) FILTER (WHERE f.status <> 'closed' AND f.due_date < CURRENT_DATE)::int AS overdue_count,
    count(*) FILTER (WHERE f.severity = 'critical' AND f.status <> 'closed')::int AS open_critical
FROM findings f
JOIN work_orders w ON w.id = f.work_order_id
WHERE (sqlc.narg(site_id)::bigint IS NULL OR w.site_id = sqlc.narg(site_id))
  AND (sqlc.narg(system_id)::bigint IS NULL OR w.system_id = sqlc.narg(system_id))
  AND (sqlc.narg(contractor_id)::bigint IS NULL OR w.contractor_id = sqlc.narg(contractor_id))
  AND (sqlc.narg(type)::wo_type IS NULL OR w.type = sqlc.narg(type));

-- name: ListAllFindingsForExport :many
SELECT f.*, w.ref AS wo_ref, s.name AS site_name, COALESCE(ct.name, '')::text AS contractor_name,
       COALESCE(c.code, '')::text AS criterion_code, COALESCE(c.label, '')::text AS criterion_label
FROM findings f
JOIN work_orders w ON w.id = f.work_order_id
JOIN sites s ON s.id = w.site_id
LEFT JOIN contractors ct ON ct.id = w.contractor_id
LEFT JOIN template_criteria c ON c.id = f.criterion_id
ORDER BY f.created_at;
