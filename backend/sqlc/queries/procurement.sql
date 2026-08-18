-- Procurement queries (0005_procurement_cart.sql). Money is BIGINT minor units
-- + CHAR(3) currency; cart item and rfq_item price fields are immutable
-- snapshots taken at add/submission time. Carts are strictly tenant-scoped
-- (RLS); rfqs expose supplier_incoming via a SELECT-only RLS policy.

-- Get or create the single active cart for the caller's organization. On
-- conflict (cart already exists) returns no rows; the caller then loads the
-- existing cart with GetActiveCart.
-- name: GetOrCreateActiveCart :one
INSERT INTO procurement_carts (org_id)
VALUES ($1)
ON CONFLICT (org_id) WHERE status = 'active' DO NOTHING
RETURNING cart_id, org_id, status, created_at, updated_at;

-- name: GetActiveCart :one
SELECT cart_id, org_id, status, created_at, updated_at
FROM procurement_carts
WHERE org_id = $1 AND status = 'active';

-- name: GetCartItem :one
SELECT cart_item_id, cart_id, org_id, product_id, supplier_org_id, product_name, quantity, unit, unit_price_minor, currency, created_at, updated_at
FROM procurement_cart_items
WHERE cart_item_id = $1 AND cart_id = $2;

-- Purchasable product for the marketplace: only ACTIVE products with a current
-- price are visible to buyer tenants (RLS enforces the same rule).
-- name: GetPurchasableProduct :one
SELECT p.product_id, p.org_id, p.name, p.sku, p.unit, p.min_order_qty, p.status,
       pr.currency, pr.unit_price_minor, o.name AS supplier_name
FROM catalog_products p
JOIN catalog_prices pr ON pr.product_id = p.product_id
JOIN organizations o ON o.org_id = p.org_id
WHERE p.product_id = $1 AND p.status = 'active';

-- Add a product to the cart or merge quantities (same active product added
-- twice merges at the original price snapshot).
-- name: UpsertCartItem :one
INSERT INTO procurement_cart_items (cart_id, org_id, product_id, supplier_org_id, product_name, quantity, unit, unit_price_minor, currency)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (cart_id, product_id)
DO UPDATE SET quantity = procurement_cart_items.quantity + EXCLUDED.quantity, updated_at = now()
RETURNING cart_item_id, cart_id, org_id, product_id, supplier_org_id, product_name, quantity, unit, unit_price_minor, currency, created_at, updated_at;

-- name: UpdateCartItemQuantity :one
UPDATE procurement_cart_items
SET quantity = $3, updated_at = now()
WHERE cart_item_id = $1 AND cart_id = $2
RETURNING cart_item_id, cart_id, org_id, product_id, supplier_org_id, product_name, quantity, unit, unit_price_minor, currency, created_at, updated_at;

-- name: DeleteCartItem :execrows
DELETE FROM procurement_cart_items
WHERE cart_item_id = $1 AND cart_id = $2;

-- name: ListCartItemsByCart :many
SELECT ci.cart_item_id, ci.cart_id, ci.org_id, ci.product_id, ci.supplier_org_id, ci.product_name, ci.quantity, ci.unit, ci.unit_price_minor, ci.currency, ci.created_at, ci.updated_at,
       o.name AS supplier_name
FROM procurement_cart_items ci
JOIN organizations o ON o.org_id = ci.supplier_org_id
WHERE ci.cart_id = $1
ORDER BY o.name, ci.created_at, ci.cart_item_id;

-- Lock the active cart and its items for an atomic submission.
-- name: GetActiveCartForUpdate :one
SELECT cart_id, org_id, status, created_at, updated_at
FROM procurement_carts
WHERE org_id = $1 AND status = 'active'
FOR UPDATE;

-- name: ListCartItemsByCartForUpdate :many
SELECT cart_item_id, cart_id, org_id, product_id, supplier_org_id, product_name, quantity, unit, unit_price_minor, currency, created_at, updated_at
FROM procurement_cart_items
WHERE cart_id = $1
FOR UPDATE;

-- name: DeleteCartItems :exec
DELETE FROM procurement_cart_items
WHERE cart_id = $1;

-- name: CreatePurchaseRequest :one
INSERT INTO rfqs (org_id, supplier_org_id, number, status, currency, total_minor, created_by)
VALUES ($1, $2, $3, 'submitted', $4, $5, $6)
RETURNING rfq_id, org_id, supplier_org_id, outlet_id, number, status, delivery_window, currency, total_minor, created_by, created_at, updated_at;

