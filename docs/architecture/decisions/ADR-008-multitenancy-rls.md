# ADR-008: Multi-tenancy — Shared Schema + Row-Level Security

**Status:** Accepted · **Date:** 2026-08-17

## Decision
**Shared schema, single database**, with `org_id` on every tenant-scoped
table and **PostgreSQL Row-Level Security** enforced on all of them, driven
by `current_setting('app.tenant_id')` set from the verified JWT per request.
RBAC permissions enforced in the API layer as the primary control; RLS is the
defense-in-depth second layer.

## Reason
- One database is the cheapest correct model for a small team; RLS makes
  "forgot the WHERE org_id" impossible to leak data — the failure mode
  becomes an error, not a breach.
- RLS policies are testable in CI (negative tenant-isolation tests).
- Future per-tenant isolation (dedicated instances for enterprise buyers)
  is a documented path, not a rewrite: tenant routing key already exists.

## Rejected alternatives
- **Schema-per-tenant / database-per-tenant** — N× migration cost,
  connection pool multiplication, backup/restore complexity — wrong for
  hundreds of small tenants at MVP scale.
- **Application-layer only (no RLS)** — single point of failure; a buggy
  query leaks cross-tenant data with no second gate.
- **Multi-cluster/partitioning** — deferred until measured need.