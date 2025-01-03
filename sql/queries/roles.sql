-- sql/queries/roles.sql

-- name: GetUsersWithRoles :many
SELECT
    u.id,
    u.name,
    u.email,
    u.auth0_id,
    u.picture,
    get_user_roles(u.id) as roles
FROM users u
ORDER BY u.name;

-- name: GetUserRoles :many
SELECT r.id, r.name, r.description, r.created_at
FROM roles r
JOIN user_roles ur ON r.id = ur.role_id
WHERE ur.user_id = $1
ORDER BY r.name;

-- name: GetRoleByName :one
SELECT id, name, description, created_at
FROM roles
WHERE name = $1
LIMIT 1;

-- name: AssignUserRole :exec
INSERT INTO user_roles (user_id, role_id, granted_by)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, role_id) DO NOTHING;

-- name: RemoveUserRole :exec
DELETE FROM user_roles
WHERE user_id = $1 AND role_id = $2;

-- name: RemoveAllUserRoles :exec
DELETE FROM user_roles
WHERE user_id = $1;

-- name: ListRoles :many
SELECT id, name, description, created_at
FROM roles
ORDER BY name;