-- name: CreatePurchaseRequestItem :one
INSERT INTO rfq_items (rfq_id, org_id, product_id, description, quantity, unit, product_name, sku, unit_price_minor, currency, line_total_minor)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING rfq_item_id, rfq_id, org_id, product_id, description, quantity, unit, product_name, sku, unit_price_minor, currency, line_total_minor;

-- name: ListPurchaseRequests :many
SELECT r.rfq_id, r.org_id, r.supplier_org_id, r.number, r.status, r.currency, r.total_minor, r.created_by, r.created_at, r.updated_at,
       o.name AS supplier_name
FROM rfqs r
JOIN organizations o ON o.org_id = r.supplier_org_id
WHERE r.org_id = $1
ORDER BY r.created_at DESC, r.rfq_id
LIMIT $2 OFFSET $3;

-- name: CountPurchaseRequests :one
SELECT count(*)::bigint
FROM rfqs
WHERE org_id = $1;

-- name: ListIncomingRequests :many
SELECT r.rfq_id, r.org_id, r.supplier_org_id, r.number, r.status, r.currency, r.total_minor, r.created_by, r.created_at, r.updated_at,
       o.name AS buyer_name
FROM rfqs r
JOIN organizations o ON o.org_id = r.org_id
WHERE r.supplier_org_id = $1
ORDER BY r.created_at DESC, r.rfq_id
LIMIT $2 OFFSET $3;

-- name: CountIncomingRequests :one
SELECT count(*)::bigint
FROM rfqs
WHERE supplier_org_id = $1;

-- name: GetPurchaseRequest :one
SELECT r.rfq_id, r.org_id, r.supplier_org_id, r.number, r.status, r.currency, r.total_minor, r.created_by, r.created_at, r.updated_at,
       o.name AS supplier_name
FROM rfqs r
JOIN organizations o ON o.org_id = r.supplier_org_id
WHERE r.rfq_id = $1;

-- name: GetPurchaseRequestItems :many
SELECT rfq_item_id, rfq_id, org_id, product_id, description, quantity, unit, product_name, sku, unit_price_minor, currency, line_total_minor
FROM rfq_items
WHERE rfq_id = $1
ORDER BY rfq_item_id;

-- Cancel a purchase request (buyer-only; RLS restricts the update to the
-- buyer's own tenant and the status guard below prevents re-cancellation).
-- name: CancelPurchaseRequest :one
UPDATE rfqs
SET status = 'cancelled', updated_at = now()
WHERE rfq_id = $1 AND status = 'submitted'
RETURNING rfq_id, org_id, supplier_org_id, number, status, currency, total_minor, created_by, created_at, updated_at;

-- name: GetIncomingRequest :one
SELECT r.rfq_id, r.org_id, r.supplier_org_id, r.number, r.status, r.currency, r.total_minor, r.created_by, r.created_at, r.updated_at,
       o.name AS buyer_name
FROM rfqs r
JOIN organizations o ON o.org_id = r.org_id
WHERE r.rfq_id = $1;
-- Idempotency for purchase request submission. Claiming the key first makes a
-- concurrent duplicate fail-safe: only one transaction owns the key; the loser
-- replays the stored response or returns a conflict.
-- name: ClaimIdempotencyKey :one
INSERT INTO idempotency_keys (org_id, key, endpoint, request_hash)
VALUES ($1, $2, $3, $4)
ON CONFLICT (org_id, key, endpoint) DO NOTHING
RETURNING id, org_id, key, endpoint, request_hash, response_body, response_code;

-- name: GetIdempotencyKey :one
SELECT id, org_id, key, endpoint, request_hash, response_body, response_code
FROM idempotency_keys
WHERE org_id = $1 AND key = $2 AND endpoint = $3;

-- name: CompleteIdempotencyKey :exec
UPDATE idempotency_keys
SET response_body = $4, response_code = $5
WHERE org_id = $1 AND key = $2 AND endpoint = $3;

-- Outbox: signal downstream systems (future Orders module) that a purchase
-- request was submitted. Written in the same transaction as the rfq.
-- name: InsertOutboxEvent :exec
INSERT INTO outbox_events (org_id, topic, entity_id, payload)
VALUES ($1, $2, $3, $4);
