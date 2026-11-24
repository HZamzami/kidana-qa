-- +goose NO TRANSACTION
-- New enum values must be committed before they can be used, so they live in
-- their own migration that runs outside a transaction.

-- +goose Up
ALTER TYPE user_role ADD VALUE IF NOT EXISTS 'contractor';
ALTER TYPE finding_status ADD VALUE IF NOT EXISTS 'responded' BEFORE 'closed';

-- +goose Down
-- Postgres cannot drop enum values; the values stay unused.
SELECT 1;
