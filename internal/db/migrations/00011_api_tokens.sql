-- +goose Up
-- Tokens for the work order API (CMMS sync). Only the SHA-256 is stored.
CREATE TABLE api_tokens (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name         text        NOT NULL,
    token_hash   text        NOT NULL UNIQUE,
    created_by   bigint      REFERENCES users (id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at   timestamptz
);

-- +goose Down
DROP TABLE api_tokens;
