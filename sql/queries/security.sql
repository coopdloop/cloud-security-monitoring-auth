-- sql/queries/security.sql

-- name: CreateSession :one
INSERT INTO sessions (
    user_id,
    device,
    location,
    ip_address,
    user_agent
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetSession :one
SELECT * FROM sessions
WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL;

-- name: ListActiveSessions :many
SELECT
    s.*,
    u.email as user_email
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.user_id = $1
AND s.revoked_at IS NULL
ORDER BY s.last_active DESC;

-- name: RevokeSession :exec
UPDATE sessions
SET revoked_at = CURRENT_TIMESTAMP
WHERE id = $1 AND user_id = $2
AND revoked_at IS NULL;

-- name: UpdateSessionLastActive :exec
UPDATE sessions
SET last_active = CURRENT_TIMESTAMP
WHERE id = $1 AND user_id = $2
AND revoked_at IS NULL;

-- name: CreateSecurityLog :one
INSERT INTO security_logs (
    user_id,
    event_type,
    event_description,
    ip_address,
    user_agent
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetUserSecurityLogs :many
SELECT
    sl.*,
    u.email as user_email
FROM security_logs sl
JOIN users u ON u.id = sl.user_id
WHERE sl.user_id = $1
ORDER BY sl.created_at DESC
LIMIT $2;

-- name: UpdateUserMFAStatus :exec
UPDATE users
SET
    mfa_enabled = $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1;

-- name: GetUserMFAStatus :one
SELECT
    id,
    email,
    mfa_enabled,
    updated_at
FROM users
WHERE id = $1;
