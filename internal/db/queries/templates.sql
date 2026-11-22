-- name: GetActiveTemplate :one
SELECT * FROM checklist_templates WHERE stage = $1 AND active;

-- name: GetTemplate :one
SELECT * FROM checklist_templates WHERE id = $1;

-- name: ListTemplates :many
SELECT t.*, (SELECT count(*) FROM inspections i WHERE i.template_id = t.id) AS inspections_count
FROM checklist_templates t
ORDER BY t.stage, t.version DESC;

-- name: ListCriteria :many
SELECT * FROM template_criteria WHERE template_id = $1 ORDER BY sort, code;

-- name: NextTemplateVersion :one
SELECT (COALESCE(max(version), 0) + 1)::int FROM checklist_templates WHERE stage = $1;

-- name: CreateTemplate :one
INSERT INTO checklist_templates (stage, version, name, active, created_by)
VALUES ($1, $2, $3, false, $4)
RETURNING *;

-- name: CreateCriterion :one
INSERT INTO template_criteria (template_id, code, label, description, weight, is_critical, sort)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: DeactivateStageTemplates :exec
UPDATE checklist_templates SET active = false WHERE stage = $1 AND active;

-- name: ActivateTemplate :exec
UPDATE checklist_templates SET active = true WHERE id = $1;
