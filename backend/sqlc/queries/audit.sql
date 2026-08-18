-- name: InsertAuditLog :exec
INSERT INTO audit_logs (org_id, actor_id, action, entity_type, entity_id, before, after, ip, user_agent)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);