# ADR-015: Webhooks — HMAC-Signed, Deduplicated, Recorded-Verbatim

**Status:** Accepted · **Date:** 2026-08-17

## Decision
- **Inbound** (PSP/lender → Tirek): HMAC-SHA256 over the raw body using a
  per-provider secret; payload stored verbatim in `webhook_events`; unique
  index on `(provider, event_id)` makes replays 200-no-op; heavy processing
  happens in the worker; handlers verify provider-side state before applying
  (e.g. GET payment status before marking `paid`).
- **Outbound** (Tirek → consumer integrations): `X-Tirek-Signature` =
  HMAC-SHA256(`timestamp.body`) with ±5 min replay window; delivery via the
  outbox with retries and dead-letter.

## Reason
- Payments cannot tolerate double-processing (double capture) or forgery;
  HMAC + dedupe + state-verification covers both.
- Recording payloads verbatim gives an independent audit source when
  reconciling with provider dashboards.
- Immediate `200` to the provider (providers are timeout-sensitive) while
  async processing happens in the worker keeps webhook latency safe.

## Rejected alternatives
- **Trusting webhook bodies without verification** — forgeable; the leading
  cause of payment fraud.
- **Synchronous processing inside the webhook handler** — risk of provider
  timeouts, retries amplifying load, and blocking the API.
- **No dedupe (just "be idempotent")** — dedupe + idempotency both needed:
  dedupe protects provider replays and malformed duplicate deliveries at the
  boundary; idempotency protects our own worker retries.