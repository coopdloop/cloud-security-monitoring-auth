-- sql/queries/claims.sql

-- name: UpsertCustomClaim :exec
INSERT INTO custom_claims (user_id, claim_key, claim_value)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, claim_key) DO UPDATE
SET claim_value = $3, updated_at = CURRENT_TIMESTAMP;

-- name: GetUserCustomClaims :many
SELECT claim_key, claim_value
FROM custom_claims
WHERE user_id = $1;

-- name: DeleteCustomClaim :exec
DELETE FROM custom_claims
WHERE user_id = $1 AND claim_key = $2;
