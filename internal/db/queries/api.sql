-- name: CreateAPIToken :one
INSERT INTO api_tokens (name, token_hash, created_by) VALUES ($1, $2, $3) RETURNING id;

-- name: ListAPITokens :many
SELECT t.*, COALESCE(u.name, '')::text AS created_by_name FROM api_tokens t LEFT JOIN users u ON u.id = t.created_by
ORDER BY t.revoked_at IS NOT NULL, t.created_at DESC;

-- name: RevokeAPIToken :exec
UPDATE api_tokens SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL;

-- name: UseAPIToken :one
UPDATE api_tokens SET last_used_at = now() WHERE token_hash = $1 AND revoked_at IS NULL RETURNING id, name;

-- Work orders for the API: names, latest scores and open findings.
-- name: ListWorkOrdersAPI :many
SELECT w.id, w.ref, w.type, w.status, w.priority, w.location, w.asset, w.technician, w.description,
       w.reported_on, w.planned_date, w.actual_date, w.updated_at,
       s.name AS site, sy.name AS system, ag.name AS asset_group, COALESCE(c.name, '')::text AS contractor,
       COALESCE(dur.score, -1)::double precision AS during_score,
       COALESCE(aft.score, -1)::double precision AS after_score,
       COALESCE(aft.passed, false)::boolean AS after_passed,
       (SELECT count(*) FROM findings f WHERE f.work_order_id = w.id AND f.status <> 'closed')::int AS open_findings
FROM work_orders w
JOIN sites s ON s.id = w.site_id
JOIN systems sy ON sy.id = w.system_id
JOIN asset_groups ag ON ag.id = w.asset_group_id
LEFT JOIN contractors c ON c.id = w.contractor_id
LEFT JOIN LATERAL (SELECT i.score FROM inspections i WHERE i.work_order_id = w.id AND i.stage = 'during' AND i.status = 'submitted'
                   ORDER BY i.attempt DESC LIMIT 1) dur ON true
LEFT JOIN LATERAL (SELECT i.score, i.passed FROM inspections i WHERE i.work_order_id = w.id AND i.stage = 'after' AND i.status = 'submitted'
                   ORDER BY i.attempt DESC LIMIT 1) aft ON true
WHERE (sqlc.narg(ref)::text IS NULL OR lower(w.ref) = lower(sqlc.narg(ref)))
  AND (sqlc.narg(updated_since)::timestamptz IS NULL OR w.updated_at >= sqlc.narg(updated_since))
ORDER BY w.updated_at, w.id
LIMIT sqlc.arg(lim);

-- An API update changes the work order fields, not its quality status.
-- name: UpdateWorkOrderAPI :one
UPDATE work_orders SET type = $2, site_id = $3, location = $4, system_id = $5, asset_group_id = $6, asset = $7,
    contractor_id = $8, technician = $9, priority = $10, description = $11, planned_date = $12, actual_date = $13,
    reported_on = COALESCE(sqlc.narg(reported_on)::date, reported_on), updated_at = now()
WHERE id = $1
RETURNING *;
