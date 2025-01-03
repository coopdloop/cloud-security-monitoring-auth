-- sql/queries/users.sql

-- name: GetUserByAuth0ID :one
SELECT * FROM users WHERE auth0_id = $1 LIMIT 1;

-- name: UpdateUser :exec
UPDATE users
SET
    name = $2,
    email = $3,
    picture = $4,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1;

-- name: ListUsers :many
SELECT
    id,
    name,
    email,
    auth0_id,
    picture,
    mfa_enabled,
    created_at,
    updated_at
FROM users
ORDER BY name;

-- name: GetUser :one
SELECT * FROM users
WHERE id = $1;


-- name: CreateUser :one
INSERT INTO users (name, email, auth0_id, picture)
VALUES ($1, $2, $3, $4)
RETURNING id, name, email, auth0_id, picture, created_at, updated_at;

-- name: DeleteUser :one
-- INSERT INTO users (name, email, auth0_id, picture)
-- VALUES ($1, $2, $3, $4)
-- RETURNING id, name, email, auth0_id, picture, created_at, updated_at;

-- name: UpdateUserProfile :one
UPDATE users
SET
    name = $2,
    picture = COALESCE($3, picture),
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
RETURNING id, name, email, auth0_id, picture, created_at, updated_at;

