-- name: GetActiveTemplate :one
SELECT * FROM checklist_templates WHERE stage = $1 AND active AND system_id IS NULL AND asset_group_id IS NULL;

-- The checklist for a new inspection: the most specific active one that
-- matches the work order (asset group, then system, then general).
-- name: PickActiveTemplate :one
SELECT * FROM checklist_templates
WHERE stage = sqlc.arg(stage) AND active
  AND (system_id IS NULL OR system_id = sqlc.arg(system_id)::bigint)
  AND (asset_group_id IS NULL OR asset_group_id = sqlc.arg(asset_group_id)::bigint)
ORDER BY (asset_group_id IS NOT NULL) DESC, (system_id IS NOT NULL) DESC
LIMIT 1;

-- name: GetTemplate :one
SELECT * FROM checklist_templates WHERE id = $1;

-- name: ListTemplates :many
SELECT t.*, (SELECT count(*) FROM inspections i WHERE i.template_id = t.id) AS inspections_count,
       COALESCE(sy.name, '')::text AS system_name, COALESCE(sy.name_en, '')::text AS system_name_en,
       COALESCE(ag.name, '')::text AS asset_group_name, COALESCE(ag.name_en, '')::text AS asset_group_name_en
FROM checklist_templates t
LEFT JOIN systems sy ON sy.id = t.system_id
LEFT JOIN asset_groups ag ON ag.id = t.asset_group_id
ORDER BY t.stage, t.active DESC, t.version DESC;

-- name: ListCriteria :many
SELECT * FROM template_criteria WHERE template_id = $1 ORDER BY sort, code;

-- name: NextTemplateVersion :one
SELECT (COALESCE(max(version), 0) + 1)::int FROM checklist_templates WHERE stage = $1;

-- name: CreateTemplate :one
INSERT INTO checklist_templates (stage, version, name, active, created_by, system_id, asset_group_id)
VALUES ($1, $2, $3, false, $4, sqlc.narg(system_id), sqlc.narg(asset_group_id))
RETURNING *;

-- name: CreateCriterion :one
INSERT INTO template_criteria (template_id, code, label, description, weight, is_critical, sort, label_en, description_en)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- Deactivates the active checklist of the same stage and scope.
-- name: DeactivateStageTemplates :exec
UPDATE checklist_templates SET active = false
WHERE stage = sqlc.arg(stage) AND active
  AND system_id IS NOT DISTINCT FROM sqlc.narg(system_id)::bigint
  AND asset_group_id IS NOT DISTINCT FROM sqlc.narg(asset_group_id)::bigint;

-- name: ActivateTemplate :exec
UPDATE checklist_templates SET active = true WHERE id = $1;
