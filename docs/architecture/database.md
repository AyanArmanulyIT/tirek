# Tirek — Database Architecture

Version: 1.0 (MVP)
Status: Approved

## 1. Overview

- **PostgreSQL 16** on AWS RDS (Multi-AZ in production, single-AZ staging).
- Schema: shared-schema multi-tenancy with **Row-Level Security (RLS)**.
- Migration tool: plain SQL files, applied by `cmd/migrate` (versioned in
  `backend/migrations/`), executed atomically inside transactions.
- Query layer: **sqlc** — SQL is source of truth, generated Go code is
  compile-time checked. No hand-written SQL strings in the app.
- IDs: **UUIDv7** (time-ordered, index-friendly) generated in the database via
  the `pgcrypto` extension (`gen_random_uuid()` time-ordered variant) or by the
  app; all PKs are UUID.
- Money: `BIGINT` minor units + `CHAR(3)` currency. **No `numeric`/`float` for
  money in application code** (see ADR-009).

## 2. Schema conventions

| Convention | Rule |
|---|---|
| Tables | `snake_case`, plural; tenant-scoped tables prefixed by module name where ambiguous (`catalog_*`, `journal_*`) |
| PK | `uuid` PK named `<table>_id`, default UUIDv7 |
| Tenant FK | `org_id uuid NOT NULL REFERENCES organizations(org_id)` on every tenant-scoped table |
| Timestamps | `created_at timestamptz NOT NULL DEFAULT now()`, `updated_at timestamptz NOT NULL DEFAULT now()` (updated by trigger where needed) |
| Money | `amount_minor bigint NOT NULL CHECK (amount_minor >= 0)` + `currency char(3)`; check currency in `iso_4217` set where cheap |
| Status | `text` with `CHECK` constraint enumerating allowed values (or `enum` where the app never extends it) |
| Soft delete | Never for financial rows. `archived_at timestamptz` for catalog/suppliers only |
| Indexes | Created in migration files, named `idx_<table>_<cols>` |

## 3. Multi-tenancy (RLS)

- Application sets `app.tenant_id` (and `app.role`) per request via
  `SET LOCAL` in a transaction wrapper (values come from the verified JWT).
- Every tenant-scoped table has:

```sql
ALTER TABLE invoices ENABLE ROW LEVEL SECURITY;
CREATE POLICY invoices_tenant_isolation ON invoices
  USING (org_id = current_setting('app.tenant_id')::uuid);
```

- The API connects as a **single low-privilege role** (`tirek_app`); all
  tenant enforcement is RLS + application checks (defense in depth).
- `app.tenant_id` is always set by middleware before any query runs; a
  startup check (`SET LOCAL` then `SELECT current_setting`) verifies the
  value is present — otherwise the request fails closed.
- Migration user (`tirek_migrator`) bypasses RLS (`BYPASSRLS`).
- Cross-tenant tables (e.g. `catalog_products` are supplier-scoped, so all
  tenant data is covered; truly global tables: `users`, `ledger_accounts`,
  `roles`, `permissions`, `audit_logs` carry `org_id` nullable/global).

## 4. Core tables and relationships

See [domain-model.md](domain-model.md) for the ER diagram. Highlights:

```text
organizations (tenant root)
├── memberships ── users
├── restaurants ── outlets
├── suppliers ── supplier_contacts ── psp_accounts
├── rfqs ─ rfq_items
├── purchase_orders ─ po_items ── orders
├── invoices ─ invoice_items ── payment_intents ─ payments ─ refunds
├── financing_applications ─ financing_agreements ─ repayment_entries
├── notifications
└── audit_logs

ledger_accounts ─ journal_entries ─ journal_lines   (org-scoped)
outbox_events, webhook_events, idempotency_keys     (platform)
```

## 5. Important constraints

