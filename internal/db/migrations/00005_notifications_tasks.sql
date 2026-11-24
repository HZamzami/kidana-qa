-- +goose Up
-- In-app notifications, inspection assignments (manual, sampled or
-- re-inspection) and the sampling rules that create them.

CREATE TABLE notifications (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind       text        NOT NULL,             -- message template, rendered in the reader's language
    params     jsonb       NOT NULL DEFAULT '{}', -- values for the template
    link       text        NOT NULL DEFAULT '',
    dedupe     text,                              -- same key for the same user = sent once
    read_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, dedupe)
);
CREATE INDEX notifications_user_idx   ON notifications (user_id, created_at DESC);
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL;

CREATE TYPE assignment_reason AS ENUM ('manual', 'sampling', 'reinspection');
CREATE TYPE assignment_status AS ENUM ('open', 'done', 'cancelled');

CREATE TABLE inspection_assignments (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    work_order_id bigint            NOT NULL REFERENCES work_orders (id) ON DELETE CASCADE,
    stage         inspection_stage  NOT NULL,
    assignee_id   bigint            REFERENCES users (id),  -- NULL = in the pool, any inspector may claim
    due_date      date,
    reason        assignment_reason NOT NULL DEFAULT 'manual',
    status        assignment_status NOT NULL DEFAULT 'open',
    note          text              NOT NULL DEFAULT '',
    inspection_id bigint            REFERENCES inspections (id) ON DELETE SET NULL,
    created_by    bigint            REFERENCES users (id),
    created_at    timestamptz       NOT NULL DEFAULT now(),
    done_at       timestamptz
);
-- one open assignment per work order and stage
CREATE UNIQUE INDEX inspection_assignments_open_uq ON inspection_assignments (work_order_id, stage) WHERE status = 'open';
CREATE INDEX inspection_assignments_assignee_idx ON inspection_assignments (assignee_id) WHERE status = 'open';

-- Which work orders get an inspection automatically. The most specific
-- active rule (type and priority, then either, then neither) decides.
CREATE TABLE sampling_rules (
    id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    stage    inspection_stage NOT NULL,
    wo_type  wo_type,      -- NULL = any type
    priority wo_priority,  -- NULL = any priority
    percent  integer  NOT NULL CHECK (percent BETWEEN 0 AND 100),
    active   boolean  NOT NULL DEFAULT true
);

-- Defaults to confirm with the business; admins can change them.
INSERT INTO sampling_rules (stage, wo_type, priority, percent) VALUES
    ('after',  NULL, 'emergency', 100),
    ('after',  NULL, 'high',      100),
    ('after',  NULL, 'normal',     50),
    ('during', NULL, 'emergency', 100),
    ('during', 'PM', NULL,         20);

-- +goose Down
DROP TABLE sampling_rules, inspection_assignments, notifications;
DROP TYPE assignment_status, assignment_reason;
