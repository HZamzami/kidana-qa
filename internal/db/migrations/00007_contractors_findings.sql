-- +goose Up
-- Contractor users respond to their findings; inspectors verify. Findings
-- also get a root cause, a link to the finding they repeat, and an owner who
-- is a real contractor user when there is one.

ALTER TABLE users ADD COLUMN contractor_id bigint REFERENCES contractors (id);
ALTER TABLE users ADD CONSTRAINT users_contractor_role_ck
    CHECK ((role = 'contractor') = (contractor_id IS NOT NULL));
CREATE INDEX users_contractor_idx ON users (contractor_id) WHERE contractor_id IS NOT NULL;

CREATE TYPE root_cause AS ENUM ('people', 'procedure', 'materials', 'equipment', 'supervision', 'design', 'external', 'other');

ALTER TABLE findings
    ADD COLUMN root_cause_category root_cause,
    ADD COLUMN root_cause          text        NOT NULL DEFAULT '',
    ADD COLUMN owner_user_id       bigint      REFERENCES users (id),
    ADD COLUMN response_note       text        NOT NULL DEFAULT '',
    ADD COLUMN responded_by        bigint      REFERENCES users (id),
    ADD COLUMN responded_at        timestamptz,
    ADD COLUMN rejection_note      text        NOT NULL DEFAULT '',
    ADD COLUMN repeat_of           bigint      REFERENCES findings (id) ON DELETE SET NULL;
CREATE INDEX findings_responded_idx ON findings (created_by) WHERE status = 'responded';

-- Attachments know what they prove: evidence on a finding, from raising,
-- responding or closing it.
ALTER TABLE attachments ADD COLUMN purpose text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE attachments DROP COLUMN purpose;
DROP INDEX findings_responded_idx;
ALTER TABLE findings DROP COLUMN repeat_of, DROP COLUMN rejection_note, DROP COLUMN responded_at,
    DROP COLUMN responded_by, DROP COLUMN response_note, DROP COLUMN owner_user_id,
    DROP COLUMN root_cause, DROP COLUMN root_cause_category;
DROP TYPE root_cause;
DROP INDEX users_contractor_idx;
ALTER TABLE users DROP CONSTRAINT users_contractor_role_ck, DROP COLUMN contractor_id;