```sql
-- Money sanity
CHECK (amount_minor >= 0)
CHECK (currency ~ '^[A-Z]{3}$')

-- A payment belongs to exactly one captured intent
CREATE UNIQUE INDEX uq_payments_intent ON payments(payment_intent_id) WHERE status = 'captured';

-- Idempotency: an idempotency key maps 1:1 to one operation
CREATE UNIQUE INDEX uq_idempotency ON idempotency_keys(org_id, key, endpoint);

-- Webhook deduplication
CREATE UNIQUE INDEX uq_webhook_events ON webhook_events(provider, event_id);

-- Outbox exactly-once delivery per event
CREATE UNIQUE INDEX uq_outbox_once ON outbox_events(id, status)
  WHERE status IN ('pending', 'processing');

-- Refunds cannot exceed captured amount (enforced in app with FOR UPDATE)
CREATE UNIQUE INDEX uq_refund_payment ON refunds(payment_id, refund_no);

-- Journal lines: no two lines with same entry+account+side (prevents duplicate postings)
CREATE UNIQUE INDEX uq_journal_line ON journal_lines(journal_entry_id, account_id, side);

-- Invoice paid only once via full payment
CREATE UNIQUE INDEX uq_invoice_full_payment ON payment_intents(invoice_id)
  WHERE kind = 'full' AND status = 'paid';
```

## 6. Important indexes

| Table | Index | Purpose |
|---|---|---|
| `journal_lines` | `(org_id, account_id, posted_at)` | Account statements / aging reports |
| `journal_lines` | `(org_id, journal_entry_id)` | Entry reconstruction |
| `invoices` | `(org_id, supplier_id, status)` | Supplier invoice lists |
| `invoices` | `(org_id, due_date)` | Payment-due scans (worker) |
| `purchase_orders` | `(org_id, outlet_id, created_at desc)` | Buyer order lists |
| `payment_intents` | `(org_id, status, created_at)` | Reconciliation scans |
| `outbox_events` | `(status, scheduled_at) where status in (pending, retry)` | Worker polling |
| `webhook_events` | `(status, created_at)` | Webhook reprocessing |
| `financing_agreements` | `(org_id, status)` | Portfolio/status views |
| `catalog_products` | `(supplier_id, active)` | Catalog browsing |
| `audit_logs` | `(org_id, created_at desc)` | Audit browsing |

## 7. Outbox (async work)

`outbox_events(id, org_id, topic, payload jsonb, status, attempts, scheduled_at, created_at)`:

- Every state-changing transaction that must trigger async work (notify,
  call PSP, call financing partner, send receipt) inserts a row **in the same
  transaction** as the business change.
- `tirek-worker` polls `status = 'pending'` ordered by `scheduled_at` with
  `FOR UPDATE SKIP LOCKED`, processes with a **dedupe key** (`topic` +
  `entity_id`), retries with exponential backoff, and dead-letters after N
  attempts.
- At-least-once semantics: handlers are idempotent (see
  [financial-architecture.md](financial-architecture.md#idempotency)).

## 8. Ledger tables

See [financial-architecture.md](financial-architecture.md#ledger-design) for
the double-entry design. Tables: `ledger_accounts`, `journal_entries`,
`journal_lines` (immutable, append-only; `REVOKE UPDATE, DELETE` on
`journal_lines` at DB level as a final guard).

## 9. Backups

- RDS automated backups: 7-day retention + PITR (production).
- Weekly manual snapshot exported to a separate region (S3 + cross-account)
  for disaster recovery (production).
- S3 bucket versioning for object storage.
- Restore drill executed in staging at least once per quarter.

## 10. Migrations workflow

- `backend/migrations/NNNN_description.sql`, strictly ordered.
- `cmd/migrate` applies pending migrations in a transaction with an advisory
  lock (safe under concurrent deploys), records applied version in
  `schema_migrations`.
- CI runs migrations against a throwaway Postgres (testcontainers) on every PR.
- Staging and production apply via the deploy job, never manually.