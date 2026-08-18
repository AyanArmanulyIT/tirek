-- 0002_identity.sql — Identity module: sessions, refresh-token rotation/reuse,
-- auth event audit, tirek_app role grants, brute-force protection indexes.
--
-- Sessions gain a tenant (org_id) and refresh-token rotation tracking so the
-- JWT can be re-issued without re-reading memberships and reuse detection can
-- revoke a session when a rotated token is presented again.
--
-- The `tirek_app` role is created by infrastructure (docker-compose init
-- scripts locally, Secrets Manager/RDS on AWS) BEFORE migrations run; this
-- file only grants privileges to it and sets up default privileges for tables
-- created by later migrations (run as the migration owner).

-- ---------------------------------------------------------------------------
-- Users: remember the user's default (login) organization.
-- ---------------------------------------------------------------------------

ALTER TABLE users
    ADD COLUMN default_org_id uuid REFERENCES organizations(org_id);

-- ---------------------------------------------------------------------------
-- Sessions: tenant context + rotation bookkeeping.
-- ---------------------------------------------------------------------------

ALTER TABLE sessions
    ADD COLUMN org_id uuid REFERENCES organizations(org_id),
    ADD COLUMN last_used_at timestamptz,
    ADD COLUMN revoked_reason text;

CREATE INDEX idx_sessions_refresh_hash ON sessions (refresh_hash);

-- Every rotated-out refresh hash is recorded here. When a token that is no
-- longer the session's current hash is presented, we look it up here: a match
-- means the token was reused (stolen) and the whole session is revoked.
CREATE TABLE session_refresh_history (
    id           bigserial PRIMARY KEY,
    session_id   uuid NOT NULL REFERENCES sessions(session_id) ON DELETE CASCADE,
    refresh_hash text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_session_refresh_history_hash ON session_refresh_history (refresh_hash);

-- ---------------------------------------------------------------------------
-- Auth event audit (identity module). Append-only in practice; used for the
-- audit trail and brute-force protection (login_failed counting).
-- ---------------------------------------------------------------------------

CREATE TABLE auth_events (
    event_id    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid REFERENCES users(user_id) ON DELETE SET NULL,
    org_id      uuid REFERENCES organizations(org_id) ON DELETE SET NULL,
    action      text NOT NULL,   -- register | login | login_failed | refresh | refresh_reuse | logout
    identifier  text,            -- sha256(email) when the user_id is unknown (failed login)
    ip          inet,
    user_agent  text,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_auth_events_user_time ON auth_events (user_id, created_at DESC);
CREATE INDEX idx_auth_events_ip_time   ON auth_events (ip, created_at DESC);

-- ---------------------------------------------------------------------------
-- Privileges for the application role. The role itself is provisioned by
-- infrastructure; this file grants least-privilege access to schema objects.
-- ---------------------------------------------------------------------------

GRANT USAGE ON SCHEMA public TO tirek_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO tirek_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO tirek_app;

-- Tables created by later migrations (owned by the migration role) are granted
-- automatically so future modules do not need per-table GRANT statements.
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO tirek_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO tirek_app;

-- The double-entry journal stays append-only even for the app role.
REVOKE UPDATE, DELETE ON journal_lines FROM tirek_app;