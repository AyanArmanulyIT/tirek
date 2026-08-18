-- 0004_catalog_products.sql — Product catalog management + marketplace browsing.
--
-- Evolves the placeholder catalog tables created in 0001_init.sql into the
-- approved domain model (docs/architecture/domain-model.md §2.4):
--   * catalog_products  — SKUs owned directly by the supplier organization.
--                         Replaces the `active`/`archived_at` booleans with an
--                         explicit lifecycle: draft → active → archived.
--   * catalog_categories — flat, per-supplier product groupings (no tree ops).
--   * catalog_prices    — a single current price point per product
--                         (BIGINT minor units + CHAR(3) currency).
--
-- Adds the `catalog.read` permission (browse the active marketplace) and
-- grants it to the System roles that did not already hold `catalog.manage`
-- (viewer, accountant, finance); admin and procurement already hold
-- `catalog.manage` from 0001. New organizations get the same grants via
-- organizations.SystemRoles (mirrored in internal/organizations/service.go).
--
-- Enforces the org-type domain rules at the database level (defense in depth,
-- mirroring the restaurants/suppliers triggers from 0003):
--   * products and categories belong only to supplier organizations
--   * a price must reference a product in the same organization
--
-- Adds SELECT-only marketplace RLS policies so buyer organizations can browse
-- the active products of all suppliers (a public marketplace), while supplier
-- management data (draft/archived products) stays invisible. See
-- docs/architecture/decisions/ for the RLS rationale.

-- ---------------------------------------------------------------------------
-- Permissions (convention: <module>.<verb>, see 0001_init.sql)
-- ---------------------------------------------------------------------------

INSERT INTO permissions (permission_id, description) VALUES
 ('catalog.read', 'Browse the active marketplace catalog');

-- viewer / accountant / finance get catalog.read; admin and procurement
-- already hold catalog.manage from 0001 (which grants catalog.read too).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.role_id, 'catalog.read'
FROM roles r
WHERE r.org_id = '00000000-0000-0000-0000-000000000001'
  AND r.name IN ('viewer', 'accountant', 'finance');

-- ---------------------------------------------------------------------------
-- Catalog products (supplier-owned SKUs)
-- ---------------------------------------------------------------------------

DROP INDEX IF EXISTS idx_catalog_products_supplier;

ALTER TABLE catalog_products
    DROP COLUMN active,
    DROP COLUMN archived_at,
    ADD COLUMN sku          text,
    ADD COLUMN description  text,
    ADD COLUMN status       text NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft','active','archived')),
    ADD COLUMN min_order_qty int NOT NULL DEFAULT 1
        CHECK (min_order_qty >= 1),
    ADD CONSTRAINT catalog_products_unit_check
        CHECK (unit IN ('piece','kg','g','l','ml','pack','box'));

-- A SKU is unique within a supplier organization (NULL = no SKU).
CREATE UNIQUE INDEX uq_catalog_products_org_sku
    ON catalog_products (org_id, sku) WHERE sku IS NOT NULL;

-- Marketplace browsing filters on active products.
CREATE INDEX idx_catalog_products_status ON catalog_products (status);

-- ---------------------------------------------------------------------------
-- Catalog categories (flat, per-supplier groupings)
-- ---------------------------------------------------------------------------

ALTER TABLE catalog_categories
    ADD COLUMN created_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

CREATE UNIQUE INDEX uq_catalog_categories_org_name
    ON catalog_categories (org_id, lower(name));

-- ---------------------------------------------------------------------------
-- Catalog prices — one current price point per product.
-- The 0001 schema allowed historical/tiered price points (product, currency,
-- min_quantity, effective_from); for the MVP we keep a single current price
-- per product (unique on product_id) and reserve min_quantity/effective_from
-- for future tiering. Money is BIGINT minor units + CHAR(3) currency.
-- ---------------------------------------------------------------------------

ALTER TABLE catalog_prices
    DROP CONSTRAINT IF EXISTS catalog_prices_product_id_currency_min_quantity_effective_from_key;

CREATE UNIQUE INDEX uq_catalog_prices_product ON catalog_prices (product_id);

