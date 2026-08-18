# ADR-005: Cache — Redis

**Status:** Accepted · **Date:** 2026-08-17

## Decision
Redis 7 (ElastiCache, single node + replica) for: rate limiting (token
bucket), refresh-token index, catalog hot cache, outbox job lease counters.

## Reason
- Boring, ubiquitous, sub-millisecond; the exact three uses (rate limit,
  short TTL cache, token index) match Redis' sweet spot.
- Redis is never the source of truth: all durable state lives in Postgres;
  a Redis outage degrades performance (cache miss) but never corrupts money.

## Rejected alternatives
- **Memcached** — no data structures needed for rate limiting, no built-in
  scripting; Redis wins.
- **Valkey/Dragonfly** — Redis fork drama-free alternatives, but for MVP the
  ecosystem and docs of Redis win; swappable later (client-compatible).
- **Self-hosted on EC2** — ElastiCache removes patching/HA work for ~$30/mo.