# ADR-010: Provider-Agnostic Payments & Financing Adapters

**Status:** Accepted · **Date:** 2026-08-17

## Decision
Payments and financing are accessed through **Go interfaces** in the
`payments`/`financing` modules; concrete providers are adapters behind those
interfaces, configured per environment:

- Payments: `mock` (dev/staging default) and `kaspi` (Kaspi Pay, first live
  adapter — QR + card capture + payouts for KZ).
- Financing: `mock` and a partner adapter (factoring/BNPL bank) — partner
  takes credit risk and holds funds; Tirek only facilitates.

All adapters speak the same domain model (intents, statuses, webhooks
mapped to `webhook_events`), so switching a provider = config change + one
adapter.

## Reason
- KZ's payment market is provider-concentrated (Kaspi) but evolving; card
  processing (Halyk, Freedom), wallets, and B2B credit partners all matter.
  Vendor lock-in is a business risk.
- Regulated operations (holding funds, lending) must live with licensed
  partners; the adapter boundary keeps that separation enforced in code.
- Mock adapters make CI deterministic and let the whole product be demoed
  without sandbox credentials.

## Rejected alternatives
- **Single PSP integration hard-coded** — fast but locks the product to one
  provider's API, fees, and webhook quirks.
- **Custom payment aggregator build-out** — Tirek is not a processor; no PCI
  scope, no fund holding (see financial-architecture.md §10).
- **Tirek as lender / BNPL balance-sheet play** — needs banking/credit
  licenses, capital, and collections infra; out of MVP scope entirely.