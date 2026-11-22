-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;

-- ---------------------------------------------------------------- users & sessions
CREATE TYPE user_role AS ENUM ('admin', 'inspector', 'viewer');

CREATE TABLE users (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email         citext      NOT NULL UNIQUE,
    name          text        NOT NULL,
    role          user_role   NOT NULL DEFAULT 'inspector',
    password_hash text        NOT NULL,
    active        boolean     NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Schema required by github.com/alexedwards/scs/pgxstore
CREATE TABLE sessions (
    token  text        PRIMARY KEY,
    data   bytea       NOT NULL,
    expiry timestamptz NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions (expiry);

-- ---------------------------------------------------------------- lookups
CREATE TABLE sites (
    id     bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name   text    NOT NULL UNIQUE,
    active boolean NOT NULL DEFAULT true,
    sort   integer NOT NULL DEFAULT 0
);

CREATE TABLE systems (
    id     bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name   text    NOT NULL UNIQUE,
    active boolean NOT NULL DEFAULT true,
    sort   integer NOT NULL DEFAULT 0
);

CREATE TABLE contractors (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name         text    NOT NULL UNIQUE,
    contact_name text    NOT NULL DEFAULT '',
    phone        text    NOT NULL DEFAULT '',
    active       boolean NOT NULL DEFAULT true
);

-- ---------------------------------------------------------------- work orders
CREATE TYPE wo_type     AS ENUM ('PM', 'WO');
CREATE TYPE wo_priority AS ENUM ('emergency', 'high', 'medium', 'low');
-- accepted = latest QC passed and no open findings (maintained by the app)
CREATE TYPE wo_status   AS ENUM ('open', 'in_progress', 'completed', 'accepted', 'cancelled');

CREATE TABLE work_orders (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ref           text        NOT NULL,              -- CMMS work order / PM number
    type          wo_type     NOT NULL,
    site_id       bigint      NOT NULL REFERENCES sites (id),
    location      text        NOT NULL DEFAULT '',   -- building / floor / room
    system_id     bigint      NOT NULL REFERENCES systems (id),
    asset         text        NOT NULL DEFAULT '',
    contractor_id bigint      REFERENCES contractors (id),
    technician    text        NOT NULL DEFAULT '',
    priority      wo_priority,
    description   text        NOT NULL DEFAULT '',
    planned_date  date,                              -- PM schedule date or WO SLA due date
    actual_date   date,                              -- date the work was actually done
    status        wo_status   NOT NULL DEFAULT 'open',
    created_by    bigint      REFERENCES users (id),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX work_orders_ref_uq ON work_orders (lower(ref));
CREATE INDEX work_orders_site_idx       ON work_orders (site_id);
CREATE INDEX work_orders_contractor_idx ON work_orders (contractor_id);
CREATE INDEX work_orders_status_idx     ON work_orders (status);

-- ---------------------------------------------------------------- checklists (versioned)
CREATE TYPE inspection_stage AS ENUM ('qa', 'qc');

CREATE TABLE checklist_templates (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    stage      inspection_stage NOT NULL,
    version    integer     NOT NULL,
    name       text        NOT NULL,
    active     boolean     NOT NULL DEFAULT false,
    created_by bigint      REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (stage, version)
);
-- exactly one active template per stage is used for new inspections
CREATE UNIQUE INDEX checklist_templates_active_uq ON checklist_templates (stage) WHERE active;

CREATE TABLE template_criteria (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    template_id bigint  NOT NULL REFERENCES checklist_templates (id) ON DELETE CASCADE,
    code        text    NOT NULL,
    label       text    NOT NULL,
    description text    NOT NULL DEFAULT '',
    weight      integer NOT NULL CHECK (weight > 0),
    is_critical boolean NOT NULL DEFAULT false,      -- a fail here fails the whole inspection
    sort        integer NOT NULL DEFAULT 0,
    UNIQUE (template_id, code)
);

-- ---------------------------------------------------------------- inspections
CREATE TYPE inspection_status AS ENUM ('draft', 'submitted');
CREATE TYPE rating            AS ENUM ('C', 'P', 'N', 'NA');

CREATE TABLE inspections (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    work_order_id   bigint            NOT NULL REFERENCES work_orders (id) ON DELETE CASCADE,
    stage           inspection_stage  NOT NULL,
    attempt         integer           NOT NULL DEFAULT 1,
    template_id     bigint            NOT NULL REFERENCES checklist_templates (id),
    inspector_id    bigint            NOT NULL REFERENCES users (id),
    status          inspection_status NOT NULL DEFAULT 'draft',
    inspected_on    date              NOT NULL DEFAULT CURRENT_DATE,
    submitted_at    timestamptz,
    score           double precision,                 -- set on submit
    passed          boolean,                          -- set on submit
    critical_failed boolean           NOT NULL DEFAULT false,
    notes           text              NOT NULL DEFAULT '',
    created_at      timestamptz       NOT NULL DEFAULT now(),
    updated_at      timestamptz       NOT NULL DEFAULT now(),
    UNIQUE (work_order_id, stage, attempt)
);
CREATE INDEX inspections_wo_idx ON inspections (work_order_id);
CREATE INDEX inspections_inspected_on_idx ON inspections (inspected_on);

CREATE TABLE inspection_items (
    inspection_id bigint NOT NULL REFERENCES inspections (id) ON DELETE CASCADE,
    criterion_id  bigint NOT NULL REFERENCES template_criteria (id),
    rating        rating,
    note          text   NOT NULL DEFAULT '',
    PRIMARY KEY (inspection_id, criterion_id)
);

-- ---------------------------------------------------------------- findings (NCRs)
CREATE TYPE finding_status   AS ENUM ('open', 'in_progress', 'closed');
CREATE TYPE finding_severity AS ENUM ('normal', 'critical');

CREATE TABLE findings (
    id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    work_order_id     bigint           NOT NULL REFERENCES work_orders (id) ON DELETE CASCADE,
    inspection_id     bigint           REFERENCES inspections (id) ON DELETE SET NULL,
    criterion_id      bigint           REFERENCES template_criteria (id),
    severity          finding_severity NOT NULL DEFAULT 'normal',
    description       text             NOT NULL,
    corrective_action text             NOT NULL,
    owner             text             NOT NULL DEFAULT '',
    due_date          date,
    status            finding_status   NOT NULL DEFAULT 'open',
    closure_note      text             NOT NULL DEFAULT '',
    closed_at         timestamptz,
    closed_by         bigint           REFERENCES users (id),
    created_by        bigint           REFERENCES users (id),
    created_at        timestamptz      NOT NULL DEFAULT now(),
    updated_at        timestamptz      NOT NULL DEFAULT now()
);
CREATE INDEX findings_wo_idx ON findings (work_order_id);
CREATE INDEX findings_open_idx ON findings (status) WHERE status <> 'closed';

-- ---------------------------------------------------------------- attachments (photos)
CREATE TABLE attachments (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    inspection_id bigint REFERENCES inspections (id) ON DELETE CASCADE,
    criterion_id  bigint REFERENCES template_criteria (id),
    finding_id    bigint REFERENCES findings (id) ON DELETE CASCADE,
    storage_key   text        NOT NULL UNIQUE,
    filename      text        NOT NULL,
    content_type  text        NOT NULL,
    size_bytes    bigint      NOT NULL,
    uploaded_by   bigint      REFERENCES users (id),
    created_at    timestamptz NOT NULL DEFAULT now(),
    CHECK (inspection_id IS NOT NULL OR finding_id IS NOT NULL)
);
CREATE INDEX attachments_inspection_idx ON attachments (inspection_id);
CREATE INDEX attachments_finding_idx ON attachments (finding_id);

-- ---------------------------------------------------------------- audit log
CREATE TABLE audit_log (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint      REFERENCES users (id),
    entity     text        NOT NULL,
    entity_id  bigint      NOT NULL,
    action     text        NOT NULL,
    details    jsonb       NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_entity_idx ON audit_log (entity, entity_id);

-- +goose Down
DROP TABLE audit_log, attachments, findings, inspection_items, inspections,
           template_criteria, checklist_templates, work_orders, contractors,
           systems, sites, sessions, users;
DROP TYPE finding_severity, finding_status, rating, inspection_status, inspection_stage,
          wo_status, wo_priority, wo_type, user_role;
