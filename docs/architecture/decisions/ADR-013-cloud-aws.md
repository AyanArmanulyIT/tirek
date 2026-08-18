# ADR-013: Cloud — AWS

**Status:** Accepted · **Date:** 2026-08-17

## Decision
**AWS** in **eu-central-1** (Frankfurt). Services: ECS Fargate (app), RDS
PostgreSQL, ElastiCache Redis, S3, Secrets Manager, KMS, ALB, CloudFront,
WAF, CloudWatch. Deployment via GitHub Actions with OIDC federation.

## Reason
- Nearest major AWS region to Kazakhstan (latency ~80–100 ms to Almaty;
  eu-west-1 measured higher for KZ). Regional/local clouds (beCloud etc.)
  exist but lack the managed services and ecosystem we need.
- Managed services minimize the ops surface for a small team; Fargate removes
  node management entirely.
- OIDC + ECR gives a secure, keyless CI/CD path.

## Rejected alternatives
- **GCP** — comparable, but AWS has the deepest KZ-local knowledge, docs,
  and partner ecosystem (Kaspi integrations reference AWS-style hosting).
- **Azure** — fine, but team familiarity and fintech ecosystem favor AWS.
- **Kubernetes (EKS)** — no need at MVP scale; Fargate covers autoscaling;
  revisit via ADR-001 extraction criteria (infrastructure.md §12).
- **Hetzner/OVH VPS + docker-compose** — cheapest, but no managed RDS/Redis/
  secrets; backups, failover, and compliance story weaker for a payments
  platform.