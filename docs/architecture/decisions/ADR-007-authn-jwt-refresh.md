# ADR-007: Authentication — JWT + Rotating Refresh Tokens

**Status:** Accepted · **Date:** 2026-08-17

## Decision
Short-lived JWT access tokens (15 min, HS256, claims: `sub`, `org_id`,
`role`, `session_id`) + opaque rotating refresh tokens (30 days, stored
hashed in Postgres, reuse detection revokes the session). Passwords hashed
with **argon2id**. Access token in `Authorization: Bearer` header; refresh
token in `HttpOnly; Secure; SameSite=Lax` cookie.

## Reason
- Stateless access tokens keep the API horizontally scalable (no session
  lookup per request).
- Short TTL + rotation + reuse detection bounds compromise windows; hashed
  refresh tokens make DB leaks non-credential leaks.
- argon2id is the current recommended password hash (memory-hard, GPU
  resistant) — Go stdlib-adjacent (`golang.org/x/crypto`).

## Rejected alternatives
- **Opaque session tokens only** — forces a DB/Redis hit on every request;
  fine at MVP but adds latency and complexity for no MVP benefit.
- **OAuth2/OIDC with external IdP (Keycloak/Auth0)** — heavy for MVP; the
  platform owns identity in-house until international expansion (then
  introduce OIDC as the boundary).
- **Long-lived JWTs** — compromise window too large.
- **bcrypt/scrypt** — argon2id is the better default; bcrypt's 72-byte limit
  is an annoyance.