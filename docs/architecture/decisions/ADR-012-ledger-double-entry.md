# ADR-012: Double-Entry Ledger with Immutable Postings

**Status:** Accepted · **Date:** 2026-08-17

## Decision
All money movements flow through a **double-entry ledger**: global chart of
accounts (`ledger_accounts`), `journal_entries` (headers referencing the
business document), and `journal_lines` (immutable debit/credit lines,
`REVOKE UPDATE, DELETE`). Balances are derived by summation; corrections are
new reversal entries. Posting happens in the same DB transaction as the
state change.

## Reason
- Double-entry guarantees the invariant Σ debits = Σ credits (checked by a
  deferred trigger) — a hard, automatic check that single-entry accounting
  never provides.
- Immutability gives the regulator-grade audit trail financiers expect:
  every entry is traceable to its source document (`reference_id`).
- Writing in the same transaction as the business state change eliminates
  drift between "what happened" and "what the books say".

## Rejected alternatives
- **Event-sourcing the ledger (all financial events as a stream)** — more
  powerful, but adds replay complexity, projection drift, and harder
  debugging for an MVP; the outbox already gives an event trail. Revisit if
  analytics needs full event history.
- **Single-entry money tracking tables** — no balance invariant, no audit
  story; unacceptable for a payments/financing product.
- **External ledger service (e.g. a "ledger as a service" API)** — data
  leaves the platform and adds a hard external dependency for the core
  domain.