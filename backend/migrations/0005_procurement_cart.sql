-- 0005_procurement_cart.sql — Procurement cart + purchase requests (RFQs).
--
-- Implements the next marketplace workflow on top of the catalog module:
--   buyer → browse catalog → add to cart → submit → per-supplier purchase
--   requests (the approved domain model's RFQ entity, §2.5).
--
-- New tables:
--   * procurement_carts     — one ACTIVE cart per buyer organization.
--   * procurement_cart_items — cart lines: product, supplier, quantity, unit
--                              and a PRICE SNAPSHOT (BIGINT minor units +
--                              CHAR(3) currency) taken at add time. The snapshot
--                              is immutable while the item is in the cart: a
--                              later supplier price change never alters it.
--
-- Evolved placeholder tables (0001):
--   * rfqs       — buyer→supplier purchase request. Replaces the 0001 status
--                  CHECK with the MVP lifecycle: submitted → cancelled. Requests
--                  are born `submitted` because creation and sending happen
--                  atomically on cart submission. `pending` is reserved for a
--                  future draft flow and is intentionally not modelled yet.
--                  Adds a server-computed total (BIGINT minor units).
--   * rfq_items  — immutable snapshots: product_name, sku, unit, unit price and
--                  line total at submission time. Historical details never
--                  depend on mutable catalog data.
--
-- RBAC: new `procurement.*` permissions; grants mirror
-- internal/organizations/service.go (SystemRoles).
--
-- Defense in depth (mirrors 0003/0004 triggers):
--   * carts/cart items belong only to buyer organizations
--   * a cart item's product must belong to the recorded supplier
--   * rfqs: buyer org → supplier org (types enforced)
--   * rfq items: must match their rfq's org and supplier product
--
-- RLS: carts and cart items are strictly tenant-scoped (org_id = tenant).
-- rfqs/rfq_items keep the 0001 tenant_isolation policy (buyer reads own) and
-- gain a SELECT-only policy so a SUPPLIER tenant can view incoming requests
-- (supplier_org_id = tenant, suppliers only). Suppliers can never modify a
-- request; only the buyer's own tenant can cancel it.

-- ---------------------------------------------------------------------------
-- Permissions (convention: <module>.<verb>, see 0001_init.sql)
-- ---------------------------------------------------------------------------

INSERT INTO permissions (permission_id, description) VALUES
 ('procurement.read',   'View procurement cart and purchase requests'),
 ('procurement.write',  'Manage the procurement cart and submit purchase requests'),
 ('procurement.manage', 'Full control over procurement');

-- Grant to the seeded System organization roles (mirror of 0001 seed; new
-- organizations get the same grants via organizations.SystemRoles).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.role_id, p.permission_id
FROM roles r
CROSS JOIN permissions p
WHERE r.org_id = '00000000-0000-0000-0000-000000000001'
  AND (
       (r.name = 'admin'       AND p.permission_id IN ('procurement.read', 'procurement.write', 'procurement.manage'))
    OR (r.name = 'procurement' AND p.permission_id IN ('procurement.read', 'procurement.write'))
    OR (r.name = 'accountant'  AND p.permission_id = 'procurement.read')
    OR (r.name = 'finance'     AND p.permission_id = 'procurement.read')
    OR (r.name = 'viewer'      AND p.permission_id = 'procurement.read')
  );

-- ---------------------------------------------------------------------------
-- Procurement carts (one active cart per buyer organization)
-- ---------------------------------------------------------------------------

CREATE TABLE procurement_carts (
    cart_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    status       text NOT NULL DEFAULT 'active' CHECK (status IN ('active','abandoned')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- Simplest model consistent with the domain: one active cart per organization.
CREATE UNIQUE INDEX uq_procurement_carts_org_active
    ON procurement_carts (org_id) WHERE status = 'active';
CREATE INDEX idx_procurement_carts_org ON procurement_carts (org_id);

CREATE TABLE procurement_cart_items (
    cart_item_id     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cart_id          uuid NOT NULL REFERENCES procurement_carts(cart_id) ON DELETE CASCADE,
    org_id           uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    product_id       uuid NOT NULL REFERENCES catalog_products(product_id),
    supplier_org_id  uuid NOT NULL REFERENCES organizations(org_id),
    -- product_name is snapshotted at add time: archived products are hidden
    -- from buyers by RLS, so a live join could leave the name empty.
    product_name     text NOT NULL,
    quantity         int NOT NULL CHECK (quantity > 0),
    unit             text NOT NULL
        CHECK (unit IN ('piece','kg','g','l','ml','pack','box')),
    -- Price snapshot (BIGINT minor units): fixed when the product is first
    -- added. Re-adding the same product merges quantity at the original
    -- snapshot; later catalog price changes never touch this value.
    unit_price_minor bigint NOT NULL CHECK (unit_price_minor >= 0),
    currency         char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

-- Re-adding a product merges quantities (deterministic cart behaviour).
CREATE UNIQUE INDEX uq_cart_items_cart_product
    ON procurement_cart_items (cart_id, product_id);
CREATE INDEX idx_cart_items_cart ON procurement_cart_items (cart_id);
CREATE INDEX idx_cart_items_supplier ON procurement_cart_items (supplier_org_id);

-- ---------------------------------------------------------------------------
-- Purchase requests (RFQs) — evolved placeholders with snapshots + totals
-- ---------------------------------------------------------------------------

-- The 0001 status CHECK (draft/sent/accepted/declined/expired) is replaced by
-- the MVP lifecycle: submitted → cancelled. Cancellation is buyer-only and only
-- allowed while the request is `submitted` (before supplier acceptance, which
-- the Orders module adds).
ALTER TABLE rfqs
    DROP CONSTRAINT IF EXISTS rfqs_status_check,
    ALTER COLUMN status SET DEFAULT 'submitted',
    ADD CONSTRAINT rfqs_status_check CHECK (status IN ('submitted','cancelled')),
    ADD COLUMN total_minor bigint NOT NULL DEFAULT 0 CHECK (total_minor >= 0),
    ADD COLUMN updated_at   timestamptz NOT NULL DEFAULT now();

CREATE INDEX idx_rfqs_buyer     ON rfqs (org_id, created_at DESC);
CREATE INDEX idx_rfqs_supplier  ON rfqs (supplier_org_id, created_at DESC);
CREATE INDEX idx_rfqs_status    ON rfqs (status);

-- Immutable snapshots for historical request details. product_id stays as a
-- reference; all display data (name, sku, unit, prices) is copied at
-- submission time and never depends on the live catalog afterwards.
ALTER TABLE rfq_items
    ALTER COLUMN product_id SET NOT NULL,
    ADD COLUMN product_name     text,
    ADD COLUMN sku              text,
    ADD COLUMN unit_price_minor bigint CHECK (unit_price_minor >= 0),
    ADD COLUMN currency         char(3) CHECK (currency ~ '^[A-Z]{3}$'),
    ADD COLUMN line_total_minor bigint CHECK (line_total_minor >= 0);

CREATE INDEX idx_rfq_items_rfq ON rfq_items (rfq_id);

-- ---------------------------------------------------------------------------
-- RLS
-- ---------------------------------------------------------------------------

-- Carts are strictly private to the owning organization.
ALTER TABLE procurement_carts ENABLE ROW LEVEL SECURITY;
ALTER TABLE procurement_cart_items ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_procurement_carts ON procurement_carts
    USING (org_id = current_setting('app.tenant_id', true)::uuid);

CREATE POLICY tenant_isolation_procurement_cart_items ON procurement_cart_items
    USING (org_id = current_setting('app.tenant_id', true)::uuid);

-- rfqs keep tenant_isolation_rfqs (buyer = org_id). Add a SELECT-only policy so
-- a supplier tenant can view requests addressed to it (its incoming queue).
-- Updates stay buyer-only, so a supplier can never modify a request.
CREATE POLICY rfqs_supplier_incoming ON rfqs FOR SELECT
USING (
    supplier_org_id = current_setting('app.tenant_id', true)::uuid
    AND EXISTS (
        SELECT 1 FROM organizations o
        WHERE o.org_id = current_setting('app.tenant_id', true)::uuid
          AND o.type = 'supplier'
    )
);

-- rfq_items: buyer reads via tenant_isolation_rfq_items (org_id); suppliers
-- read the items of their incoming requests through the same supplier gate.
CREATE POLICY rfq_items_supplier_incoming ON rfq_items FOR SELECT
USING (
    EXISTS (
        SELECT 1 FROM rfqs r
        WHERE r.rfq_id = rfq_items.rfq_id
          AND r.supplier_org_id = current_setting('app.tenant_id', true)::uuid
    )
    AND EXISTS (
        SELECT 1 FROM organizations o
        WHERE o.org_id = current_setting('app.tenant_id', true)::uuid
          AND o.type = 'supplier'
    )
);

-- ---------------------------------------------------------------------------
-- Domain rules at the database level (defense in depth)
-- ---------------------------------------------------------------------------

-- Carts and cart items belong only to buyer organizations.
CREATE OR REPLACE FUNCTION enforce_cart_org_type() RETURNS trigger AS $$
DECLARE org_type text;
BEGIN
    SELECT o.type INTO org_type FROM organizations o WHERE o.org_id = NEW.org_id;
    IF org_type IS NULL THEN
        RAISE EXCEPTION 'organization % does not exist', NEW.org_id;
    END IF;
    IF org_type <> 'buyer' THEN
        RAISE EXCEPTION 'procurement carts are only available to buyer organizations';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER trg_procurement_carts_org_type
    BEFORE INSERT OR UPDATE ON procurement_carts
    FOR EACH ROW EXECUTE FUNCTION enforce_cart_org_type();

CREATE TRIGGER trg_cart_items_org_type
    BEFORE INSERT OR UPDATE ON procurement_cart_items
    FOR EACH ROW EXECUTE FUNCTION enforce_cart_org_type();

-- A cart item's product must belong to the recorded supplier organization.
CREATE OR REPLACE FUNCTION enforce_cart_item_product_org() RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM catalog_products p
        WHERE p.product_id = NEW.product_id AND p.org_id = NEW.supplier_org_id
    ) THEN
        RAISE EXCEPTION 'product % does not belong to supplier %', NEW.product_id, NEW.supplier_org_id;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER trg_cart_items_product_org
    BEFORE INSERT OR UPDATE ON procurement_cart_items
    FOR EACH ROW EXECUTE FUNCTION enforce_cart_item_product_org();

-- Purchase requests: buyer org → supplier org (types enforced).
CREATE OR REPLACE FUNCTION enforce_rfq_org_types() RETURNS trigger AS $$
DECLARE buyer_type text; supplier_type text;
BEGIN
    SELECT o.type INTO buyer_type   FROM organizations o WHERE o.org_id = NEW.org_id;
    SELECT o.type INTO supplier_type FROM organizations o WHERE o.org_id = NEW.supplier_org_id;
    IF buyer_type IS NULL THEN
        RAISE EXCEPTION 'organization % does not exist', NEW.org_id;
    END IF;
    IF supplier_type IS NULL THEN
        RAISE EXCEPTION 'organization % does not exist', NEW.supplier_org_id;
    END IF;
    IF buyer_type <> 'buyer' THEN
        RAISE EXCEPTION 'purchase requests are only available to buyer organizations';
    END IF;
    IF supplier_type <> 'supplier' THEN
        RAISE EXCEPTION 'purchase request supplier must be a supplier organization';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER trg_rfqs_org_types
    BEFORE INSERT OR UPDATE ON rfqs
    FOR EACH ROW EXECUTE FUNCTION enforce_rfq_org_types();

-- An rfq item must belong to its rfq's org, and the product must be offered by
-- the rfq's supplier.
CREATE OR REPLACE FUNCTION enforce_rfq_item_orgs() RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM rfqs r
        WHERE r.rfq_id = NEW.rfq_id AND r.org_id = NEW.org_id
    ) THEN
        RAISE EXCEPTION 'purchase request % does not belong to the item organization', NEW.rfq_id;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM rfqs r
        JOIN catalog_products p ON p.product_id = NEW.product_id
        WHERE r.rfq_id = NEW.rfq_id AND p.org_id = r.supplier_org_id
    ) THEN
        RAISE EXCEPTION 'product % does not belong to the purchase request supplier', NEW.product_id;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER trg_rfq_items_orgs
    BEFORE INSERT OR UPDATE ON rfq_items
    FOR EACH ROW EXECUTE FUNCTION enforce_rfq_item_orgs();