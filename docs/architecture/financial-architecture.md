# Tirek — Financial Architecture

Version: 1.0 (MVP)
Status: Approved

> Tirek is a **facilitator**, never a bank, PSP, or lender. Regulated
> financial operations (holding funds, payment processing, credit origination)
> are delegated to licensed partners. This document defines how Tirek's
> systems stay financially correct in that role.

## 1. Money representation

- **All money is integer minor units (`BIGINT`) + ISO 4217 currency code
  (`CHAR(3)`)**. No `float`, no `numeric` in application code (ADR-009).
  - KZT has 100 tiyn per tenge → `1 000.00 ₸` = `100000` minor units.
- Sums, percentages, tax calculations use integer arithmetic with explicit
  rounding functions; rounding happens **once, at the smallest unit**, at the
  boundary where the total is derived (e.g. VAT = round_half_up(total × rate,
  0) — defined per-currency rules in `platform/money`).
- Formatting (currency symbols, decimal places) is a **presentation-layer**
  concern only.
- Currencies supported at MVP: `KZT` (primary), plus `USD`/`EUR` for future
  export — the money library is currency-agnostic from day one.

## 2. Double-entry ledger

Every money movement produces a journal entry **in the same DB transaction**
as the state change (single-writer consistency).

### Chart of accounts (global, immutable)

| Code | Account | Type | Notes |
|---|---|---|---|
| 1000 | Cash (PSP settlements) | Asset | increases on settlement reports |
| 1100 | PSP receivables | Asset | money owed to Tirek by PSPs (captured not yet settled) |
| 1200 | Buyer receivables (financing) | Asset | claim on buyer when lender funds |
| 2000 | Supplier payables | Liability | money owed to suppliers (settlements pending) |
| 2100 | Refund liability | Liability | captured payments awaiting refund execution |
| 2200 | Financing payable | Liability | obligation to repay lender (facilitation flow) |
| 4000 | Platform fee revenue | Revenue | service fees on orders/financing |
| 5000 | PSP gateway fees | Expense | processing fees charged by PSP |

### Tables

- `ledger_accounts(account_id, code, name, type, normal_side)` — seed data,
  immutable.
- `journal_entries(entry_id, org_id, type, reference_type, reference_id,
  currency, posted_at, description, created_by)` — reference_id is the
  invoice/payment/refund/agreement UUID; full traceability.
- `journal_lines(line_id, entry_id, org_id, account_id, side, amount_minor,
  currency)` — **immutable**: `REVOKE UPDATE, DELETE` at DB level. Corrections
  are new reversal entries (debit/credit swapped), never edits.

### Invariants

1. Every entry balances: `Σ debits = Σ credits` (checked by `CHECK` via a
   deferred trigger).
2. Balances are **derived** by summing lines — no stored balances to corrupt.
   Read-side account statements aggregate `journal_lines` with a covering
   index. (If and only if a measured bottleneck appears: materialize monthly
   closing balances with a `journal_closings` table, still derived.)
3. Negative money is impossible: `CHECK (amount_minor >= 0)`; direction is
   expressed by side + account type.
4. A line references exactly one entry; an entry references exactly one
   business document (invoice, payment, refund, financing agreement).
5. Ledger rows are written only by the `ledger` module's service methods —
   no other module touches these tables directly (module boundary).

## 3. Order → invoice → payment flow

