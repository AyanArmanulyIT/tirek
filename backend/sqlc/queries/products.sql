-- Catalog queries (0004_catalog_products.sql). Money is BIGINT minor units +
-- CHAR(3) currency. Marketplace queries are buyer-scoped; RLS exposes only the
-- active products of all suppliers.

-- name: CreateCatalogProduct :one
INSERT INTO catalog_products (org_id, category_id, name, sku, description, unit, status, vat_rate_bps, image_s3_key, min_order_qty)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING product_id, org_id, category_id, name, sku, description, unit, status, vat_rate_bps, image_s3_key, min_order_qty, created_at, updated_at;

-- name: GetCatalogProductByID :one
SELECT product_id, org_id, category_id, name, sku, description, unit, status, vat_rate_bps, image_s3_key, min_order_qty, created_at, updated_at
FROM catalog_products
WHERE product_id = $1;

-- name: ListCatalogProducts :many
SELECT product_id, org_id, category_id, name, sku, description, unit, status, vat_rate_bps, image_s3_key, min_order_qty, created_at, updated_at
FROM catalog_products
WHERE org_id = $1
ORDER BY created_at DESC, product_id
LIMIT $2 OFFSET $3;

-- name: CountCatalogProducts :one
SELECT count(*)::bigint
FROM catalog_products
WHERE org_id = $1;

-- name: UpdateCatalogProduct :one
UPDATE catalog_products
SET name = $2, category_id = $3, sku = $4, description = $5, unit = $6,
    status = $7, vat_rate_bps = $8, image_s3_key = $9, min_order_qty = $10,
    updated_at = now()
WHERE product_id = $1
RETURNING product_id, org_id, category_id, name, sku, description, unit, status, vat_rate_bps, image_s3_key, min_order_qty, created_at, updated_at;

-- name: UpsertCatalogPrice :one
INSERT INTO catalog_prices (product_id, org_id, currency, unit_price_minor, min_quantity, effective_from)
VALUES ($1, $2, $3, $4, 1, current_date)
ON CONFLICT (product_id)
DO UPDATE SET currency = EXCLUDED.currency, unit_price_minor = EXCLUDED.unit_price_minor
RETURNING price_id, product_id, org_id, currency, unit_price_minor, min_quantity, effective_from;

-- name: GetCatalogPrice :one
SELECT price_id, product_id, org_id, currency, unit_price_minor, min_quantity, effective_from
FROM catalog_prices
WHERE product_id = $1;

-- name: CreateCatalogCategory :one
INSERT INTO catalog_categories (org_id, name)
VALUES ($1, $2)
RETURNING category_id, org_id, name, created_at, updated_at;

-- name: GetCatalogCategoryByID :one
SELECT category_id, org_id, name, created_at, updated_at
FROM catalog_categories
WHERE category_id = $1;

-- name: ListCatalogCategories :many
SELECT category_id, org_id, name, created_at, updated_at
FROM catalog_categories
WHERE org_id = $1
ORDER BY created_at, category_id;

-- name: CountCatalogCategories :one
SELECT count(*)::bigint
FROM catalog_categories
WHERE org_id = $1;

-- name: ListCatalogCategoriesForBrowse :many
SELECT category_id, org_id, name
FROM catalog_categories
ORDER BY name, category_id;

-- name: CountCatalogCategoriesForBrowse :one
SELECT count(*)::bigint
FROM catalog_categories;

-- Marketplace browsing (buyer tenant; RLS exposes only active supplier products).
-- name: BrowseCatalogProducts :many
SELECT p.product_id, p.org_id, p.category_id, p.name, p.sku, p.description, p.unit,
       p.status, p.vat_rate_bps, p.image_s3_key, p.min_order_qty, p.created_at, p.updated_at,
       pr.currency, pr.unit_price_minor,
       o.name AS supplier_name,
       c.name AS category_name
FROM catalog_products p
JOIN catalog_prices pr ON pr.product_id = p.product_id
JOIN organizations o ON o.org_id = p.org_id
LEFT JOIN catalog_categories c ON c.category_id = p.category_id
WHERE p.status = 'active'
  AND (sqlc.narg('q')::text = '' OR p.name ILIKE '%' || sqlc.narg('q')::text || '%')
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR p.org_id = sqlc.narg('supplier_id')::uuid)
  AND (sqlc.narg('category_id')::uuid IS NULL OR p.category_id = sqlc.narg('category_id')::uuid)
ORDER BY p.name, p.product_id
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountBrowseCatalogProducts :one
SELECT count(*)::bigint
FROM catalog_products p
WHERE p.status = 'active'
  AND (sqlc.narg('q')::text = '' OR p.name ILIKE '%' || sqlc.narg('q')::text || '%')
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR p.org_id = sqlc.narg('supplier_id')::uuid)
  AND (sqlc.narg('category_id')::uuid IS NULL OR p.category_id = sqlc.narg('category_id')::uuid);