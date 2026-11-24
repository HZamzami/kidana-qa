-- +goose Up
-- A checklist can be specific to a system (المنظومة) or an asset group
-- (مجموعة الأصل). A new inspection uses the most specific active checklist
-- for its stage: asset group, then system, then the general one.
ALTER TABLE checklist_templates
    ADD COLUMN system_id      bigint REFERENCES systems (id),
    ADD COLUMN asset_group_id bigint REFERENCES asset_groups (id);

DROP INDEX checklist_templates_active_uq;
CREATE UNIQUE INDEX checklist_templates_active_uq
    ON checklist_templates (stage, COALESCE(system_id, 0), COALESCE(asset_group_id, 0)) WHERE active;

-- +goose Down
DROP INDEX checklist_templates_active_uq;
DELETE FROM checklist_templates WHERE active AND (system_id IS NOT NULL OR asset_group_id IS NOT NULL);
CREATE UNIQUE INDEX checklist_templates_active_uq ON checklist_templates (stage) WHERE active;
ALTER TABLE checklist_templates DROP COLUMN asset_group_id, DROP COLUMN system_id;
