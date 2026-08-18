# ADR-003: Frontend — Next.js (App Router)

**Status:** Accepted · **Date:** 2026-08-17

## Decision
Next.js 15 (App Router, React Server Components), TypeScript, Tailwind CSS,
TanStack Query (client data), Zod (validation shared types), Vitest +
Playwright (tests).

## Reason
- Fastest path to a production-grade SSR app for two portals (buyer,
  supplier) with one codebase.
- RSC + server-side BFF pattern keeps API tokens off the browser and gives
  clean CSP.
- Vercel-compatible but deployable anywhere (we deploy on ECS Fargate) —
  no lock-in.
- Huge ecosystem; hiring pool in KZ is large for React/TS.

## Rejected alternatives
- **React SPA (Vite + MUI)** — slower first paint, worse SEO, more manual
  auth/token plumbing.
- **Vue/Nuxt** — fine, but smaller fintech component ecosystem.
- **SvelteKit** — excellent, but smaller team familiarity and ecosystem.
- **Server-rendered templates (Go html/template)** — too limited for the
  dashboard-heavy product.