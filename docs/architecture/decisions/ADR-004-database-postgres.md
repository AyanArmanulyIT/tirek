# ADR-004: Database — PostgreSQL 16 + sqlc

**Status:** Accepted · **Date:** 2026-08-17

## Decision
PostgreSQL 16 (RDS) as the single source of truth. `sqlc` for all queries;
SQL migrations as plain files. `pgcrypto` for UUIDv7 and optional field
encryption. Redis is used only for cache/rate-limit/session-index (ADR-005).

## Reason
- Postgres gives the transactional integrity money domains require, RLS for
  multi-tenancy (ADR-008), advisory locks, and the outbox pattern — all in
  one boring, well-known system.
- `sqlc` makes SQL the checked source of truth: no ORM impedance, no
  hand-written SQL strings, compile-time verification.
- JSONB covers flexible provider payloads without a second store.

## Rejected alternatives
- **MySQL** — weaker RLS story, weaker JSON, advisory locking less ergonomic.
- **MongoDB** — document flexibility does not justify losing transactions +
  joins for financial records.
- **CockroachDB / YugabyteDB** — distributed Postgres is over-engineering for
  MVP scale and adds operational cost.
- **GORM / ent** — ORMs hide the SQL; for money queries, explicit SQL +
  review wins. (ent considered; sqlc chosen for transparency and zero runtime
  dependency.)