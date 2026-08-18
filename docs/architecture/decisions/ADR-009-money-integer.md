# ADR-009: Money — Integer Minor Units

**Status:** Accepted · **Date:** 2026-08-17

## Decision
All monetary values are stored and computed as **`BIGINT` minor units**
(1 KZT = 100 tiyn → 1.00 ₸ = 100) paired with an ISO 4217 `CHAR(3)` currency
code. **Floating point is banned for money** in the entire codebase; the
`platform/money` package is the only place that converts, rounds (half-up at
the smallest unit, once per derived total), and formats money.

## Reason
- Floating point cannot exactly represent decimal amounts; accumulated
  rounding errors in ledgers are a class of bug regulators and accountants
  reject.
- BIGINT is exact, simple, and fast; minor-unit integers are the standard
  in payment systems (Stripe et al.).
- KZT itself has no fractional circulation, but tiyn exists at 1/100 —
  two decimal places covers it; future currencies (USD/EUR with cents) work
  identically.

## Rejected alternatives
- **`numeric(19,4)` in Postgres + decimal types** — correct but slower and
  leaks decimal precision decisions into every query; integers are simpler
  end-to-end and match provider APIs (which use minor units).
- **float64 + rounding on display** — banned; silent drift.
- **Currency-float pairs in JSON/API** — the API exposes minor units +
  currency only; presentation formatting happens client-side.