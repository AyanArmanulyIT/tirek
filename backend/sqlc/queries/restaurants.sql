-- name: CreateRestaurant :one
INSERT INTO restaurants (org_id, name, legal_name, bin, status, country, default_currency, phone, email)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING restaurant_id, org_id, name, legal_name, bin, status, country, default_currency, phone, email, created_at, updated_at;

-- name: GetRestaurantByID :one
SELECT restaurant_id, org_id, name, legal_name, bin, status, country, default_currency, phone, email, created_at, updated_at
FROM restaurants
WHERE restaurant_id = $1;

-- name: ListRestaurants :many
SELECT restaurant_id, org_id, name, legal_name, bin, status, country, default_currency, phone, email, created_at, updated_at
FROM restaurants
WHERE org_id = $1
ORDER BY created_at DESC, restaurant_id
LIMIT $2 OFFSET $3;

-- name: CountRestaurants :one
SELECT count(*)::bigint
FROM restaurants
WHERE org_id = $1;

-- name: UpdateRestaurant :one
UPDATE restaurants
SET name = $2, legal_name = $3, status = $4, country = $5, default_currency = $6,
    phone = $7, email = $8, updated_at = now()
WHERE restaurant_id = $1
RETURNING restaurant_id, org_id, name, legal_name, bin, status, country, default_currency, phone, email, created_at, updated_at;

-- name: CreateRestaurantLocation :one
INSERT INTO outlets (restaurant_id, org_id, name, address, city, country, status, phone, email)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING outlet_id, restaurant_id, org_id, name, address, city, country, status, phone, email, created_at, updated_at;

-- name: GetRestaurantLocationByID :one
SELECT outlet_id, restaurant_id, org_id, name, address, city, country, status, phone, email, created_at, updated_at
FROM outlets
WHERE outlet_id = $1;

-- name: ListRestaurantLocations :many
SELECT outlet_id, restaurant_id, org_id, name, address, city, country, status, phone, email, created_at, updated_at
FROM outlets
WHERE restaurant_id = $1 AND org_id = $2
ORDER BY created_at DESC, outlet_id
LIMIT $3 OFFSET $4;

-- name: CountRestaurantLocations :one
SELECT count(*)::bigint
FROM outlets
WHERE restaurant_id = $1 AND org_id = $2;

-- name: UpdateRestaurantLocation :one
UPDATE outlets
SET name = $2, address = $3, city = $4, country = $5, status = $6,
    phone = $7, email = $8, updated_at = now()
WHERE outlet_id = $1
RETURNING outlet_id, restaurant_id, org_id, name, address, city, country, status, phone, email, created_at, updated_at;