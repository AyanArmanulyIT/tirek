-- name: CreateOrganization :one
INSERT INTO organizations (name, type, country, default_currency, bin)
VALUES ($1, $2, $3, $4, $5)
RETURNING org_id, name, type, country, default_currency, bin, status, created_at, updated_at;

-- name: GetOrganizationByID :one
SELECT org_id, name, type, country, default_currency, bin, status, created_at, updated_at
FROM organizations
WHERE org_id = $1;