# ADR-011: API — REST/JSON over HTTPS

**Status:** Accepted · **Date:** 2026-08-17

## Decision
REST/JSON over HTTPS, versioned in the path (`/v1`), RFC 7807 problem+json
errors, cursor pagination for growing lists, `Idempotency-Key` header for
state-changing endpoints, HMAC-signed webhooks. Documentation via OpenAPI
(`openapi.yaml` generated from route registry in CI).

## Reason
- REST is boring, cacheable, debuggable, and consumable by the POS/ERP
  integrations that matter for B2B adoption.
- Version-in-path is the least-surprise evolution strategy for partners.
- Idempotency + webhook replay protection are non-negotiable for payments
  (see financial-architecture.md §6–7).

## Rejected alternatives
- **GraphQL** — great DX but: harder caching, complex authorization at field
  level (dangerous with multi-tenancy), more back-end machinery, and POS
  integrations would need a second API anyway.
- **gRPC** — internal-only value; not consumable by browsers/POS without a
  gateway; revisit only if a service-to-service boundary appears (ADR-001
  extraction).
- **OData** — overkill for MVP.