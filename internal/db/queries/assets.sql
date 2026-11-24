-- name: ListAssets :many
SELECT a.*, COALESCE(s.name, '')::text AS site_name, COALESCE(s.name_en, '')::text AS site_name_en,
       COALESCE(sy.name, '')::text AS system_name, COALESCE(sy.name_en, '')::text AS system_name_en,
       COALESCE(ag.name, '')::text AS asset_group_name, COALESCE(ag.name_en, '')::text AS asset_group_name_en,
       (SELECT count(*) FROM work_orders w WHERE w.asset_id = a.id)::int AS work_orders,
       (SELECT count(*) FROM findings f JOIN work_orders w ON w.id = f.work_order_id
         WHERE w.asset_id = a.id AND f.status <> 'closed')::int AS open_findings,
       (SELECT count(*) FROM inspections i JOIN work_orders w ON w.id = i.work_order_id
         WHERE w.asset_id = a.id AND i.status = 'submitted' AND NOT i.passed)::int AS failures,
       COALESCE((SELECT max(i.inspected_on) FROM inspections i JOIN work_orders w ON w.id = i.work_order_id
         WHERE w.asset_id = a.id AND i.status = 'submitted'), '0001-01-01')::date AS last_inspected -- zero = never
FROM assets a
LEFT JOIN sites s ON s.id = a.site_id
LEFT JOIN systems sy ON sy.id = a.system_id
LEFT JOIN asset_groups ag ON ag.id = a.asset_group_id
WHERE (sqlc.narg(q)::text IS NULL OR a.code ILIKE '%' || sqlc.narg(q) || '%' OR a.name ILIKE '%' || sqlc.narg(q) || '%'
       OR a.name_en ILIKE '%' || sqlc.narg(q) || '%' OR a.location ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(site_id)::bigint IS NULL OR a.site_id = sqlc.narg(site_id))
  AND (sqlc.narg(system_id)::bigint IS NULL OR a.system_id = sqlc.narg(system_id))
  AND (sqlc.narg(asset_group_id)::bigint IS NULL OR a.asset_group_id = sqlc.narg(asset_group_id))
  AND (NOT sqlc.arg(active_only)::boolean OR a.active)
ORDER BY a.code
LIMIT sqlc.arg(lim);

-- name: GetAsset :one
SELECT a.*, COALESCE(s.name, '')::text AS site_name, COALESCE(s.name_en, '')::text AS site_name_en,
       COALESCE(sy.name, '')::text AS system_name, COALESCE(sy.name_en, '')::text AS system_name_en,
       COALESCE(ag.name, '')::text AS asset_group_name, COALESCE(ag.name_en, '')::text AS asset_group_name_en
FROM assets a
LEFT JOIN sites s ON s.id = a.site_id
LEFT JOIN systems sy ON sy.id = a.system_id
LEFT JOIN asset_groups ag ON ag.id = a.asset_group_id
WHERE a.id = $1;

-- name: GetAssetByCode :one
SELECT * FROM assets WHERE code = $1;

-- Finds or creates the asset for a work order's asset code; a new asset takes
-- the work order's site, system, group and location.
-- name: EnsureAsset :one
INSERT INTO assets (code, site_id, system_id, asset_group_id, location)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (code) DO UPDATE SET code = assets.code
RETURNING id;

-- name: UpdateAsset :exec
UPDATE assets SET name = $2, name_en = $3, site_id = $4, system_id = $5, asset_group_id = $6, location = $7, active = $8
WHERE id = $1;

-- name: SetWorkOrderAsset :exec
UPDATE work_orders SET asset_id = $2 WHERE id = $1;

-- name: ListAssetCodes :many
SELECT code::text AS code, name FROM assets WHERE active ORDER BY code LIMIT 3000;

-- name: ListWorkOrdersByAsset :many
SELECT w.id, w.ref, w.type, w.status, w.priority, w.planned_date, w.actual_date, w.description,
       COALESCE(c.name, '')::text AS contractor_name, COALESCE(c.name_en, '')::text AS contractor_name_en,
       COALESCE((SELECT i.score FROM inspections i WHERE i.work_order_id = w.id AND i.stage = 'after' AND i.status = 'submitted'
                 ORDER BY i.attempt DESC LIMIT 1), -1)::double precision AS after_score,
       COALESCE((SELECT i.passed FROM inspections i WHERE i.work_order_id = w.id AND i.stage = 'after' AND i.status = 'submitted'
                 ORDER BY i.attempt DESC LIMIT 1), false)::boolean AS after_passed,
       (SELECT count(*) FROM findings f WHERE f.work_order_id = w.id AND f.status <> 'closed')::int AS open_findings
FROM work_orders w LEFT JOIN contractors c ON c.id = w.contractor_id
WHERE w.asset_id = $1 AND (sqlc.narg(contractor_id)::bigint IS NULL OR w.contractor_id = sqlc.narg(contractor_id))
ORDER BY w.created_at DESC
LIMIT 200;

-- name: ListFindingsByAsset :many
SELECT f.id, f.status, f.severity, f.description, f.due_date, f.created_at, f.repeat_of, w.ref AS wo_ref, w.id AS work_order_id,
       COALESCE(c.code, '')::text AS criterion_code
FROM findings f JOIN work_orders w ON w.id = f.work_order_id
LEFT JOIN template_criteria c ON c.id = f.criterion_id
WHERE w.asset_id = $1 AND (sqlc.narg(contractor_id)::bigint IS NULL OR w.contractor_id = sqlc.narg(contractor_id))
ORDER BY f.created_at DESC
LIMIT 200;
