-- ---------------------------------------------------------------- sites
-- name: ListSites :many
SELECT * FROM sites ORDER BY active DESC, sort, name;

-- name: ListActiveSites :many
SELECT * FROM sites WHERE active ORDER BY sort, name;

-- name: GetSiteByName :one
SELECT * FROM sites WHERE name = $1;

-- name: CreateSite :one
INSERT INTO sites (name, sort) VALUES ($1, $2) RETURNING *;

-- name: UpdateSite :one
UPDATE sites SET name = $2, sort = $3, active = $4 WHERE id = $1 RETURNING *;

-- ---------------------------------------------------------------- systems
-- name: ListSystems :many
SELECT * FROM systems ORDER BY active DESC, sort, name;

-- name: ListActiveSystems :many
SELECT * FROM systems WHERE active ORDER BY sort, name;

-- name: GetSystemByName :one
SELECT * FROM systems WHERE name = $1;

-- name: CreateSystem :one
INSERT INTO systems (name, sort) VALUES ($1, $2) RETURNING *;

-- name: UpdateSystem :one
UPDATE systems SET name = $2, sort = $3, active = $4 WHERE id = $1 RETURNING *;

-- ---------------------------------------------------------------- asset_groups
-- name: ListAssetGroups :many
SELECT * FROM asset_groups ORDER BY active DESC, sort, name;

-- name: ListActiveAssetGroups :many
SELECT * FROM asset_groups WHERE active ORDER BY sort, name;

-- name: GetAssetGroupByName :one
SELECT * FROM asset_groups WHERE name = $1;

-- name: CreateAssetGroup :one
INSERT INTO asset_groups (name, sort) VALUES ($1, $2) RETURNING *;

-- name: UpdateAssetGroup :one
UPDATE asset_groups SET name = $2, sort = $3, active = $4 WHERE id = $1 RETURNING *;

-- ---------------------------------------------------------------- contractors
-- name: ListContractors :many
SELECT * FROM contractors ORDER BY active DESC, name;

-- name: ListActiveContractors :many
SELECT * FROM contractors WHERE active ORDER BY name;

-- name: GetContractorByName :one
SELECT * FROM contractors WHERE name = $1;

-- name: CreateContractor :one
INSERT INTO contractors (name, contact_name, phone) VALUES ($1, $2, $3) RETURNING *;

-- name: UpdateContractor :one
UPDATE contractors SET name = $2, contact_name = $3, phone = $4, active = $5 WHERE id = $1 RETURNING *;
