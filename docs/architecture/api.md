# Tirek — API Architecture

Version: 1.0 (MVP)
Status: Approved

## 1. Style and versioning

- **REST over HTTPS**, JSON bodies. Chosen over GraphQL: boring, cacheable,
  easy to secure and audit, trivially consumable by POS integrations (see
  ADR-011).
- Version in the URL path: `/v1/...`. Version increments are additive;
  breaking changes bump the path version and both live side-by-side for one
  deprecation cycle.
- Server uses **UTC**; clients pass explicit offsets for date inputs.

## 2. Authentication

- `Authorization: Bearer <access_token>` (JWT, 15 min TTL).
- Refresh via `POST /v1/auth/refresh` with rotating refresh token (HTTP-only,
  SameSite=Lax, Secure cookie; rotation on every use, reuse detection
  invalidates the session).
- Login: `POST /v1/auth/login` (email + password, argon2id verify).
- Webhook endpoints are authenticated by **HMAC signature header**, not Bearer
  tokens (see §8).
- POS/future machine integrations: API keys (`X-API-Key`), scoped to
  organization + permission set, stored hashed.

## 3. Tenancy context

- The access token carries `sub` (user id), `org_id` (active organization),
  `role`, `session_id`.
- The app reads tenancy from the token only — never from request bodies or
  query params (avoids tenant-swap attacks). The API rejects requests where
  token `org_id` differs from path `org_id` when both are present.

## 4. Pagination

- **Cursor pagination** for lists that grow (orders, payments, invoices,
  audit): `?cursor=<opaque>&limit=50`.
  - Response: `{ "data": [...], "next_cursor": "..." }`.
  - Cursor = base64 of `(created_at, id)` of last item; stable under inserts.
- **Offset pagination** only for admin/small reference lists (users in org,
  roles) where cursor adds no value.
- Default `limit=50`, max `limit=200`.

## 5. Errors

RFC 7807 `application/problem+json`:

```json
{
  "type": "https://api.tirek.kz/errors/insufficient-funds",
  "title": "Insufficient funds",
  "status": 422,
  "detail": "Payment intent cannot be captured: invoice has an outstanding balance of 0.",
  "instance": "/v1/payments/intents/in_abc",
  "code": "INSUFFICIENT_FUNDS",
  "request_id": "req_01J..."
}
```

| HTTP | Meaning |
|---|---|
| 400 | Malformed request / validation |
| 401 | Missing/invalid credentials |
| 403 | Authenticated but not permitted (RBAC) |
| 404 | Resource not found (or not in tenant — same response) |
| 409 | Conflict: state transition invalid, duplicate idempotency key |
| 422 | Business rule violation |
| 429 | Rate limited (Retry-After header) |
| 5xx | Server error; `detail` generic; `request_id` for correlation |

Always include `request_id`; the backend logs it with every log line.

## 6. Idempotency

- Any state-changing endpoint that triggers money movement or external calls
  accepts `Idempotency-Key: <uuid>` header.
- Server: `idempotency_keys(org_id, key, endpoint)` with unique index; stores
  the request hash + response. Replays return the stored response (200),
  mismatching request bodies return 409.
- Key TTL: 24 h, then garbage-collected.
- Keys are also used internally by the worker to guarantee at-least-once
  processing of outbox events (dedupe key = `topic:entity_id`).

## 7. Endpoints (v1, MVP subset)

| Method & path | Purpose |
|---|---|
| `POST /v1/auth/login` · `POST /v1/auth/refresh` · `POST /v1/auth/logout` | Session |
| `GET /v1/me` | Current user + memberships |
| `GET/POST /v1/orgs` · `GET /v1/orgs/{id}/members` | Organizations, membership mgmt |
| `GET/POST/PATCH /v1/orgs/{id}/roles` | RBAC administration |
| `GET/POST /v1/orgs/{id}/restaurants` · `.../{restaurantId}/outlets` | Restaurants/outlets |
| `GET/POST /v1/orgs/{id}/suppliers` · `.../{id}/contacts` | Supplier directory |
| `GET/POST /v1/orgs/{id}/catalog/categories` · `.../products` | Catalog |
| `POST /v1/orgs/{id}/rfqs` · `POST /v1/orgs/{id}/purchase-orders` | Procurement |
| `GET /v1/orgs/{id}/orders` | Buyer order list |
| `POST /v1/orgs/{id}/invoices` · `POST /v1/orgs/{id}/invoices/{id}/approve` | Invoicing |
| `POST /v1/orgs/{id}/payment-intents` · `GET /v1/orgs/{id}/payment-intents/{id}` | Payments (idempotent) |
| `POST /v1/orgs/{id}/payments/{id}/refunds` | Refunds |
| `POST /v1/orgs/{id}/financing/applications` · `GET /v1/orgs/{id}/financing/agreements` | Financing |
| `GET /v1/orgs/{id}/ledger/accounts/{id}/entries` | Account statement |
| `GET /v1/orgs/{id}/audit-logs` | Audit trail |
| `GET /v1/health` · `GET /v1/ready` | Liveness/readiness (ALB target checks) |
| `POST /v1/webhooks/{provider}` | PSP/partner webhooks (HMAC) |

## 8. Webhooks

Inbound (from PSPs, financing partners, and future integrations):

- Signed with **HMAC-SHA256** over the raw body using the provider's shared
  secret (stored per provider in Secrets Manager, rotated).
- Provider webhook payloads are stored verbatim in `webhook_events(provider,
  event_id)` — unique index on `(provider, event_id)` **drops duplicate
  deliveries** (returns 200 for replays without reprocessing).
- Processing: parse → validate signature → record → enqueue outbox event →
  idempotent handler (payment capture, financing funding, etc.).
- Respond `200 OK` fast; heavy work goes to the worker. Timeout-sensitive
  providers get an immediate `200` after recording, processing async.

Outbound (Tirek → buyer/supplier apps, future customer webhooks):

- Signed with `WEBHOOK_SIGNING_SECRET`; header `X-Tirek-Signature` =
  `HMAC-SHA256(secret, timestamp + "." + body)`; replay window ±5 min.
- Delivery via outbox with retries and dead-letter; events ordered per
  entity.

## 9. Rate limiting

- Redis-backed token bucket per `(org_id, user_id)`; stricter bucket per IP
  for unauthenticated endpoints (login, signup).
- Limits: default 100 req/min/org-user, 10 req/min for login attempts
  (plus per-IP), webhook endpoints 500 req/min.
- `429` + `Retry-After`.

## 10. Validation and serialization

- Request validation with typed Go structs (json tags) + a small validator;
  every input is bounded (length, charset, enum checks).
- SQL access exclusively via sqlc — no string-built queries (SQL injection is
  structurally impossible).
- File uploads limited (10 MB images), validated content type, stored in S3,
  served through signed URLs.