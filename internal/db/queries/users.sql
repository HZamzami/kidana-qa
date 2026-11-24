-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: ListUsers :many
SELECT * FROM users ORDER BY active DESC, name;

-- name: CreateUser :one
INSERT INTO users (email, name, role, password_hash, contractor_id)
VALUES ($1, $2, $3, $4, sqlc.narg(contractor_id))
RETURNING *;

-- name: UpdateUser :one
UPDATE users SET email = $2, name = $3, role = $4, active = $5, contractor_id = sqlc.narg(contractor_id), updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: SetUserLang :exec
UPDATE users SET lang = $2 WHERE id = $1;

-- name: SetTOTP :exec
UPDATE users SET totp_secret = $2, totp_enabled = $3, recovery_codes = $4, updated_at = now() WHERE id = $1;

-- name: SetRecoveryCodes :exec
UPDATE users SET recovery_codes = $2 WHERE id = $1;

-- name: GetUserByOIDCSubject :one
SELECT * FROM users WHERE oidc_subject = $1;

-- name: SetOIDCSubject :exec
UPDATE users SET oidc_subject = $2 WHERE id = $1;

-- Signs out every session of the user (sessions carry the version they started with).
-- name: BumpSessionVersion :exec
UPDATE users SET session_version = session_version + 1 WHERE id = $1;

-- name: ListAdmins :many
SELECT * FROM users WHERE role = 'admin' AND active;

-- name: CreatePasswordResetRequest :exec
INSERT INTO password_reset_requests (user_id) VALUES ($1);

-- name: ListOpenPasswordResetRequests :many
SELECT r.id, r.created_at, u.id AS user_id, u.name, u.email
FROM password_reset_requests r JOIN users u ON u.id = r.user_id
WHERE r.handled_at IS NULL
ORDER BY r.created_at;

-- name: HandlePasswordResetRequests :exec
UPDATE password_reset_requests SET handled_at = now(), handled_by = $2 WHERE user_id = $1 AND handled_at IS NULL;

-- name: CreatePasswordReset :exec
INSERT INTO password_resets (user_id, token_hash, expires_at, created_by) VALUES ($1, $2, $3, $4);

-- name: GetPasswordReset :one
SELECT r.*, u.name, u.email, u.active
FROM password_resets r JOIN users u ON u.id = r.user_id
WHERE r.token_hash = $1;

-- name: UsePasswordReset :execrows
UPDATE password_resets SET used_at = now() WHERE id = $1 AND used_at IS NULL AND expires_at > now();

-- Active users of a contractor (the contractor portal).
-- name: ListContractorUsers :many
SELECT * FROM users WHERE contractor_id = $1 AND active ORDER BY name;
