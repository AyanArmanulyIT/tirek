# Tirek — Security Architecture

Version: 1.0 (MVP)
Status: Approved

## 1. Trust model

- **Users** are authenticated globally; their authorization is scoped to an
  **active organization** chosen at login and carried in the JWT.
- **Tenants are mutually untrusted**: supplier must never read a buyer's
  order details beyond its own POs; a buyer must never read another buyer's
  data. Enforced twice: at the API layer (scoped queries) and at the database
  layer (RLS) — defense in depth.
- **External providers** (PSPs, lenders) are partially trusted: they can only
  send signed webhooks to dedicated endpoints, and their claims are verified
  against provider-side state (e.g. reconcile `paid` with PSP dashboard).

## 2. Authentication

| Credential | Mechanism |
|---|---|
| Passwords | argon2id (cost tuned to server); never logged, never stored plaintext |
| Access token | JWT HS256, 15 min TTL, `sub`, `org_id`, `role`, `session_id` claims |
| Refresh token | Opaque random, stored hashed server-side, 30 days, rotation on use, reuse → revoke session |
| API keys (POS) | Random 256-bit, stored hashed, scoped to org + permissions, revoked on rotation |
| Provider webhooks | HMAC-SHA256 of raw body with per-provider secret |

- JWT secrets: 48+ bytes of CSPRNG, stored in AWS Secrets Manager, rotated
  quarterly, dual-key support (grace period) during rotation.
- All cookies: `HttpOnly; SameSite=Lax; Secure` (Secure always in production),
  path scoped to `/api`.

## 3. Authorization (RBAC)

- `permissions` are granular actions (`orders.create`, `payments.approve`,
  `invoices.manage`, `finance.request`, `audit.read`, …).
- `roles` bundle permissions; built-in roles per org type:

| Role | Buyer org | Supplier org |
|---|---|---|
| `owner` | everything | everything |
| `admin` | everything except owner-only (billing, member removal) | same |
| `procurement` | create orders, view catalog | manage catalog, confirm POs |
| `accountant` | approve invoices, view ledger | view payouts, issue invoices |
| `finance` | request financing, view agreements | — |
| `viewer` | read-only | read-only |

- Enforcement: HTTP middleware resolves role from JWT → checks permission
  against an in-memory permission map (loaded from DB, TTL cache).
- Owners can create custom roles via the RBAC API (same permission set).

## 4. Tenant isolation

1. All tenant-scoped tables carry `org_id`; RLS enabled with policy
   `org_id = current_setting('app.tenant_id')::uuid`.
2. Middleware sets `app.tenant_id` from JWT before any DB work; fail-closed if
   unset.
3. Path `org_id` must equal token `org_id`.
4. Cross-tenant references (buyer PO ↔ supplier invoice) are always resolved
   through explicit FK relationships with dual-tenant checks in the query
   (both `org_id`s in the WHERE clause).
5. RLS policies tested in CI: negative tests assert tenant A cannot read
   tenant B rows through every query path (testcontainers).

## 5. API security

- HTTPS only (ALB terminates TLS, TLS 1.2+); HSTS header.
- Security headers: `X-Content-Type-Options: nosniff`, `X-Frame-Options:
  DENY`, CSP (default-src 'self'), `Referrer-Policy`, `Permissions-Policy`.
- CORS restricted to `WEB_ORIGIN` (single origin per environment), cookies
  SameSite.
- CSRF: refresh/login use JSON bodies (no form posts); SameSite=Lax cookies;
  state-changing endpoints require the Bearer token (not cookie), which
  eliminates CSRF for the API.
- Rate limiting (Redis) per §9 of api.md; stricter on auth endpoints.
- Request size limits (1 MB bodies), upload size limits, content-type checks.
- `request_id` correlation on every request/log entry; structured logs never
  include tokens, passwords, or full card data.
- API key hashes stored, not keys.

## 6. Secrets

- **Never** in env files or repo (`.env.example` holds placeholders only).
- Runtime: AWS Secrets Manager (DB creds, JWT secrets, provider keys,
  webhook secrets) with rotation Lambda or manual rotation runbook.
- RDS credentials: IAM-auth-capable; MVP uses Secrets Manager rotation.
- Local dev: `.env` (gitignored), docker-compose defaults are throwaway.
- Access to Secrets Manager: IAM roles, least privilege, audit trail.

## 7. PII and data protection

- PII inventory (users' emails, phones, IIN/BIN, bank details):
  - Encryption at rest: RDS (AES-256), S3 (SSE-S3/SSE-KMS), EBS.
  - Encryption in transit: TLS everywhere.
  - Field-level encryption not needed for MVP; compensating controls are RLS +
    RBAC + audit + restricted DB roles.
- Bank/payout details: stored in PSP partner vault where possible; when
  stored locally, PII columns are `pgcrypto` `pgp_sym_encrypt` with a KMS-
  managed key (available, used for bank accounts).
- Logging: PII redacted by default in structured logs; S3 objects private +
  signed URLs.
- Retention: audit logs retained 5 years (KZ regulatory horizon); sessions
  90 days; webhook bodies 90 days; exports 30 days.
- Data deletion: account deletion anonymizes PII, retains financial records
  (regulatory) with `deleted` marker.

## 8. Audit logs

- `audit_logs`: append-only (`REVOKE UPDATE, DELETE`), written in the same
  transaction as the state change: `actor_id, org_id, action, entity_type,
  entity_id, before/after (jsonb), ip, user_agent, created_at`.
- Readable only by `owner`/`admin`/`accountant` (permission `audit.read`).
- Financial auditability is deeper: the immutable double-entry journal (see
  financial-architecture.md) records every money movement.

## 9. OWASP Top 10 (MVP checklist)

| Risk | Mitigation |
|---|---|
| A01 Broken access control | RBAC + RLS + org checks + negative tests |
| A02 Cryptographic failures | TLS 1.2+, AES-256 at rest, argon2id, HS256 JWTs, HMAC webhooks |
| A03 Injection | sqlc (parameterized), no string SQL, validator bounds |
| A04 Insecure design | threat model doc, financial invariants, idempotency |
| A05 Misconfiguration | IaC-consistent task definitions, security headers, least-privilege IAM, hardened images (non-root, distroless), dependency scanning |
| A06 Vulnerable components | Dependabot + Renovate, `govulncheck` in CI, image scans (Trivy) in CI |
| A07 Auth failures | argon2id, token rotation, reuse detection, login rate limiting |
| A08 Data integrity | HMAC webhooks, immutable journal, outbox exactly-once, checksummed exports |
| A09 Logging/monitoring | OTel + Loki, alerts on auth failures/5xx/spike anomalies |
| A10 SSRF | Webhook targets allowlisted per tenant; image fetch via signed URLs only |

## 10. Rate limiting, backups, DR

- Rate limiting: see api.md §9 (Redis token bucket).
- Backups: RDS automated (7d + PITR), weekly cross-region snapshot, S3
  versioning + replication, quarterly restore drills in staging.
- DR: RDS cross-region read replica + restore runbook; infra described in
  code (dockerfiles + task definitions) so staging rebuilds in < 1 h.
- Incident response: runbooks in `docs/runbooks/` (TBD on first incident),
  `on-call` rotation when production traffic justifies it.

## 11. Security testing cadence

- Every PR: SAST (golangci-lint security rules, govulncheck), dependency scan,
  RLS negative tests.
- Nightly: Trivy image scan; OWASP ZAP baseline against staging (weekly).
- Quarterly: secret rotation; access review; restore drill.
- Annual: external penetration test (when production revenue exists).