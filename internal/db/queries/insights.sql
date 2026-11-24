-- ---------------------------------------------------------------- SLA
-- name: ListSLATargets :many
SELECT * FROM sla_targets ORDER BY key;

-- name: UpdateSLATarget :exec
UPDATE sla_targets SET days = $2 WHERE key = $1;

-- ---------------------------------------------------------------- settings
-- name: GetSetting :one
SELECT value FROM settings WHERE key = $1;

-- name: SetSetting :exec
INSERT INTO settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

-- ---------------------------------------------------------------- history
-- Everything that happened to a work order, its inspections, findings and assignments.
-- name: ListWorkOrderHistory :many
SELECT a.id, a.entity, a.entity_id, a.action, a.details, a.created_at, COALESCE(u.name, '')::text AS user_name
FROM audit_log a LEFT JOIN users u ON u.id = a.user_id
WHERE (a.entity = 'work_order' AND a.entity_id = $1)
   OR (a.entity = 'inspection' AND a.entity_id IN (SELECT id FROM inspections WHERE work_order_id = $1))
   OR (a.entity = 'finding' AND a.entity_id IN (SELECT id FROM findings WHERE work_order_id = $1))
   OR (a.entity = 'assignment' AND a.entity_id IN (SELECT id FROM inspection_assignments WHERE work_order_id = $1))
ORDER BY a.created_at DESC, a.id DESC
LIMIT 300;

-- name: SearchAudit :many
SELECT a.id, a.entity, a.entity_id, a.action, a.details, a.created_at, COALESCE(u.name, '')::text AS user_name
FROM audit_log a LEFT JOIN users u ON u.id = a.user_id
WHERE (sqlc.narg(user_id)::bigint IS NULL OR a.user_id = sqlc.narg(user_id))
  AND (sqlc.narg(entity)::text IS NULL OR a.entity = sqlc.narg(entity))
  AND (sqlc.narg(date_from)::date IS NULL OR a.created_at >= sqlc.narg(date_from)::date)
  AND (sqlc.narg(date_to)::date IS NULL OR a.created_at < sqlc.narg(date_to)::date + 1)
ORDER BY a.created_at DESC, a.id DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- ---------------------------------------------------------------- scorecards
-- Per contractor, over a period: inspection averages and first-time pass.
-- name: ContractorInspectionStats :many
SELECT c.id, c.name, c.name_en, c.active,
       COALESCE(avg(i.score) FILTER (WHERE i.stage = 'during'), -1)::double precision AS during_avg,
       COALESCE(avg(i.score) FILTER (WHERE i.stage = 'after'), -1)::double precision AS after_avg,
       count(i.id)::int AS inspections,
       count(i.id) FILTER (WHERE i.stage = 'after' AND i.attempt = 1)::int AS first_after,
       count(i.id) FILTER (WHERE i.stage = 'after' AND i.attempt = 1 AND i.passed)::int AS first_after_passed
FROM contractors c
LEFT JOIN work_orders w ON w.contractor_id = c.id
LEFT JOIN inspections i ON i.work_order_id = w.id AND i.status = 'submitted'
     AND i.inspected_on >= sqlc.arg(date_from) AND i.inspected_on <= sqlc.arg(date_to)
GROUP BY c.id
ORDER BY c.name;

-- Per contractor, findings raised in the period: open, overdue, closed on time, days to close.
-- name: ContractorFindingStats :many
SELECT w.contractor_id,
       count(*)::int AS total,
       count(*) FILTER (WHERE f.status <> 'closed')::int AS open,
       count(*) FILTER (WHERE f.status <> 'closed' AND f.due_date < CURRENT_DATE)::int AS overdue,
       count(*) FILTER (WHERE f.status = 'closed')::int AS closed,
       count(*) FILTER (WHERE f.status = 'closed' AND (f.due_date IS NULL OR f.closed_at::date <= f.due_date))::int AS closed_on_time,
       COALESCE(avg(EXTRACT(EPOCH FROM (f.closed_at - f.created_at)) / 86400) FILTER (WHERE f.status = 'closed'), -1)::double precision AS avg_days_to_close
FROM findings f JOIN work_orders w ON w.id = f.work_order_id
WHERE w.contractor_id IS NOT NULL AND f.created_at::date >= sqlc.arg(date_from) AND f.created_at::date <= sqlc.arg(date_to)
GROUP BY w.contractor_id;

-- Work orders due in the period, for resolution-target compliance.
-- name: ListWorkOrdersForSLA :many
SELECT w.id, w.ref, w.priority, w.type, w.reported_on, w.planned_date, w.actual_date, w.status, w.contractor_id,
       COALESCE(c.name, '')::text AS contractor_name, COALESCE(c.name_en, '')::text AS contractor_name_en
FROM work_orders w LEFT JOIN contractors c ON c.id = w.contractor_id
WHERE w.status <> 'cancelled' AND w.planned_date IS NOT NULL
  AND w.planned_date >= sqlc.arg(date_from) AND w.planned_date <= sqlc.arg(date_to)
  AND (sqlc.narg(contractor_id)::bigint IS NULL OR w.contractor_id = sqlc.narg(contractor_id))
ORDER BY w.planned_date;

-- Findings opened and closed per month, for the trend chart.
-- name: FindingsByMonth :many
SELECT to_char(m, 'YYYY-MM') AS month,
       (SELECT count(*) FROM findings f JOIN work_orders w ON w.id = f.work_order_id
         WHERE date_trunc('month', f.created_at) = m
           AND (sqlc.narg(contractor_id)::bigint IS NULL OR w.contractor_id = sqlc.narg(contractor_id)))::int AS opened,
       (SELECT count(*) FROM findings f JOIN work_orders w ON w.id = f.work_order_id
         WHERE date_trunc('month', f.closed_at) = m
           AND (sqlc.narg(contractor_id)::bigint IS NULL OR w.contractor_id = sqlc.narg(contractor_id)))::int AS closed
FROM generate_series(date_trunc('month', sqlc.arg(date_from)::date), date_trunc('month', sqlc.arg(date_to)::date), interval '1 month') AS m
ORDER BY m;

-- ---------------------------------------------------------------- saved views
-- name: ListSavedViews :many
SELECT v.*, u.name AS owner_name FROM saved_views v JOIN users u ON u.id = v.user_id
WHERE v.page = $2 AND (v.user_id = $1 OR v.shared)
ORDER BY v.user_id <> $1, v.name;

-- name: CreateSavedView :exec
INSERT INTO saved_views (user_id, page, name, query, shared) VALUES ($1, $2, $3, $4, $5);

-- name: DeleteSavedView :execrows
DELETE FROM saved_views WHERE id = $1 AND user_id = $2;

-- ---------------------------------------------------------------- report runs
-- name: CreateReportRun :one
INSERT INTO report_runs (kind, period) VALUES ($1, $2) ON CONFLICT (kind, period) DO NOTHING RETURNING *;

-- name: ListReportRuns :many
SELECT * FROM report_runs ORDER BY period DESC LIMIT 24;
