-- name: CreateUser :one
INSERT INTO users (email, password_hash, full_name, phone)
VALUES ($1, $2, $3, $4)
RETURNING user_id, email, password_hash, full_name, phone, status, default_org_id, created_at, updated_at;

-- name: GetUserByEmail :one
SELECT user_id, email, password_hash, full_name, phone, status, default_org_id, created_at, updated_at
FROM users
WHERE lower(email) = lower($1);

-- name: GetUserByID :one
SELECT user_id, email, password_hash, full_name, phone, status, default_org_id, created_at, updated_at
FROM users
WHERE user_id = $1;

-- name: SetUserDefaultOrg :exec
UPDATE users
SET default_org_id = $2, updated_at = now()
WHERE user_id = $1;