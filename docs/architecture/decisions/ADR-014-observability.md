# ADR-014: Observability — OpenTelemetry + Prometheus + Grafana/Loki

**Status:** Accepted · **Date:** 2026-08-17

## Decision
- **Traces**: OpenTelemetry SDK (Go + Next.js) → OTLP → OTel Collector →
  Grafana Tempo.
- **Metrics**: Prometheus (scrape `/metrics`) → Grafana dashboards.
- **Logs**: structured JSON (zerolog) → CloudWatch Logs → Loki, correlated by
  `request_id`/`trace_id`.
- **Errors**: Sentry (frontend + Go panics).
- Alerting on the first-set list in infrastructure.md §9.

## Reason
- OTel is the vendor-neutral standard — swapping backends later is cheap.
- Prometheus/Grafana/Loki are boring, self-hostable, and free-tier-able
  (Grafana Cloud free tier fits MVP budget).
- `request_id` correlation across logs/traces is what makes a 3-person team
  able to debug a payment-webhook incident fast.

## Rejected alternatives
- **Datadog** — excellent but expensive for MVP (~$15+/host/mo × many
  services).
- **CloudWatch-only** — metrics/traces in CloudWatch are workable but
  dashboards and alert templating are worse; Grafana wins on DX.
- **Jaeger alone** — no dashboard/alert story without pairing with
  Prometheus anyway.