```mermaid
sequenceDiagram
    autonumber
    actor Buyer as Buyer (web app)
    participant API as tirek-api
    participant DB as PostgreSQL
    participant W as tirek-worker
    participant PSP as PSP (Kaspi Pay)
    actor Supplier as Supplier

    Buyer->>API: POST /v1/orgs/{id}/purchase-orders (Idempotency-Key)
    API->>DB: create PO (draft) + audit + outbox(notify)
    W->>Supplier: notification: PO sent
    Supplier->>API: confirm PO
    API->>DB: PO confirmed; outbox(notify)
    Note over Supplier,API: goods delivered (offline / delivery note)
    Supplier->>API: POST /v1/orgs/{id}/invoices (for PO)
    API->>DB: invoice issued (draft) + audit
    Buyer->>API: POST invoices/{id}/approve
    API->>DB: invoice approved
    API->>DB: journal entry #1: Dr 5000? No—see note
    Note over API,DB: Accrual: Dr 2000 Supplier payables? See §4.1
    Buyer->>API: POST /v1/orgs/{id}/payment-intents (Idempotency-Key)
    API->>DB: intent created (pending_psp); journal: accrual
    API-->>W: outbox: psp.capture
    W->>PSP: create payment (redirect/QR)
    PSP-->>W: payment_url
    W->>DB: intent → requires_action
    W-->>Buyer: payment_url (QR/redirect)
    Buyer->>PSP: completes payment (card/QR)
    PSP->>API: POST /v1/webhooks/kaspi (HMAC, event_id)
    API->>DB: webhook_events (dedupe) + outbox: psp.capture.confirmed
    W->>PSP: GET status (verify provider-side, double check)
    W->>DB: intent → paid; payment captured
    W->>DB: journal entry #2: Dr 1100 PSP receivables, Cr 2000 Supplier payables
    W->>DB: journal entry #3: Dr 2000, Cr 1100 (settlement accrual), Dr 5000 gateway fee
    W->>DB: invoice → paid; outbox(notify + receipt)
    W-->>Buyer: receipt email
    W-->>Supplier: payout scheduled (via PSP payout/QR)
    Supplier->>PSP: receives funds
```

## 4. Ledger entries for the core lifecycle

| Event | Entry (Dr / Cr) | Note |
|---|---|---|
| Invoice approved (accrual) | Dr 2000 Supplier payables · Cr 1100 PSP receivables | mirrors future settlement path |
| Payment captured | Dr 1100 PSP receivables · Cr 2000 Supplier payables | increases both sides, balance preserved |
| Settlement (PSP report) | Dr 2000 · Cr 1100 | clears receivable/payable pair |
| Gateway fee | Dr 5000 PSP gateway fees · Cr 1100 | fee reduces settlement |
| Refund issued | Dr 2100 Refund liability · Cr 1100 | liability recognized |
| Refund executed | Dr 1100 · Cr 2100 | cleared |
| Financing: lender funds supplier | Dr 1200 Buyer receivables · Cr 2200 Financing payable | facilitation claim |
| Financing: buyer repays | Dr 2200 · Cr 1200 | claim cleared |
| Financing: facilitation fee | Dr 1200 · Cr 4000 Platform fee revenue | recognized at repayment |

All entries are auto-generated by the `payments`/`financing` modules calling
the `ledger` module's posting API — the modules never hand-write SQL against
ledger tables.

## 5. Refunds and fees

- Refunds are full or partial, never exceed captured amount
  (`SELECT ... FOR UPDATE` on the payment row before posting).
- Refund flow: `refunds` row (unique `refund_no`) → PSP refund request via
  adapter → PSP webhook → mark refunded → journal entries above (Dr 2100 /
  Cr 1100), **plus reversal of the original fee** (Dr 1100 / Cr 5000 for the
  proportional gateway fee).
- Fees: platform fee charged on orders (defined per org/plan), posted when
  payment captures. Fee amount computed in minor units with the same rounding
  rules as the order totals; fee rows live in `journal_entries` with
  `reference_type = 'payment'`, never hidden inside totals.
- A refund never deletes or edits prior lines — only reversal entries.

## 6. Idempotency and concurrency

- **Payment intents**: unique `idempotency_key` per intent; creating an
  intent for the same invoice twice returns the existing intent (200) instead
  of a duplicate.
- **Webhooks**: `(provider, event_id)` unique index → replays are 200-no-op.
- **Outbox handlers**: dedupe key `(topic, entity_id)`; workers use
  `FOR UPDATE SKIP LOCKED` and at-least-once semantics — handlers are
  idempotent (verify state before applying; e.g. `capture` only when intent
  is `pending_psp`).
- **Locking**: single-writer via Postgres row locks (`FOR UPDATE`) on the
  intent/invoice rows during state transitions; no lost updates. Optimistic
  `version` column on `invoices`/`agreements` for the read-modify-write UI
  flows (409 on stale writes).

