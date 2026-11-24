-- ---------------------------------------------------------------- sites
-- name: ListSites :many
SELECT * FROM sites ORDER BY active DESC, sort, name;

-- name: ListActiveSites :many
SELECT * FROM sites WHERE active ORDER BY sort, name;

-- name: GetSiteByName :one
SELECT * FROM sites WHERE name = $1;

-- name: CreateSite :one
INSERT INTO sites (name, name_en, sort) VALUES (sqlc.arg(name), sqlc.arg(name_en), sqlc.arg(sort)) RETURNING *;

-- name: UpdateSite :one
UPDATE sites SET name = sqlc.arg(name), name_en = sqlc.arg(name_en), sort = sqlc.arg(sort), active = sqlc.arg(active)
WHERE id = sqlc.arg(id) RETURNING *;

-- ---------------------------------------------------------------- systems
-- name: ListSystems :many
SELECT * FROM systems ORDER BY active DESC, sort, name;

-- name: ListActiveSystems :many
SELECT * FROM systems WHERE active ORDER BY sort, name;

-- name: GetSystemByName :one
SELECT * FROM systems WHERE name = $1;

-- name: CreateSystem :one
INSERT INTO systems (name, name_en, sort) VALUES (sqlc.arg(name), sqlc.arg(name_en), sqlc.arg(sort)) RETURNING *;

-- name: UpdateSystem :one
UPDATE systems SET name = sqlc.arg(name), name_en = sqlc.arg(name_en), sort = sqlc.arg(sort), active = sqlc.arg(active)
WHERE id = sqlc.arg(id) RETURNING *;

-- ---------------------------------------------------------------- asset_groups
-- name: ListAssetGroups :many
SELECT * FROM asset_groups ORDER BY active DESC, sort, name;

-- name: ListActiveAssetGroups :many
SELECT * FROM asset_groups WHERE active ORDER BY sort, name;

-- name: GetAssetGroupByName :one
SELECT * FROM asset_groups WHERE name = $1;

-- name: CreateAssetGroup :one
INSERT INTO asset_groups (name, name_en, sort) VALUES (sqlc.arg(name), sqlc.arg(name_en), sqlc.arg(sort)) RETURNING *;

-- name: UpdateAssetGroup :one
UPDATE asset_groups SET name = sqlc.arg(name), name_en = sqlc.arg(name_en), sort = sqlc.arg(sort), active = sqlc.arg(active)
WHERE id = sqlc.arg(id) RETURNING *;

-- ---------------------------------------------------------------- contractors
-- name: ListContractors :many
SELECT * FROM contractors ORDER BY active DESC, name;

-- name: ListActiveContractors :many
SELECT * FROM contractors WHERE active ORDER BY name;

-- name: GetContractorByName :one
SELECT * FROM contractors WHERE name = $1;

-- name: CreateContractor :one
INSERT INTO contractors (name, name_en, contact_name, phone)
VALUES (sqlc.arg(name), sqlc.arg(name_en), sqlc.arg(contact_name), sqlc.arg(phone)) RETURNING *;

-- name: UpdateContractor :one
UPDATE contractors SET name = sqlc.arg(name), name_en = sqlc.arg(name_en), contact_name = sqlc.arg(contact_name),
    phone = sqlc.arg(phone), active = sqlc.arg(active)
WHERE id = sqlc.arg(id) RETURNING *;
