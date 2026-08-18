-- name: CreateSupplier :one
INSERT INTO suppliers (org_id, name, legal_name, bin, payment_terms, status, country, default_currency, phone, email)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING supplier_id, org_id, name, legal_name, bin, payout_account, payment_terms, rating, status, country, default_currency, phone, email, created_at, updated_at;

-- name: GetSupplierByID :one
SELECT supplier_id, org_id, name, legal_name, bin, payout_account, payment_terms, rating, status, country, default_currency, phone, email, created_at, updated_at
FROM suppliers
WHERE supplier_id = $1;

-- name: GetSupplierByOrgID :one
SELECT supplier_id, org_id, name, legal_name, bin, payout_account, payment_terms, rating, status, country, default_currency, phone, email, created_at, updated_at
FROM suppliers
WHERE org_id = $1;

-- name: ListSuppliers :many
SELECT supplier_id, org_id, name, legal_name, bin, payout_account, payment_terms, rating, status, country, default_currency, phone, email, created_at, updated_at
FROM suppliers
WHERE org_id = $1
ORDER BY created_at DESC, supplier_id
LIMIT $2 OFFSET $3;

-- name: CountSuppliers :one
SELECT count(*)::bigint
FROM suppliers
WHERE org_id = $1;

-- name: UpdateSupplier :one
UPDATE suppliers
SET name = $2, legal_name = $3, status = $4, country = $5, default_currency = $6,
    phone = $7, email = $8, updated_at = now()
WHERE supplier_id = $1
RETURNING supplier_id, org_id, name, legal_name, bin, payout_account, payment_terms, rating, status, country, default_currency, phone, email, created_at, updated_at;