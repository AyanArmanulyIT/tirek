-- name: CreateRole :one
INSERT INTO roles (org_id, name, is_system)
VALUES ($1, $2, $3)
RETURNING role_id, org_id, name, is_system;

-- name: CreateRolePermission :exec
INSERT INTO role_permissions (role_id, permission_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: CreateMembership :one
INSERT INTO memberships (org_id, user_id, role_id)
VALUES ($1, $2, $3)
RETURNING membership_id, org_id, user_id, role_id, created_at;

-- name: GetMembershipByUserOrg :one
SELECT membership_id, org_id, user_id, role_id, created_at
FROM memberships
WHERE user_id = $1 AND org_id = $2;

-- name: GetRoleByID :one
SELECT role_id, org_id, name, is_system
FROM roles
WHERE role_id = $1;

-- name: GetRoleByOrgName :one
SELECT role_id, org_id, name, is_system
FROM roles
WHERE org_id = $1 AND name = $2;

-- name: ListRolePermissions :many
SELECT permission_id
FROM role_permissions
WHERE role_id = $1
ORDER BY permission_id;

-- name: ListMembershipRolesByUserOrg :many
SELECT r.role_id, r.name
FROM roles r
JOIN memberships m ON m.role_id = r.role_id
WHERE m.user_id = $1 AND m.org_id = $2
ORDER BY r.name;