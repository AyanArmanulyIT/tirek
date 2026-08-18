# ADR-006: Background jobs — Transactional Outbox + River

**Status:** Accepted · **Date:** 2026-08-17

## Decision
Async work (notifications, PSP calls, finance submissions, webhook
processing) is driven by a **transactional outbox** table in Postgres,
consumed by `tirek-worker` using River (a Postgres-backed, Go-native job
queue). No external broker in the MVP.

## Reason
- The outbox gives exactly-once-ish, auditable, retryable async semantics
  with **zero extra infrastructure** — the same Postgres already enforces
  money invariants; the outbox row commits with the business change.
- River is boring (pure SQL under the hood), Go-native, supports unique
  jobs + retries + dead-letter, and scales far beyond MVP load.
- When (if) throughput demands a broker, the outbox table is the contract —
  swap the consumer only.

## Rejected alternatives
- **RabbitMQ / Kafka / SQS** — real brokers are operationally heavier and
  solve a scale problem the MVP does not have; the outbox table is the same
  pattern Kafka users rely on anyway.
- **Redis lists/streams as the queue** — at-least-once with durable retries
  is harder than Postgres' transactional guarantees; Redis stays a cache.
- **Cron-only** — no retry/backoff/dead-letter semantics.