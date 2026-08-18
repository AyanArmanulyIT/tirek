-- name: CreateSession :one
INSERT INTO sessions (user_id, org_id, refresh_hash, expires_at, ip, user_agent)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING session_id, user_id, org_id, refresh_hash, expires_at, revoked_at, revoked_reason, last_used_at, created_at, ip, user_agent;

-- name: GetSessionByRefreshHash :one
SELECT session_id, user_id, org_id, refresh_hash, expires_at, revoked_at, revoked_reason, last_used_at, created_at, ip, user_agent
FROM sessions
WHERE refresh_hash = $1;

-- name: GetSessionByRefreshHashForUpdate :one
SELECT session_id, user_id, org_id, refresh_hash, expires_at, revoked_at, revoked_reason, last_used_at, created_at, ip, user_agent
FROM sessions
WHERE refresh_hash = $1
FOR UPDATE;

-- name: RotateSession :one
UPDATE sessions
SET refresh_hash = $2, expires_at = $3, last_used_at = now()
WHERE session_id = $1 AND revoked_at IS NULL
RETURNING session_id, user_id, org_id, refresh_hash, expires_at, revoked_at, revoked_reason, last_used_at, created_at, ip, user_agent;

-- name: RevokeSession :one
UPDATE sessions
SET revoked_at = now(), revoked_reason = $2
WHERE session_id = $1 AND revoked_at IS NULL
RETURNING session_id, user_id, org_id, refresh_hash, expires_at, revoked_at, revoked_reason, last_used_at, created_at, ip, user_agent;

-- name: InsertRefreshHistory :exec
INSERT INTO session_refresh_history (session_id, refresh_hash)
VALUES ($1, $2);

-- name: GetSessionIDByHistoryHash :one
SELECT session_id
FROM session_refresh_history
WHERE refresh_hash = $1;