-- ---------------------------------------------------------------------------
-- Domain rules at the database level (defense in depth). The application
-- returns a clean ORG_TYPE_MISMATCH before these triggers ever fire; they are
-- the backstop so a misbehaving role can never violate the rules.
-- ---------------------------------------------------------------------------

-- Products and categories only belong to supplier organizations.
CREATE OR REPLACE FUNCTION enforce_catalog_product_org_type() RETURNS trigger AS $$
DECLARE org_type text;
BEGIN
    SELECT o.type INTO org_type FROM organizations o WHERE o.org_id = NEW.org_id;
    IF org_type IS NULL THEN
        RAISE EXCEPTION 'organization % does not exist', NEW.org_id;
    END IF;
    IF org_type <> 'supplier' THEN
        RAISE EXCEPTION 'products are only available to supplier organizations';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER trg_catalog_products_org_type
    BEFORE INSERT OR UPDATE ON catalog_products
    FOR EACH ROW EXECUTE FUNCTION enforce_catalog_product_org_type();

CREATE OR REPLACE FUNCTION enforce_catalog_category_org_type() RETURNS trigger AS $$
DECLARE org_type text;
BEGIN
    SELECT o.type INTO org_type FROM organizations o WHERE o.org_id = NEW.org_id;
    IF org_type IS NULL THEN
        RAISE EXCEPTION 'organization % does not exist', NEW.org_id;
    END IF;
    IF org_type <> 'supplier' THEN
        RAISE EXCEPTION 'catalog categories are only available to supplier organizations';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER trg_catalog_categories_org_type
    BEFORE INSERT OR UPDATE ON catalog_categories
    FOR EACH ROW EXECUTE FUNCTION enforce_catalog_category_org_type();

-- A price must reference a product in the same organization.
CREATE OR REPLACE FUNCTION enforce_catalog_price_product_org() RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM catalog_products p
        WHERE p.product_id = NEW.product_id AND p.org_id = NEW.org_id
    ) THEN
        RAISE EXCEPTION 'product % does not belong to the price organization', NEW.product_id;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER trg_catalog_prices_product_org
    BEFORE INSERT OR UPDATE ON catalog_prices
    FOR EACH ROW EXECUTE FUNCTION enforce_catalog_price_product_org();

-- ---------------------------------------------------------------------------
-- Marketplace browsing: buyer organizations may read the ACTIVE products of
-- every supplier (a public marketplace). These policies are SELECT-only and
-- never change who can manage (insert/update/delete) catalog data — that stays
-- with the owning supplier via the 0001 tenant_isolation_* policies.
--
-- Design note: exposing active products of other suppliers is intentional —
-- they ARE the marketplace a buyer browses. Non-public data (draft/archived
-- products, supplier profiles, contacts, payouts) is never exposed.
-- ---------------------------------------------------------------------------

CREATE POLICY marketplace_browse_products ON catalog_products FOR SELECT
USING (
    org_id = current_setting('app.tenant_id', true)::uuid
    OR (
        status = 'active'
        AND EXISTS (
            SELECT 1 FROM organizations o
            WHERE o.org_id = current_setting('app.tenant_id', true)::uuid
              AND o.type = 'buyer'
        )
    )
);

CREATE POLICY marketplace_browse_categories ON catalog_categories FOR SELECT
USING (
    org_id = current_setting('app.tenant_id', true)::uuid
    OR EXISTS (
        SELECT 1 FROM organizations o
        WHERE o.org_id = current_setting('app.tenant_id', true)::uuid
          AND o.type = 'buyer'
    )
);

-- Prices are visible to buyers only for products that are active (a buyer
-- must never infer the price of a draft or archived product).
CREATE POLICY marketplace_browse_prices ON catalog_prices FOR SELECT
USING (
    org_id = current_setting('app.tenant_id', true)::uuid
    OR (
        EXISTS (
            SELECT 1 FROM catalog_products p
            WHERE p.product_id = catalog_prices.product_id
              AND p.status = 'active'
        )
        AND EXISTS (
            SELECT 1 FROM organizations o
            WHERE o.org_id = current_setting('app.tenant_id', true)::uuid
              AND o.type = 'buyer'
        )
    )
);