-- The general checklist of a stage (no system or asset group).
-- name: GetActiveTemplate :one
SELECT t.* FROM checklist_templates t
WHERE t.stage = $1 AND t.active AND t.asset_group_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM checklist_template_systems s WHERE s.template_id = t.id)
ORDER BY t.id DESC LIMIT 1;

-- The checklist for a new inspection: the most specific active one that
-- matches the work order (asset group, then system, then general; fewer
-- systems is more specific).
-- name: PickActiveTemplate :one
SELECT t.* FROM checklist_templates t
WHERE t.stage = sqlc.arg(stage) AND t.active
  AND (t.asset_group_id IS NULL OR t.asset_group_id = sqlc.arg(asset_group_id)::bigint)
  AND (NOT EXISTS (SELECT 1 FROM checklist_template_systems s WHERE s.template_id = t.id)
       OR EXISTS (SELECT 1 FROM checklist_template_systems s WHERE s.template_id = t.id AND s.system_id = sqlc.arg(system_id)::bigint))
ORDER BY (t.asset_group_id IS NOT NULL) DESC,
         EXISTS (SELECT 1 FROM checklist_template_systems s WHERE s.template_id = t.id) DESC,
         (SELECT count(*) FROM checklist_template_systems s WHERE s.template_id = t.id) ASC,
         t.id DESC
LIMIT 1;

-- Scopes of the active checklists of a stage, to refuse overlaps.
-- name: ListActiveScopes :many
SELECT t.id, t.family_id, t.name, t.asset_group_id,
       COALESCE((SELECT array_agg(s.system_id ORDER BY s.system_id) FROM checklist_template_systems s WHERE s.template_id = t.id), '{}')::bigint[] AS system_ids
FROM checklist_templates t
WHERE t.stage = $1 AND t.active;

-- name: ListTemplateSystems :many
SELECT sy.id, sy.name, sy.name_en FROM checklist_template_systems s JOIN systems sy ON sy.id = s.system_id
WHERE s.template_id = $1 ORDER BY sy.name;

-- name: AddTemplateSystem :exec
INSERT INTO checklist_template_systems (template_id, system_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- The newest version of a checklist.
-- name: GetFamilyLatest :one
SELECT * FROM checklist_templates WHERE family_id = $1 ORDER BY version DESC LIMIT 1;

-- name: GetTemplate :one
SELECT * FROM checklist_templates WHERE id = $1;

-- name: ListTemplates :many
SELECT t.*, (SELECT count(*) FROM inspections i WHERE i.template_id = t.id) AS inspections_count,
       COALESCE((SELECT string_agg(sy.name, '، ' ORDER BY sy.name) FROM checklist_template_systems s JOIN systems sy ON sy.id = s.system_id WHERE s.template_id = t.id), '')::text AS system_names,
       COALESCE((SELECT string_agg(COALESCE(NULLIF(sy.name_en, ''), sy.name), ', ' ORDER BY sy.name) FROM checklist_template_systems s JOIN systems sy ON sy.id = s.system_id WHERE s.template_id = t.id), '')::text AS system_names_en,
       COALESCE(ag.name, '')::text AS asset_group_name, COALESCE(ag.name_en, '')::text AS asset_group_name_en
FROM checklist_templates t
LEFT JOIN asset_groups ag ON ag.id = t.asset_group_id
ORDER BY t.stage, t.family_id, t.version DESC;

-- name: ListCriteria :many
SELECT * FROM template_criteria WHERE template_id = $1 ORDER BY sort, code;

-- name: NextTemplateVersion :one
SELECT (COALESCE(max(version), 0) + 1)::int FROM checklist_templates WHERE family_id = $1;

-- name: NewTemplateID :one
SELECT nextval(pg_get_serial_sequence('checklist_templates', 'id'))::bigint;

-- A new checklist family uses its first version's id as family_id.
-- name: CreateTemplate :one
INSERT INTO checklist_templates (id, stage, version, name, active, created_by, asset_group_id, family_id)
OVERRIDING SYSTEM VALUE
VALUES (sqlc.arg(id), sqlc.arg(stage), sqlc.arg(version), sqlc.arg(name), false, sqlc.arg(created_by), sqlc.narg(asset_group_id), sqlc.arg(family_id))
RETURNING *;

-- name: CreateCriterion :one
INSERT INTO template_criteria (template_id, code, label, description, weight, is_critical, sort, label_en, description_en)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- Deactivates the active version of a checklist.
-- name: DeactivateFamily :exec
UPDATE checklist_templates SET active = false WHERE family_id = $1 AND active;

-- name: ActivateTemplate :exec
UPDATE checklist_templates SET active = true WHERE id = $1;

-- name: GetAssetGroup :one
SELECT * FROM asset_groups WHERE id = $1;
