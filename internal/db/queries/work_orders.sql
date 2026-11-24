-- name: CreateWorkOrder :one
INSERT INTO work_orders (ref, type, site_id, location, system_id, asset_group_id, asset, contractor_id,
                         technician, priority, description, planned_date, actual_date, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: UpdateWorkOrder :one
UPDATE work_orders SET
    ref = $2, type = $3, site_id = $4, location = $5, system_id = $6, asset_group_id = $7, asset = $8,
    contractor_id = $9, technician = $10, priority = $11, description = $12,
    planned_date = $13, actual_date = $14, status = $15, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetWorkOrderStatus :exec
UPDATE work_orders SET status = $2, updated_at = now() WHERE id = $1;

-- name: GetWorkOrderRefExists :one
SELECT EXISTS (SELECT 1 FROM work_orders WHERE lower(ref) = lower(sqlc.arg(ref)) AND id <> sqlc.arg(exclude_id));

-- name: GetWorkOrderByRef :one
SELECT * FROM work_orders WHERE lower(ref) = lower($1);

-- name: GetWorkOrder :one
SELECT w.*, s.name AS site_name, sy.name AS system_name, ag.name AS asset_group_name,
       COALESCE(c.name, '')::text AS contractor_name,
       COALESCE(u.name, '')::text AS created_by_name
FROM work_orders w
JOIN sites s ON s.id = w.site_id
JOIN systems sy ON sy.id = w.system_id
JOIN asset_groups ag ON ag.id = w.asset_group_id
LEFT JOIN contractors c ON c.id = w.contractor_id
LEFT JOIN users u ON u.id = w.created_by
WHERE w.id = $1;

-- Filters are optional: a NULL argument disables that filter.
-- Latest submitted during/after scores are reported as -1 when there is none.
-- name: ListWorkOrders :many
SELECT w.id, w.ref, w.type, w.asset, w.location, w.status, w.priority, w.planned_date, w.actual_date, w.created_at,
       s.name AS site_name, sy.name AS system_name, ag.name AS asset_group_name, COALESCE(c.name, '')::text AS contractor_name,
       COALESCE(dur.score, -1)::double precision AS during_score,
       COALESCE(aft.score, -1)::double precision AS after_score,
       COALESCE(aft.passed, false)::boolean      AS after_passed,
       COALESCE(aft.attempt, 0)::int             AS after_attempt,
       (SELECT count(*) FROM findings f WHERE f.work_order_id = w.id AND f.status <> 'closed')::int AS open_findings,
       (SELECT count(*) FROM inspections d WHERE d.work_order_id = w.id AND d.status = 'draft')::int AS draft_inspections
FROM work_orders w
JOIN sites s ON s.id = w.site_id
JOIN systems sy ON sy.id = w.system_id
JOIN asset_groups ag ON ag.id = w.asset_group_id
LEFT JOIN contractors c ON c.id = w.contractor_id
LEFT JOIN LATERAL (
    SELECT i.score FROM inspections i
    WHERE i.work_order_id = w.id AND i.stage = 'during' AND i.status = 'submitted'
    ORDER BY i.attempt DESC LIMIT 1
) dur ON true
LEFT JOIN LATERAL (
    SELECT i.score, i.passed, i.attempt FROM inspections i
    WHERE i.work_order_id = w.id AND i.stage = 'after' AND i.status = 'submitted'
    ORDER BY i.attempt DESC LIMIT 1
) aft ON true
WHERE (sqlc.narg(site_id)::bigint IS NULL OR w.site_id = sqlc.narg(site_id))
  AND (sqlc.narg(system_id)::bigint IS NULL OR w.system_id = sqlc.narg(system_id))
  AND (sqlc.narg(asset_group_id)::bigint IS NULL OR w.asset_group_id = sqlc.narg(asset_group_id))
  AND (sqlc.narg(contractor_id)::bigint IS NULL OR w.contractor_id = sqlc.narg(contractor_id))
  AND (sqlc.narg(type)::wo_type IS NULL OR w.type = sqlc.narg(type))
  AND (sqlc.narg(status)::wo_status IS NULL OR w.status = sqlc.narg(status))
  AND (sqlc.narg(priority)::wo_priority IS NULL OR w.priority = sqlc.narg(priority))
  AND (sqlc.narg(q)::text IS NULL
       OR w.ref ILIKE '%' || sqlc.narg(q) || '%'
       OR w.asset ILIKE '%' || sqlc.narg(q) || '%'
       OR w.location ILIKE '%' || sqlc.narg(q) || '%'
       OR w.technician ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY w.created_at DESC, w.id DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountWorkOrders :one
SELECT count(*)
FROM work_orders w
WHERE (sqlc.narg(site_id)::bigint IS NULL OR w.site_id = sqlc.narg(site_id))
  AND (sqlc.narg(system_id)::bigint IS NULL OR w.system_id = sqlc.narg(system_id))
  AND (sqlc.narg(asset_group_id)::bigint IS NULL OR w.asset_group_id = sqlc.narg(asset_group_id))
  AND (sqlc.narg(contractor_id)::bigint IS NULL OR w.contractor_id = sqlc.narg(contractor_id))
  AND (sqlc.narg(type)::wo_type IS NULL OR w.type = sqlc.narg(type))
  AND (sqlc.narg(status)::wo_status IS NULL OR w.status = sqlc.narg(status))
  AND (sqlc.narg(priority)::wo_priority IS NULL OR w.priority = sqlc.narg(priority))
  AND (sqlc.narg(q)::text IS NULL
       OR w.ref ILIKE '%' || sqlc.narg(q) || '%'
       OR w.asset ILIKE '%' || sqlc.narg(q) || '%'
       OR w.location ILIKE '%' || sqlc.narg(q) || '%'
       OR w.technician ILIKE '%' || sqlc.narg(q) || '%');

-- Deletes a work order with its inspections, findings and attachments (cascade).
-- name: DeleteWorkOrder :exec
DELETE FROM work_orders WHERE id = $1;
