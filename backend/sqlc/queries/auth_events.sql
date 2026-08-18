-- name: InsertAuthEvent :exec
INSERT INTO auth_events (user_id, org_id, action, identifier, ip, user_agent)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: CountRecentLoginFailuresByUser :one
SELECT count(*)::bigint
FROM auth_events
WHERE action = 'login_failed' AND user_id = $1 AND created_at > $2;

-- name: CountRecentLoginFailuresByIP :one
SELECT count(*)::bigint
FROM auth_events
WHERE action = 'login_failed' AND ip = $1 AND created_at > $2;