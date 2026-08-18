-- 0001_init.sql — Tirek initial schema (MVP)
-- Applies in one transaction. Uses UUIDv7 via pgcrypto for time-ordered ids.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Platform
-- ---------------------------------------------------------------------------

CREATE TABLE outbox_events (
    id            bigserial PRIMARY KEY,
    org_id        uuid,
    topic         text NOT NULL,
    entity_id     text NOT NULL,
    payload       jsonb NOT NULL DEFAULT '{}'::jsonb,
    status        text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','done','retry','dead')),
    attempts      int  NOT NULL DEFAULT 0,
    scheduled_at  timestamptz NOT NULL DEFAULT now(),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_outbox_poll ON outbox_events (status, scheduled_at)
    WHERE status IN ('pending','retry');
CREATE UNIQUE INDEX uq_outbox_once ON outbox_events (topic, entity_id)
    WHERE status IN ('pending','processing');

CREATE TABLE webhook_events (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider    text NOT NULL,
    event_id    text NOT NULL,
    payload     jsonb NOT NULL,
    status      text NOT NULL DEFAULT 'recorded' CHECK (status IN ('recorded','processed','failed')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz
);
CREATE UNIQUE INDEX uq_webhook_events ON webhook_events (provider, event_id);
CREATE INDEX idx_webhook_status ON webhook_events (status, created_at);

CREATE TABLE idempotency_keys (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        uuid,
    key           text NOT NULL,
    endpoint      text NOT NULL,
    request_hash  text NOT NULL,
    response_body jsonb,
    response_code int,
    created_at    timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL DEFAULT now() + interval '24 hours'
);
CREATE UNIQUE INDEX uq_idempotency ON idempotency_keys (org_id, key, endpoint);

CREATE TABLE audit_logs (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       uuid,
    actor_id     uuid,
    action       text NOT NULL,
    entity_type  text NOT NULL,
    entity_id    text,
    before       jsonb,
    after        jsonb,
    ip           inet,
    user_agent   text,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_org_time ON audit_logs (org_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- Identity / Organizations
-- ---------------------------------------------------------------------------

CREATE TABLE organizations (
    org_id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL,
    type            text NOT NULL CHECK (type IN ('buyer','supplier')),
    country         char(2) NOT NULL DEFAULT 'KZ',
    default_currency char(3) NOT NULL DEFAULT 'KZT',
    bin             text,           -- business identifier (BIN/IIN), PII
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','closed')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    user_id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email           text NOT NULL,
    password_hash   text NOT NULL,          -- argon2id
    full_name       text NOT NULL,
    phone           text,                    -- PII
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_users_email ON users (lower(email));

CREATE TABLE sessions (
    session_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    refresh_hash    text NOT NULL,
    expires_at      timestamptz NOT NULL,
    revoked_at      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    ip              inet,
    user_agent      text
);
CREATE INDEX idx_sessions_user ON sessions (user_id);
CREATE INDEX idx_sessions_expiry ON sessions (expires_at);

CREATE TABLE permissions (
    permission_id   text PRIMARY KEY,        -- e.g. 'orders.create'
    description     text NOT NULL
);

CREATE TABLE roles (
    role_id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    name            text NOT NULL,           -- owner | admin | procurement | accountant | finance | viewer | custom
    is_system       boolean NOT NULL DEFAULT false,
    UNIQUE (org_id, name)
);

CREATE TABLE role_permissions (
    role_id         uuid NOT NULL REFERENCES roles(role_id) ON DELETE CASCADE,
    permission_id   text NOT NULL REFERENCES permissions(permission_id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE memberships (
    membership_id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    role_id         uuid NOT NULL REFERENCES roles(role_id) ON DELETE RESTRICT,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, user_id, role_id)
);
CREATE INDEX idx_memberships_user ON memberships (user_id);

CREATE TABLE api_keys (
    api_key_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    name            text NOT NULL,
    key_hash        text NOT NULL,           -- sha256 of the raw key; raw key shown once
    scopes          text[] NOT NULL DEFAULT '{}',
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_used_at    timestamptz
);
CREATE UNIQUE INDEX uq_api_keys_hash ON api_keys (key_hash);

-- ---------------------------------------------------------------------------
-- Restaurants / Suppliers
-- ---------------------------------------------------------------------------

CREATE TABLE restaurants (
    restaurant_id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    name            text NOT NULL,
    legal_name      text,
    bin             text,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE outlets (
    outlet_id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id   uuid NOT NULL REFERENCES restaurants(restaurant_id) ON DELETE CASCADE,
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    name            text NOT NULL,
    address         text NOT NULL,
    city            text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE suppliers (
    supplier_id     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    name            text NOT NULL,
    legal_name      text,
    bin             text,
    payout_account  text,                     -- bank details, PII, pgp-encrypted where enabled
    payment_terms   text NOT NULL DEFAULT 'net14',
    rating          numeric(3,2),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE supplier_contacts (
    contact_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    supplier_id     uuid NOT NULL REFERENCES suppliers(supplier_id) ON DELETE CASCADE,
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    full_name       text NOT NULL,
    phone           text,
    email           text,
    role            text
);

-- ---------------------------------------------------------------------------
-- Payments (PSP accounts per org)
-- ---------------------------------------------------------------------------

CREATE TABLE psp_accounts (
    psp_account_id  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    provider        text NOT NULL,           -- kaspi | mock
    mode            text NOT NULL DEFAULT 'sandbox' CHECK (mode IN ('sandbox','live')),
    merchant_id     text,
    credentials_ref text,                    -- Secrets Manager key, never the secret itself
    status          text NOT NULL DEFAULT 'active',
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Catalog
-- ---------------------------------------------------------------------------

CREATE TABLE catalog_categories (
    category_id     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    name            text NOT NULL,
    parent_id       uuid REFERENCES catalog_categories(category_id),
    archived_at     timestamptz
);

CREATE TABLE catalog_products (
    product_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    category_id     uuid REFERENCES catalog_categories(category_id),
    name            text NOT NULL,
    unit            text NOT NULL,           -- kg | box | pcs | liter
    vat_rate_bps    int NOT NULL DEFAULT 1200, -- 12.00% KZ VAT
    image_s3_key    text,
    active          boolean NOT NULL DEFAULT true,
    archived_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_catalog_products_supplier ON catalog_products (org_id, active);

CREATE TABLE catalog_prices (
    price_id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id      uuid NOT NULL REFERENCES catalog_products(product_id) ON DELETE CASCADE,
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    currency        char(3) NOT NULL DEFAULT 'KZT' CHECK (currency ~ '^[A-Z]{3}$'),
    unit_price_minor bigint NOT NULL CHECK (unit_price_minor >= 0),
    min_quantity    int NOT NULL DEFAULT 1,
    effective_from  date NOT NULL DEFAULT current_date,
    UNIQUE (product_id, currency, min_quantity, effective_from)
);

-- ---------------------------------------------------------------------------
-- Procurement
-- ---------------------------------------------------------------------------

CREATE TABLE rfqs (
    rfq_id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    supplier_org_id uuid NOT NULL REFERENCES organizations(org_id),
    outlet_id       uuid REFERENCES outlets(outlet_id),
    number          text NOT NULL,
    status          text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','sent','accepted','declined','expired')),
    delivery_window daterange,
    currency        char(3) NOT NULL DEFAULT 'KZT',
    created_by      uuid REFERENCES users(user_id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, number)
);

CREATE TABLE rfq_items (
    rfq_item_id     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    rfq_id          uuid NOT NULL REFERENCES rfqs(rfq_id) ON DELETE CASCADE,
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    product_id      uuid REFERENCES catalog_products(product_id),
    description     text NOT NULL,
    quantity        int NOT NULL CHECK (quantity > 0),
    unit            text NOT NULL
);

CREATE TABLE purchase_orders (
    po_id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    supplier_org_id uuid NOT NULL REFERENCES organizations(org_id),
    outlet_id       uuid REFERENCES outlets(outlet_id),
    rfq_id          uuid REFERENCES rfqs(rfq_id),
    number          text NOT NULL,
    status          text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','sent','confirmed','fulfilled','invoiced','cancelled')),
    currency        char(3) NOT NULL DEFAULT 'KZT' CHECK (currency ~ '^[A-Z]{3}$'),
    total_minor     bigint NOT NULL DEFAULT 0 CHECK (total_minor >= 0),
    delivery_date   date,
    created_by      uuid REFERENCES users(user_id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, number)
);
CREATE INDEX idx_po_buyer ON purchase_orders (org_id, created_at DESC);
CREATE INDEX idx_po_supplier ON purchase_orders (supplier_org_id, created_at DESC);

CREATE TABLE po_items (
    po_item_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    po_id           uuid NOT NULL REFERENCES purchase_orders(po_id) ON DELETE CASCADE,
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    product_id      uuid REFERENCES catalog_products(product_id),
    description     text NOT NULL,
    quantity        int NOT NULL CHECK (quantity > 0),
    unit            text NOT NULL,
    unit_price_minor bigint NOT NULL CHECK (unit_price_minor >= 0),
    vat_rate_bps    int NOT NULL DEFAULT 1200,
    line_total_minor bigint NOT NULL CHECK (line_total_minor >= 0)
);

CREATE TABLE orders (
    order_id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    po_id           uuid NOT NULL REFERENCES purchase_orders(po_id),
    outlet_id       uuid REFERENCES outlets(outlet_id),
    number          text NOT NULL,
    currency        char(3) NOT NULL DEFAULT 'KZT',
    total_minor     bigint NOT NULL CHECK (total_minor >= 0),
    status          text NOT NULL DEFAULT 'open' CHECK (status IN ('open','completed','cancelled')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, number)
);

-- ---------------------------------------------------------------------------
-- Invoicing
-- ---------------------------------------------------------------------------

CREATE TABLE invoices (
    invoice_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    supplier_org_id uuid NOT NULL REFERENCES organizations(org_id),
    po_id           uuid NOT NULL REFERENCES purchase_orders(po_id),
    number          text NOT NULL,
    currency        char(3) NOT NULL DEFAULT 'KZT' CHECK (currency ~ '^[A-Z]{3}$'),
    subtotal_minor  bigint NOT NULL CHECK (subtotal_minor >= 0),
    vat_minor       bigint NOT NULL CHECK (vat_minor >= 0),
    total_minor     bigint NOT NULL CHECK (total_minor >= 0),
    paid_minor      bigint NOT NULL DEFAULT 0 CHECK (paid_minor >= 0),
    status          text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','issued','approved','disputed','paid','partially_paid','cancelled')),
    issue_date      date,
    due_date        date,
    version         int NOT NULL DEFAULT 1,
    created_by      uuid REFERENCES users(user_id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, number)
);
CREATE INDEX idx_invoices_supplier ON invoices (org_id, supplier_org_id, status);
CREATE INDEX idx_invoices_due ON invoices (org_id, due_date);

CREATE TABLE invoice_items (
    invoice_item_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id      uuid NOT NULL REFERENCES invoices(invoice_id) ON DELETE CASCADE,
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    po_item_id      uuid REFERENCES po_items(po_item_id),
    description     text NOT NULL,
    quantity        int NOT NULL CHECK (quantity > 0),
    unit_price_minor bigint NOT NULL CHECK (unit_price_minor >= 0),
    vat_rate_bps    int NOT NULL DEFAULT 1200,
    line_total_minor bigint NOT NULL CHECK (line_total_minor >= 0)
);

-- ---------------------------------------------------------------------------
-- Payments
-- ---------------------------------------------------------------------------

CREATE TABLE payment_intents (
    intent_id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    invoice_id      uuid NOT NULL REFERENCES invoices(invoice_id),
    psp_account_id  uuid REFERENCES psp_accounts(psp_account_id),
    kind            text NOT NULL DEFAULT 'full' CHECK (kind IN ('full','partial')),
    amount_minor    bigint NOT NULL CHECK (amount_minor > 0),
    currency        char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    provider        text NOT NULL,
    provider_ref    text,
    status          text NOT NULL DEFAULT 'created' CHECK (status IN ('created','pending_psp','requires_action','paid','failed','expired','cancelled')),
    idempotency_key text,
    created_by      uuid REFERENCES users(user_id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_intent_idempotency ON payment_intents (org_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX uq_invoice_full_payment ON payment_intents (invoice_id) WHERE kind = 'full' AND status IN ('paid','pending_psp','requires_action','created');
CREATE INDEX idx_intents_status ON payment_intents (org_id, status, created_at);

CREATE TABLE payments (
    payment_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    intent_id       uuid NOT NULL REFERENCES payment_intents(intent_id),
    amount_minor    bigint NOT NULL CHECK (amount_minor > 0),
    currency        char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    provider_ref    text,
    status          text NOT NULL DEFAULT 'captured' CHECK (status IN ('captured','refunded','partially_refunded')),
    captured_at     timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_payments_intent ON payments (intent_id);
CREATE INDEX idx_payments_org ON payments (org_id, captured_at DESC);

CREATE TABLE refunds (
    refund_id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    payment_id      uuid NOT NULL REFERENCES payments(payment_id),
    refund_no       text NOT NULL,
    amount_minor    bigint NOT NULL CHECK (amount_minor > 0),
    currency        char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    reason          text,
    status          text NOT NULL DEFAULT 'requested' CHECK (status IN ('requested','processed','failed')),
    provider_ref    text,
    created_by      uuid REFERENCES users(user_id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, refund_no)
);

CREATE TABLE reconciliation_issues (
    issue_id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    provider        text NOT NULL,
    date            date NOT NULL,
    expected_minor  bigint NOT NULL,
    reported_minor  bigint NOT NULL,
    status          text NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved')),
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Financing (external regulated partners; Tirek facilitates only)
-- ---------------------------------------------------------------------------

CREATE TABLE financing_applications (
    application_id  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    invoice_id      uuid NOT NULL REFERENCES invoices(invoice_id),
    product         text NOT NULL CHECK (product IN ('invoice_factoring','bnpl','credit_line')),
    amount_minor    bigint NOT NULL CHECK (amount_minor > 0),
    currency        char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    term_days       int NOT NULL CHECK (term_days > 0),
    partner         text NOT NULL,
    partner_ref     text,
    status          text NOT NULL DEFAULT 'submitted' CHECK (status IN ('submitted','under_review','approved','rejected','cancelled')),
    created_by      uuid REFERENCES users(user_id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_fin_app_invoice ON financing_applications (invoice_id) WHERE status IN ('submitted','under_review','approved');

CREATE TABLE financing_agreements (
    agreement_id    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    application_id  uuid NOT NULL REFERENCES financing_applications(application_id),
    partner_ref     text NOT NULL,
    apr_bps         int NOT NULL CHECK (apr_bps >= 0),
    fee_minor       bigint NOT NULL CHECK (fee_minor >= 0),
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active','repaid','defaulted','cancelled')),
    funded_at       timestamptz,
    repaid_at       timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_fin_agreements_org ON financing_agreements (org_id, status);

CREATE TABLE repayment_entries (
    repayment_id    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    agreement_id    uuid NOT NULL REFERENCES financing_agreements(agreement_id) ON DELETE CASCADE,
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    installment_no  int NOT NULL,
    principal_minor bigint NOT NULL CHECK (principal_minor >= 0),
    fee_minor       bigint NOT NULL CHECK (fee_minor >= 0),
    due_date        date NOT NULL,
    status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','settled','overdue','defaulted')),
    partner_ref     text,
    settled_at      timestamptz,
    UNIQUE (agreement_id, installment_no)
);

-- ---------------------------------------------------------------------------
-- Ledger (double-entry, immutable)
-- ---------------------------------------------------------------------------

CREATE TABLE ledger_accounts (
    account_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code            text NOT NULL UNIQUE,
    name            text NOT NULL,
    type            text NOT NULL CHECK (type IN ('asset','liability','revenue','expense')),
    normal_side     text NOT NULL CHECK (normal_side IN ('debit','credit'))
);

CREATE TABLE journal_entries (
    entry_id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    type            text NOT NULL CHECK (type IN ('invoice_accrual','payment_capture','settlement','gateway_fee','refund','financing_funding','financing_repayment','financing_fee','reversal','adjustment')),
    reference_type  text NOT NULL,
    reference_id    uuid NOT NULL,
    currency        char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    description     text,
    created_by      uuid,
    posted_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_journal_ref ON journal_entries (org_id, reference_type, reference_id);
CREATE INDEX idx_journal_time ON journal_entries (org_id, posted_at);

CREATE TABLE journal_lines (
    line_id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entry_id        uuid NOT NULL REFERENCES journal_entries(entry_id) ON DELETE CASCADE,
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    account_id      uuid NOT NULL REFERENCES ledger_accounts(account_id),
    side            text NOT NULL CHECK (side IN ('debit','credit')),
    amount_minor    bigint NOT NULL CHECK (amount_minor > 0),
    currency        char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    posted_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_journal_lines_entry ON journal_lines (org_id, entry_id);
CREATE INDEX idx_journal_lines_account ON journal_lines (org_id, account_id, posted_at) INCLUDE (amount_minor);

REVOKE UPDATE, DELETE ON journal_lines FROM PUBLIC;

-- Deferred trigger: every journal entry must balance (Σ debits = Σ credits)
CREATE OR REPLACE FUNCTION enforce_journal_balance() RETURNS trigger AS $$
DECLARE
    d bigint; c bigint;
BEGIN
    SELECT COALESCE(SUM(amount_minor) FILTER (WHERE side = 'debit'), 0),
           COALESCE(SUM(amount_minor) FILTER (WHERE side = 'credit'), 0)
      INTO d, c
      FROM journal_lines WHERE entry_id = NEW.entry_id;
    IF d <> c THEN
        RAISE EXCEPTION 'journal entry % does not balance: debits % credits %', NEW.entry_id, d, c;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_journal_balance
    AFTER INSERT OR UPDATE ON journal_lines
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION enforce_journal_balance();

-- ---------------------------------------------------------------------------
-- Notifications
-- ---------------------------------------------------------------------------

CREATE TABLE notifications (
    notification_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    user_id         uuid REFERENCES users(user_id),
    channel         text NOT NULL CHECK (channel IN ('email','sms','push')),
    subject         text,
    body            text NOT NULL,
    status          text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','sent','failed')),
    provider_ref    text,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notifications_org ON notifications (org_id, created_at DESC);

CREATE TABLE notification_preferences (
    user_id         uuid PRIMARY KEY REFERENCES users(user_id) ON DELETE CASCADE,
    email_enabled   boolean NOT NULL DEFAULT true,
    sms_enabled     boolean NOT NULL DEFAULT true,
    push_enabled    boolean NOT NULL DEFAULT false
);

-- ---------------------------------------------------------------------------
-- RLS: enable on every tenant-scoped table; policy compares org_id with the
-- app.tenant_id session setting (set by API middleware from the JWT).
-- ---------------------------------------------------------------------------

DO $$
DECLARE t text;
BEGIN
    FOR t IN SELECT tablename FROM pg_tables
        WHERE schemaname = 'public'
          AND tablename IN (
            'memberships','roles','restaurants','outlets','suppliers','supplier_contacts',
            'psp_accounts','catalog_categories','catalog_products','catalog_prices',
            'rfqs','rfq_items','purchase_orders','po_items','orders',
            'invoices','invoice_items','payment_intents','payments','refunds',
            'reconciliation_issues','financing_applications','financing_agreements',
            'repayment_entries','journal_entries','journal_lines','notifications',
            'audit_logs','idempotency_keys','api_keys')
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format(
            'CREATE POLICY %I ON %I USING (org_id = current_setting(''app.tenant_id'', true)::uuid)',
            'tenant_isolation_' || t, t);
    END LOOP;
END $$;

-- ---------------------------------------------------------------------------
-- Seed: permissions, roles, chart of accounts
-- ---------------------------------------------------------------------------

INSERT INTO permissions (permission_id, description) VALUES
 ('orders.create',        'Create and send purchase orders'),
 ('orders.confirm',       'Confirm or cancel purchase orders'),
 ('catalog.manage',       'Manage product catalog and prices'),
 ('invoices.issue',       'Issue supplier invoices'),
 ('invoices.approve',     'Approve invoices for payment'),
 ('payments.create',      'Create payment intents'),
 ('payments.approve',     'Approve and capture payments'),
 ('payments.refund',      'Process refunds'),
 ('finance.request',      'Request financing'),
 ('ledger.read',          'Read ledger and statements'),
 ('audit.read',           'Read audit logs'),
 ('members.manage',       'Manage members and roles'),
 ('settings.manage',      'Manage organization settings'),
 ('org.owner',            'All owner-level operations');

INSERT INTO organizations (org_id, name, type) VALUES
 ('00000000-0000-0000-0000-000000000001', 'System', 'buyer');

INSERT INTO roles (role_id, org_id, name, is_system) VALUES
 ('00000000-0000-0000-0000-000000000101', '00000000-0000-0000-0000-000000000001', 'owner', true),
 ('00000000-0000-0000-0000-000000000102', '00000000-0000-0000-0000-000000000001', 'admin', true),
 ('00000000-0000-0000-0000-000000000103', '00000000-0000-0000-0000-000000000001', 'procurement', true),
 ('00000000-0000-0000-0000-000000000104', '00000000-0000-0000-0000-000000000001', 'accountant', true),
 ('00000000-0000-0000-0000-000000000105', '00000000-0000-0000-0000-000000000001', 'finance', true),
 ('00000000-0000-0000-0000-000000000106', '00000000-0000-0000-0000-000000000001', 'viewer', true);

INSERT INTO role_permissions (role_id, permission_id) VALUES
 ('00000000-0000-0000-0000-000000000101', 'org.owner'),
 ('00000000-0000-0000-0000-000000000102', 'orders.create'),
 ('00000000-0000-0000-0000-000000000102', 'orders.confirm'),
 ('00000000-0000-0000-0000-000000000102', 'catalog.manage'),
 ('00000000-0000-0000-0000-000000000102', 'invoices.approve'),
 ('00000000-0000-0000-0000-000000000102', 'payments.approve'),
 ('00000000-0000-0000-0000-000000000102', 'payments.refund'),
 ('00000000-0000-0000-0000-000000000102', 'finance.request'),
 ('00000000-0000-0000-0000-000000000102', 'ledger.read'),
 ('00000000-0000-0000-0000-000000000102', 'audit.read'),
 ('00000000-0000-0000-0000-000000000102', 'members.manage'),
 ('00000000-0000-0000-0000-000000000102', 'settings.manage'),
 ('00000000-0000-0000-0000-000000000103', 'orders.create'),
 ('00000000-0000-0000-0000-000000000103', 'catalog.manage'),
 ('00000000-0000-0000-0000-000000000104', 'invoices.approve'),
 ('00000000-0000-0000-0000-000000000104', 'ledger.read'),
 ('00000000-0000-0000-0000-000000000105', 'finance.request'),
 ('00000000-0000-0000-0000-000000000105', 'ledger.read'),
 ('00000000-0000-0000-0000-000000000106', 'ledger.read');

INSERT INTO ledger_accounts (account_id, code, name, type, normal_side) VALUES
 ('00000000-0000-0000-0000-000000000201', '1000', 'Cash (PSP settlements)',      'asset',      'debit'),
 ('00000000-0000-0000-0000-000000000202', '1100', 'PSP receivables',             'asset',      'debit'),
 ('00000000-0000-0000-0000-000000000203', '1200', 'Buyer receivables (financing)','asset',     'debit'),
 ('00000000-0000-0000-0000-000000000204', '2000', 'Supplier payables',           'liability',  'credit'),
 ('00000000-0000-0000-0000-000000000205', '2100', 'Refund liability',            'liability',  'credit'),
 ('00000000-0000-0000-0000-000000000206', '2200', 'Financing payable',           'liability',  'credit'),
 ('00000000-0000-0000-0000-000000000207', '4000', 'Platform fee revenue',        'revenue',    'credit'),
 ('00000000-0000-0000-0000-000000000208', '5000', 'PSP gateway fees',            'expense',    'debit');