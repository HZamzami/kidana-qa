-- +goose Up
-- Resolution targets per priority, saved filter views, scheduled report
-- runs, and app settings (scorecard weights).

-- Days allowed to resolve a work order by priority, and to close a finding by
-- severity. Defaults to confirm with the business; admins can change them.
CREATE TABLE sla_targets (
    key  text PRIMARY KEY CHECK (key IN ('normal', 'high', 'emergency', 'finding_normal', 'finding_critical')),
    days integer NOT NULL CHECK (days BETWEEN 0 AND 365)
);
INSERT INTO sla_targets (key, days) VALUES
    ('emergency', 1), ('high', 3), ('normal', 7), ('finding_critical', 2), ('finding_normal', 7);

-- When the job was reported; the due date defaults from it and the priority.
ALTER TABLE work_orders ADD COLUMN reported_on date NOT NULL DEFAULT CURRENT_DATE;
UPDATE work_orders SET reported_on = created_at::date;

CREATE TABLE saved_views (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    page       text        NOT NULL CHECK (page IN ('work-orders', 'findings', 'dashboard')),
    name       text        NOT NULL,
    query      text        NOT NULL DEFAULT '',
    shared     boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX saved_views_user_idx ON saved_views (user_id, page);

-- One row per generated periodic report (the monthly quality report).
CREATE TABLE report_runs (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind       text        NOT NULL DEFAULT 'monthly',
    period     date        NOT NULL,  -- first day of the month covered
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (kind, period)
);

CREATE TABLE settings (
    key   text PRIMARY KEY,
    value text NOT NULL
);
-- Contractor scorecard: weights of the composite score (sum 100).
INSERT INTO settings (key, value) VALUES ('scorecard_weights', '{"after":40,"during":20,"sla":20,"findings":20}');

-- +goose Down
DROP TABLE settings, report_runs, saved_views;
ALTER TABLE work_orders DROP COLUMN reported_on;
DROP TABLE sla_targets;
