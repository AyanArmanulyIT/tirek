-- 0003_restaurants_suppliers.sql — Restaurants + suppliers foundation.
--
-- Evolves the placeholder tables created in 0001_init.sql into the full
-- foundation models:
--   * restaurants  (owned by buyer organizations)  — added operational fields
--   * outlets      (restaurant locations/branches) — added operational fields
--   * suppliers    (1:1 profile of a supplier organization) — added fields
--   * supplier_contacts — timestamps
--
-- Adds the restaurants.* / suppliers.* permission graph, grants it to the
-- seeded System roles (mirrored for new organizations in
-- internal/organizations/service.go), and enforces the org-type domain rules
-- (buyer-only restaurants, supplier-only supplier profiles) at the database
-- level with triggers — defense in depth behind the application checks.

-- ---------------------------------------------------------------------------
-- Permissions (convention: <module>.<verb>, see 0001_init.sql)
-- ---------------------------------------------------------------------------

INSERT INTO permissions (permission_id, description) VALUES
 ('restaurants.read',   'View restaurants and their locations'),
 ('restaurants.write',  'Create and update restaurants and locations'),
 ('restaurants.manage', 'Full control over restaurants and locations'),
 ('suppliers.read',     'View the supplier profile'),
 ('suppliers.write',    'Create and update the supplier profile'),
 ('suppliers.manage',   'Full control over the supplier profile');

-- Grant to the seeded System organization roles (mirror of 0001 seed; new
-- organizations get the same grants via organizations.SystemRoles).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.role_id, p.permission_id
FROM roles r
CROSS JOIN permissions p
WHERE r.org_id = '00000000-0000-0000-0000-000000000001'
  AND (
       (r.name = 'admin'       AND p.permission_id IN ('restaurants.manage', 'suppliers.manage'))
    OR (r.name = 'procurement' AND p.permission_id = 'restaurants.read')
    OR (r.name = 'viewer'      AND p.permission_id IN ('restaurants.read', 'suppliers.read'))
  );

-- ---------------------------------------------------------------------------
-- Restaurants (buyer organizations own restaurants)
-- ---------------------------------------------------------------------------

ALTER TABLE restaurants
    ADD COLUMN status            text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active','suspended','closed')),
    ADD COLUMN country           char(2) NOT NULL DEFAULT 'KZ',
    ADD COLUMN default_currency  char(3) NOT NULL DEFAULT 'KZT'
        CHECK (default_currency ~ '^[A-Z]{3}$'),
    ADD COLUMN phone             text,
    ADD COLUMN email             text,
    ADD COLUMN updated_at        timestamptz NOT NULL DEFAULT now();

CREATE INDEX idx_restaurants_org ON restaurants (org_id);

-- ---------------------------------------------------------------------------
-- Outlets = restaurant locations/branches (approved domain model: outlets)
-- ---------------------------------------------------------------------------

ALTER TABLE outlets
    ADD COLUMN status            text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active','inactive')),
    ADD COLUMN country           char(2) NOT NULL DEFAULT 'KZ',
    ADD COLUMN phone             text,
    ADD COLUMN email             text,
    ADD COLUMN updated_at        timestamptz NOT NULL DEFAULT now();

CREATE INDEX idx_outlets_org        ON outlets (org_id);
CREATE INDEX idx_outlets_restaurant ON outlets (restaurant_id);

-- ---------------------------------------------------------------------------
-- Suppliers (one profile per supplier organization)
-- ---------------------------------------------------------------------------

ALTER TABLE suppliers
    ADD COLUMN status            text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active','suspended','closed')),
    ADD COLUMN country           char(2) NOT NULL DEFAULT 'KZ',
    ADD COLUMN default_currency  char(3) NOT NULL DEFAULT 'KZT'
        CHECK (default_currency ~ '^[A-Z]{3}$'),
    ADD COLUMN phone             text,
    ADD COLUMN email             text;

-- One supplier profile per organization (domain model: ORGANIZATION ||--o{ SUPPLIER : is).
CREATE UNIQUE INDEX uq_suppliers_org ON suppliers (org_id);
CREATE INDEX idx_suppliers_org ON suppliers (org_id);

ALTER TABLE supplier_contacts
    ADD COLUMN created_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

-- ---------------------------------------------------------------------------
-- Domain rules at the database level (defense in depth). The application
-- returns a clean ORG_TYPE_MISMATCH before these triggers ever fire; they are
-- the backstop so a misbehaving role can never violate the rules.
-- ---------------------------------------------------------------------------

-- Restaurants (and their outlets) only belong to buyer organizations.
CREATE OR REPLACE FUNCTION enforce_restaurant_org_type() RETURNS trigger AS $$
DECLARE org_type text;
BEGIN
    SELECT o.type INTO org_type FROM organizations o WHERE o.org_id = NEW.org_id;
    IF org_type IS NULL THEN
        RAISE EXCEPTION 'organization % does not exist', NEW.org_id;
    END IF;
    IF org_type <> 'buyer' THEN
        RAISE EXCEPTION 'restaurants are only available to buyer organizations';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER trg_restaurants_org_type
    BEFORE INSERT OR UPDATE ON restaurants
    FOR EACH ROW EXECUTE FUNCTION enforce_restaurant_org_type();

CREATE TRIGGER trg_outlets_org_type
    BEFORE INSERT OR UPDATE ON outlets
    FOR EACH ROW EXECUTE FUNCTION enforce_restaurant_org_type();

-- An outlet must belong to a restaurant in the same organization.
CREATE OR REPLACE FUNCTION enforce_outlet_restaurant_org() RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM restaurants r
        WHERE r.restaurant_id = NEW.restaurant_id AND r.org_id = NEW.org_id
    ) THEN
        RAISE EXCEPTION 'restaurant % does not belong to the outlet organization', NEW.restaurant_id;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER trg_outlets_restaurant_org
    BEFORE INSERT OR UPDATE ON outlets
    FOR EACH ROW EXECUTE FUNCTION enforce_outlet_restaurant_org();

-- Supplier profiles only belong to supplier organizations.
CREATE OR REPLACE FUNCTION enforce_supplier_org_type() RETURNS trigger AS $$
DECLARE org_type text;
BEGIN
    SELECT o.type INTO org_type FROM organizations o WHERE o.org_id = NEW.org_id;
    IF org_type IS NULL THEN
        RAISE EXCEPTION 'organization % does not exist', NEW.org_id;
    END IF;
    IF org_type <> 'supplier' THEN
        RAISE EXCEPTION 'supplier profiles are only available to supplier organizations';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER trg_suppliers_org_type
    BEFORE INSERT OR UPDATE ON suppliers
    FOR EACH ROW EXECUTE FUNCTION enforce_supplier_org_type();