-- +goose Up
-- A checklist (e.g. "after-work, camps") is a family of versions. Editing
-- makes a new version in the same family; a new checklist starts a new
-- family with its own scope and version 1. A checklist can cover several
-- systems (المنظومات), e.g. the three camp systems.
ALTER TABLE checklist_templates ADD COLUMN family_id bigint REFERENCES checklist_templates (id);

UPDATE checklist_templates t SET family_id = f.first_id
FROM (SELECT id, min(id) OVER (PARTITION BY stage, COALESCE(system_id, 0), COALESCE(asset_group_id, 0)) AS first_id
      FROM checklist_templates) f
WHERE f.id = t.id;

ALTER TABLE checklist_templates ALTER COLUMN family_id SET NOT NULL;

CREATE TABLE checklist_template_systems (
    template_id bigint NOT NULL REFERENCES checklist_templates (id) ON DELETE CASCADE,
    system_id   bigint NOT NULL REFERENCES systems (id),
    PRIMARY KEY (template_id, system_id)
);
INSERT INTO checklist_template_systems (template_id, system_id)
SELECT id, system_id FROM checklist_templates WHERE system_id IS NOT NULL;

-- Versions are numbered within each family.
ALTER TABLE checklist_templates DROP CONSTRAINT checklist_templates_stage_version_key;
UPDATE checklist_templates t SET version = v.n
FROM (SELECT id, row_number() OVER (PARTITION BY family_id ORDER BY id) AS n FROM checklist_templates) v
WHERE v.id = t.id;

-- Names carry their version ("… — الإصدار 2"); keep them in step.
UPDATE checklist_templates SET name = regexp_replace(name, ' — الإصدار [0-9]+$', '') || ' — الإصدار ' || version;

DROP INDEX checklist_templates_active_uq;
ALTER TABLE checklist_templates DROP COLUMN system_id;
-- One active version per checklist. Overlapping scopes between checklists
-- are refused by the app (domain.ScopesOverlap).
CREATE UNIQUE INDEX checklist_templates_active_uq ON checklist_templates (family_id) WHERE active;
ALTER TABLE checklist_templates ADD CONSTRAINT checklist_templates_family_version_key UNIQUE (family_id, version);

-- +goose Down
DROP INDEX checklist_templates_active_uq;
ALTER TABLE checklist_templates DROP CONSTRAINT checklist_templates_family_version_key;
UPDATE checklist_templates t SET version = v.n
FROM (SELECT id, row_number() OVER (PARTITION BY stage ORDER BY id) AS n FROM checklist_templates) v
WHERE v.id = t.id;
ALTER TABLE checklist_templates ADD CONSTRAINT checklist_templates_stage_version_key UNIQUE (stage, version);
ALTER TABLE checklist_templates ADD COLUMN system_id bigint REFERENCES systems (id);
UPDATE checklist_templates t SET system_id = (SELECT min(system_id) FROM checklist_template_systems s WHERE s.template_id = t.id);
DROP TABLE checklist_template_systems;
ALTER TABLE checklist_templates DROP COLUMN family_id;
CREATE UNIQUE INDEX checklist_templates_active_uq
    ON checklist_templates (stage, COALESCE(system_id, 0), COALESCE(asset_group_id, 0)) WHERE active;