## 7. Webhook handling (inbound, provider → Tirek)

1. **Verify** HMAC-SHA256 over raw body with provider secret (per provider).
2. **Record** `webhook_events(provider, event_id, payload)` — unique index
   drops duplicates.
3. **Respond** 200 immediately (providers are timeout-sensitive).
4. **Process** via outbox: `psp.capture.confirmed` handler re-reads provider
   state (GET status on PSP) before applying `paid` — the second source
   prevents trusting a forged-but-signed edge case.
5. **Reconcile**: nightly job compares PSP reports/ledger vs Tirek
   `payments` table; mismatches alert finance team.

## 8. Financing flow (external partner)

```mermaid
sequenceDiagram
    autonumber
    actor Buyer as Buyer (restaurant)
    participant API as tirek-api
    participant DB as PostgreSQL
    participant W as tirek-worker
    participant P as Financing partner (bank/BNPL/factoring)
    actor Supplier as Supplier

    Note over Buyer,Supplier: Invoice approved (eligible, e.g. ≥ 3 paid invoices history)
    Buyer->>API: POST /v1/orgs/{id}/financing/applications
    API->>DB: application submitted + audit
    API-->>W: outbox: finance.submit
    W->>P: submit application (amount, term, invoice, KYC snapshot)
    P-->>W: under_review (partner ref)
    W->>DB: application → under_review
    P->>API: POST /v1/webhooks/finance (HMAC) — decision
    API->>DB: webhook recorded → outbox: finance.decision
    W->>P: GET application status (verify)
    alt approved
        W->>P: confirm funding
        P-->>Supplier: funds supplier (fees/APR per partner terms)
        P->>API: webhook: funded
        W->>DB: agreement active; repayment schedule created
        W->>DB: journal: Dr 1200 / Cr 2200 (facilitation claims)
        W-->>Buyer: financing active + schedule
        loop each installment due
            Buyer->>API: repay (or auto-debit via partner)
            P->>API: webhook: repayment received
            W->>DB: repayment_entries settled
            W->>DB: journal: Dr 2200 / Cr 1200 (+ fee revenue)
            W-->>Buyer: installment confirmation
        end
    else rejected
        W->>DB: application → rejected; notify buyer
    end
```

Guiding rules:

- Tirek never extends credit; the partner takes credit risk, sets APR, and
  funds the supplier. Tirek collects a **facilitation fee** (per agreement).
- Repayment schedules mirror the partner's plan 1:1 (`repayment_entries`);
  partner webhooks are the source of truth for payments received.
- KYC/regulatory data is captured by the partner; Tirek stores only the
  minimal business data needed to operate the agreement.
- Defaults are reported by the partner; Tirek marks the agreement
  `defaulted`, posts a ledger note entry, and stops facilitating.

## 9. Reconciliation and auditability

- **Reconciliation ledger**: nightly job pulls PSP settlement reports → posts
  settlement entries (Dr 2000 / Cr 1100) → compares per-day totals vs
  captured payments; discrepancies → `reconciliation_issues` + alert.
- **Audit**: every financial mutation also writes `audit_logs` (actor, action,
  before/after). The journal itself is the financial audit trail —
  immutable, referenceable, regulator-friendly.
- **Export**: quarterly ledger export (CSV, checksummed) for accountant use;
  retained per retention policy.
- **Money movement provenance**: `journal_entries.reference_id` always points
  to the originating document — "why does this money exist" is answerable
  from the database alone.

## 10. MVP scope limits (accepted risks)

| Limit | Mitigation |
|---|---|
| No PCI scope — cards handled by PSP pages only | PSA-DSS responsibility at partner; Tirek stores no PAN |
| Single payout model (PSP QR/payout to supplier) | Adapter interface supports bank-transfer payouts later |
| No multi-currency cross-payment at launch | Money lib is currency-aware; enforce one currency per intent |
| No auto-debit for financing at launch | Partner handles collection; Tirek tracks via webhooks |
| Fraud tooling is manual (flags + review) | Reconciliation job + alerts; partner-side risk engine |