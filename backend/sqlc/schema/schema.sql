-- sqlc/schema — codegen view of the Tirek schema.
-- This is a parseable mirror of the tables defined in backend/migrations
-- (0001_init.sql, 0002_identity.sql, 0003_restaurants_suppliers.sql). The
-- migrations remain the source of truth for the live database; keep this file
-- in sync with them.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE organizations (
    org_id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL,
    type            text NOT NULL CHECK (type IN ('buyer','supplier')),
    country         char(2) NOT NULL DEFAULT 'KZ',
    default_currency char(3) NOT NULL DEFAULT 'KZT',
    bin             text,
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','closed')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    user_id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email           text NOT NULL,
    password_hash   text NOT NULL,
    full_name       text NOT NULL,
    phone           text,
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    default_org_id  uuid REFERENCES organizations(org_id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_users_email ON users (lower(email));

CREATE TABLE sessions (
    session_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    org_id          uuid REFERENCES organizations(org_id),
    refresh_hash    text NOT NULL,
    expires_at      timestamptz NOT NULL,
    revoked_at      timestamptz,
    revoked_reason  text,
    last_used_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    ip              inet,
    user_agent      text
);
CREATE INDEX idx_sessions_user ON sessions (user_id);
CREATE INDEX idx_sessions_expiry ON sessions (expires_at);
CREATE INDEX idx_sessions_refresh_hash ON sessions (refresh_hash);

CREATE TABLE session_refresh_history (
    id            bigserial PRIMARY KEY,
    session_id    uuid NOT NULL REFERENCES sessions(session_id) ON DELETE CASCADE,
    refresh_hash  text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_session_refresh_history_hash ON session_refresh_history (refresh_hash);

CREATE TABLE auth_events (
    event_id    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid REFERENCES users(user_id) ON DELETE SET NULL,
    org_id      uuid REFERENCES organizations(org_id) ON DELETE SET NULL,
    action      text NOT NULL,
    identifier  text,
    ip          inet,
    user_agent  text,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_auth_events_user_time ON auth_events (user_id, created_at DESC);
CREATE INDEX idx_auth_events_ip_time   ON auth_events (ip, created_at DESC);

CREATE TABLE permissions (
    permission_id   text PRIMARY KEY,
    description     text NOT NULL
);

CREATE TABLE roles (
    role_id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    name            text NOT NULL,
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

CREATE TABLE restaurants (
    restaurant_id    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    name             text NOT NULL,
    legal_name       text,
    bin              text,
    status           text NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','closed')),
    country          char(2) NOT NULL DEFAULT 'KZ',
    default_currency char(3) NOT NULL DEFAULT 'KZT' CHECK (default_currency ~ '^[A-Z]{3}$'),
    phone            text,
    email            text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_restaurants_org ON restaurants (org_id);

CREATE TABLE outlets (
    outlet_id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id   uuid NOT NULL REFERENCES restaurants(restaurant_id) ON DELETE CASCADE,
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    name            text NOT NULL,
    address         text NOT NULL,
    city            text NOT NULL,
    country         char(2) NOT NULL DEFAULT 'KZ',
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive')),
    phone           text,
    email           text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_outlets_org        ON outlets (org_id);
CREATE INDEX idx_outlets_restaurant ON outlets (restaurant_id);

CREATE TABLE suppliers (
    supplier_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    name             text NOT NULL,
    legal_name       text,
    bin              text,
    payout_account   text,
    payment_terms    text NOT NULL DEFAULT 'net14',
    rating           numeric(3,2),
    status           text NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','closed')),
    country          char(2) NOT NULL DEFAULT 'KZ',
    default_currency char(3) NOT NULL DEFAULT 'KZT' CHECK (default_currency ~ '^[A-Z]{3}$'),
    phone            text,
    email            text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_suppliers_org ON suppliers (org_id);
CREATE INDEX idx_suppliers_org ON suppliers (org_id);

CREATE TABLE supplier_contacts (
    contact_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    supplier_id     uuid NOT NULL REFERENCES suppliers(supplier_id) ON DELETE CASCADE,
    org_id          uuid NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    full_name       text NOT NULL,
    phone           text,
    email           text,
    role            text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

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