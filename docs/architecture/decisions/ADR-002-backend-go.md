# ADR-002: Backend — Go

**Status:** Accepted · **Date:** 2026-08-17

## Decision
Go 1.24 with `chi` (router), `pgx` (Postgres driver/pool), `sqlc` (typed
queries), `zerolog` (structured logs), `golang-jwt`, `github.com/riverqueue/river`
(Postgres-backed jobs).

## Reason
- Single static binary, low memory, trivially containerized and operated by a
  small team.
- Strong typing + `sqlc` compile-time SQL checking reduce the class of bugs
  most dangerous in fintech.
- Excellent concurrency for webhook handling; boring, proven ecosystem.
- Excellent cross-platform story for future service extraction.

## Rejected alternatives
- **Node.js/NestJS** — faster initial scaffolding, but weaker type safety for
  money math, GC pauses under burst webhooks, heavier dependency tree.
- **Python/FastAPI** — great DX, but async DB stack less mature for
  high-integrity transactional code; typing weaker for money.
- **Java/Spring** — proven but heavier (JVM, memory, build) than a 4-person
  startup needs.
- **Rust** — excellent correctness, but developer velocity for an MVP team is
  lower.