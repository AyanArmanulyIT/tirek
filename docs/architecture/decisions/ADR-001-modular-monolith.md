# ADR-001: Modular Monolith (not microservices)

**Status:** Accepted · **Date:** 2026-08-17

## Decision
Build Tirek as a **modular monolith**: one Go codebase with 13 clearly
bounded modules (identity, organizations, restaurants, suppliers, catalog,
procurement, orders, invoicing, payments, financing, ledger, notifications,
audit) plus a shared `platform` kernel. One deployable API process + one
worker process.

## Reason
- Team of 2–4 engineers: one deployable unit keeps velocity high.
- Money/audit domains need **transactional consistency across modules**
  (invoice + payment + journal in one DB transaction) — impossible across
  services without sagas/outbox ceremony.
- Module boundaries + outbox events make extraction mechanical later.
- Boring: no distributed-systems failure modes in the MVP.

## Rejected alternatives
- **Microservices (order, payment, ledger services)** — operational
  complexity before scale; cross-service transactions for money are
  error-prone at MVP stage.
- **Kubernetes + service mesh** — no scale problem exists yet; ops burden
  does not fit a small team.

## Extraction criteria (future)
Scale/security/team-ownership trigger per `architecture.md` §6; in-process
calls become outbox events consumed by the new service.