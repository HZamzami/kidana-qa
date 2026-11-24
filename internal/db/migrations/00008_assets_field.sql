-- +goose Up
-- Asset register (with QR labels), signatures, and proof of presence on
-- inspections and photos.

CREATE TABLE assets (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code           citext      NOT NULL UNIQUE,   -- the CMMS asset code, printed on the QR label
    name           text        NOT NULL DEFAULT '',
    name_en        text        NOT NULL DEFAULT '',
    site_id        bigint      REFERENCES sites (id),
    system_id      bigint      REFERENCES systems (id),
    asset_group_id bigint      REFERENCES asset_groups (id),
    location       text        NOT NULL DEFAULT '',
    active         boolean     NOT NULL DEFAULT true,
    created_at     timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE work_orders ADD COLUMN asset_id bigint REFERENCES assets (id);
CREATE INDEX work_orders_asset_idx ON work_orders (asset_id);

-- Existing work orders: one asset per distinct code, details from the latest work order.
INSERT INTO assets (code, site_id, system_id, asset_group_id, location)
SELECT DISTINCT ON (lower(asset)) asset, site_id, system_id, asset_group_id, location
FROM work_orders WHERE asset <> ''
ORDER BY lower(asset), created_at DESC;
UPDATE work_orders w SET asset_id = a.id FROM assets a WHERE w.asset <> '' AND lower(w.asset) = lower(a.code::text);

ALTER TABLE attachments
    ADD COLUMN kind         text NOT NULL DEFAULT 'photo' CHECK (kind IN ('photo', 'signature')),
    ADD COLUMN signer_name  text NOT NULL DEFAULT '',
    ADD COLUMN signer_role  text NOT NULL DEFAULT '',       -- inspector | contractor_rep
    ADD COLUMN lat          double precision,
    ADD COLUMN lng          double precision,
    ADD COLUMN accuracy     real,                           -- metres
    ADD COLUMN captured_at  timestamptz,                    -- device time when taken
    ADD COLUMN annotated_of bigint REFERENCES attachments (id) ON DELETE CASCADE;

-- Where the inspector was when the inspection started.
ALTER TABLE inspections
    ADD COLUMN lat        double precision,
    ADD COLUMN lng        double precision,
    ADD COLUMN accuracy   real,
    ADD COLUMN located_at timestamptz;

-- +goose Down
ALTER TABLE inspections DROP COLUMN located_at, DROP COLUMN accuracy, DROP COLUMN lng, DROP COLUMN lat;
ALTER TABLE attachments DROP COLUMN annotated_of, DROP COLUMN captured_at, DROP COLUMN accuracy, DROP COLUMN lng,
    DROP COLUMN lat, DROP COLUMN signer_role, DROP COLUMN signer_name, DROP COLUMN kind;
DROP INDEX work_orders_asset_idx;
ALTER TABLE work_orders DROP COLUMN asset_id;
DROP TABLE assets;
