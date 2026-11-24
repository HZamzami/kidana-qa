-- +goose Up
-- Optional two-factor sign-in (TOTP), Microsoft sign-in (OIDC), session
-- invalidation after a password change, and password resets by one-time link.

ALTER TABLE users
    ADD COLUMN totp_secret     text    NOT NULL DEFAULT '',
    ADD COLUMN totp_enabled    boolean NOT NULL DEFAULT false,
    ADD COLUMN recovery_codes  text[]  NOT NULL DEFAULT '{}',  -- SHA-256 hashes, each usable once
    ADD COLUMN oidc_subject    text,
    ADD COLUMN session_version integer NOT NULL DEFAULT 1;      -- bumped to sign out every session
CREATE UNIQUE INDEX users_oidc_subject_uq ON users (oidc_subject) WHERE oidc_subject IS NOT NULL;

-- «نسيت كلمة المرور» requests, handled by an admin.
CREATE TABLE password_reset_requests (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    handled_at timestamptz,
    handled_by bigint      REFERENCES users (id)
);
CREATE INDEX password_reset_requests_open_idx ON password_reset_requests (user_id) WHERE handled_at IS NULL;

-- One-time reset links issued by an admin. Only the token's SHA-256 is kept.
CREATE TABLE password_resets (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash text        NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_by bigint      REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE password_resets, password_reset_requests;
DROP INDEX users_oidc_subject_uq;
ALTER TABLE users DROP COLUMN session_version, DROP COLUMN oidc_subject, DROP COLUMN recovery_codes,
                  DROP COLUMN totp_enabled, DROP COLUMN totp_secret;
