# Tirek

B2B platform for restaurants and food-service businesses in Kazakhstan:
**procurement · suppliers · orders · payments · financing**.

## 1. Project Overview

Tirek connects restaurants/food-service businesses with suppliers and
facilitates orders, payments and financing. Payments and financing are
provider-agnostic: Tirek is a facilitator, while regulated partners (PSPs,
banks) hold funds and take credit risk. All financial mutations are recorded
in an immutable double-entry ledger, and every tenant is isolated with
PostgreSQL Row-Level Security.

### Technology stack

| Layer | Technology |
|---|---|
| Backend | Go 1.24, chi router, pgx (PostgreSQL driver), sqlc-generated data access, JWT auth, zerolog |
| Database | PostgreSQL 16, Redis 7 |
| Frontend | Next.js 15 (App Router), React 19, TypeScript, Tailwind CSS |
| Infra | Docker + Docker Compose (local), AWS ECS/Fargate, RDS, ElastiCache, S3, Secrets Manager |
| CI/CD | GitHub Actions (CI + deploy) |

### Repository structure

```text
backend/                 Go modular-monolith API + worker + migrate binary
  cmd/api/               HTTP API entrypoint
  cmd/worker/            Outbox worker (payments, financing, notifications)
  cmd/migrate/           SQL migration runner
  internal/              Modules (identity, organizations) + platform packages
  migrations/            Versioned SQL migrations (applied in order)
  sqlc/                  sqlc schema/queries used to generate data access
frontend/                Next.js web application (buyer + supplier portals)
infra/                   Docker init scripts, ECS task definitions, monitoring
docs/architecture/       Architecture documentation (start here)
.github/workflows/       CI/CD pipelines
docker-compose.yml       Local environment: postgres, redis, api, worker, web
Makefile                 Local development orchestration (see below)
```

## 2. Prerequisites

Install and verify the following before starting:

| Tool | Minimum version | Verify with |
|---|---|---|
| Go | 1.24.x | `go version` |
| Node.js | 20+ | `node --version` |
| npm | (ships with Node) | `npm --version` |
| Docker | any recent | `docker --version` |
| Docker Compose | v2+ | `docker compose version` |
| PostgreSQL | 16 (via Docker Compose) | `docker compose ps` |
| Redis | 7 (via Docker Compose) | `docker compose ps` |
| golangci-lint | 1.64+ | `golangci-lint version` (required by `make lint` / `make verify`) |
| sqlc | 1.x | `sqlc version` (only required to regenerate SQL code with `make generate`) |

```bash
go version
node --version
npm --version
docker --version
docker compose version
golangci-lint version
```

Install the Go-based tools (they land in `$GOPATH/bin` — make sure that
directory is on your `PATH`):

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest   # only if you regenerate SQL code
```

## 3. Installation

```bash
git clone <repository-url>
cd tirek
```

**Backend dependencies** (the Go module is self-contained):

```bash
cd backend
go mod download
cd ..
```

**Frontend dependencies:**

```bash
cd frontend
npm install
cd ..
```

**Prepare the environment** — copy the template to your local `.env` (see
[Environment Configuration](#4-environment-configuration)):

```bash
cp .env.example .env
```

If you already have a valid local `.env`, keep it — it must not be overwritten.

## 4. Environment Configuration

Tirek reads configuration from environment variables. Locally these are
provided by the `.env` file (loaded via `docker compose` `env_file:` or by the
`make run-api` / `make run-worker` targets).

- **`.env`** — your real local credentials and secrets. **Never commit this
  file to Git** (it is already excluded by `.gitignore`).
- **`.env.example`** — template with variable names and placeholders only. No
  real secrets. This is the file to commit.

Create your local environment file:

```bash
cp .env.example .env
```

> **Important:** The repository already ignores `.env` and all `.env.*`
> variants (only `.env.example` is tracked). Double-check with
> `git status` that `.env` never appears as a new/untracked file.

### Required environment variables

| Variable | Description | Local default |
|---|---|---|
| `APP_ENV` | Runtime environment: `local` / `staging` / `production` | `local` |
| `LOG_LEVEL` | Log verbosity: `debug` / `info` / `warn` / `error` | `info` |
| `API_PORT` | HTTP port for the API | `8080` |
| `API_EXTERNAL_URL` | Public base URL of the API | `http://localhost:8080` |
| `WEB_ORIGIN` | Allowed web origin (CORS) | `http://localhost:3000` |
| `POSTGRES_PORT` | Host port mapped to the Docker PostgreSQL container (see [PostgreSQL](#5-database)) | `5433` |
| `DATABASE_URL` | PostgreSQL connection string (low-privilege `tirek_app` role) | `postgres://tirek_app:tirek_app@localhost:5433/tirek` |
| `DATABASE_POOL_MAX` | Max DB connections in the pool | `20` |
| `REDIS_URL` | Redis connection string | `redis://localhost:6379/0` |
| `JWT_ACCESS_SECRET` | HMAC secret for access tokens (≥ 32 bytes) | generated |
| `JWT_REFRESH_SECRET` | HMAC secret for refresh tokens (≥ 32 bytes) | generated |
| `JWT_ACCESS_TTL` | Access token lifetime | `15m` |
| `JWT_REFRESH_TTL` | Refresh token lifetime | `720h` (30 days, rotated on use) |
| `AUTH_COOKIE_NAME` | Refresh-token cookie name | `tirek_refresh` |
| `AUTH_COOKIE_DOMAIN` | Cookie domain | `localhost` |
| `AUTH_COOKIE_SECURE` | `true` to set the cookie `Secure` flag | `false` |
| `S3_REGION` | Object-storage region (assets) | `eu-central-1` |
| `S3_BUCKET` | Object-storage bucket | `tirek-assets` |
| `S3_PUBLIC_BASE_URL` | Public URL of the storage bucket | `http://localhost:9000/tirek-assets` |
| `PAYMENT_PROVIDER` | Payment adapter: `mock` / `kaspi` / `stripe` | `mock` |
| `PAYMENT_MODE` | Payment mode: `sandbox` / `live` | `sandbox` |
| `KASPI_MERCHANT_ID` | Kaspi merchant id (empty in sandbox/mock) | *(empty)* |
| `KASPI_API_KEY` | Kaspi API key (empty in sandbox/mock) | *(empty)* |
| `KASPI_SIGNATURE_KEY` | Kaspi webhook HMAC signature key | *(empty)* |
| `KASPI_BASE_URL` | Kaspi API base URL | `https://sandbox.kaspi.kz` |
| `FINANCING_PROVIDER` | Financing adapter: `mock` / partner | `mock` |
| `FINANCING_PARTNER_API_KEY` | Partner financing API key | *(empty)* |
| `FINANCING_PARTNER_WEBHOOK_SECRET` | Partner webhook secret | *(empty)* |
| `OUTBOX_POLL_INTERVAL` | Outbox worker poll interval | `2s` |
| `OUTBOX_BATCH_SIZE` | Outbox worker batch size | `100` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OpenTelemetry OTLP endpoint | `http://localhost:4317` |
| `SENTRY_DSN` | Sentry DSN (empty = disabled) | *(empty)* |
| `WEBHOOK_SIGNING_SECRET` | HMAC secret for signing outgoing webhooks (≥ 32 bytes) | generated |

Generate JWT/webhook secrets with:

```bash
openssl rand -base64 48   # must be >= 32 bytes
```

The API refuses to start if `JWT_ACCESS_SECRET` or `JWT_REFRESH_SECRET` is
shorter than 32 bytes (see `config.Validate()`).

## 5. Database

PostgreSQL and Redis are started from `docker-compose.yml`. The init scripts in
`infra/postgres-init/` provision the development roles (`tirek_app`,
`tirek_migrator`) on first boot.

### PostgreSQL ports

The Docker PostgreSQL container is exposed on a **configurable host port** so
it never collides with a host PostgreSQL already listening on the default
`5432`:

```text
Host machine                 Docker network
localhost:5433   ──────────▶  postgres:5432
```

- **Host (local backend, `make run-api`, tests):** `localhost:${POSTGRES_PORT}`
  (default `5433`) — set in `.env` / `.env.example`.
- **Inside the Docker network (api/worker/migrate containers):**
  `postgres:5432` — the containers connect to the Compose service name, never
  to `localhost`.

If `5433` is also occupied, change `POSTGRES_PORT` in `.env` (e.g.
`POSTGRES_PORT=5434`), then recreate the container:

```bash
POSTGRES_PORT=5434 docker compose up -d --force-recreate postgres
```

Update `DATABASE_URL` in `.env` to match the new port if you run the backend on
the host.

```bash
# Start PostgreSQL and Redis
make infra

# Verify they are healthy
docker compose ps
pg_isready -h localhost -p "${POSTGRES_PORT:-5433}"   # or: docker compose exec postgres pg_isready -U tirek
```

**Apply migrations** (runs as the owner role, guarded by an advisory lock):

```bash
make migrate
```

Migrations apply the schema and seed baseline reference data (permissions,
roles, chart of accounts) — no separate seed script is needed. Applied versions
are recorded in `schema_migrations`.

If you run a PostgreSQL on your host instead of the container, use:

```bash
make migrate-local
```

`make migrate-local` connects to `localhost:${POSTGRES_PORT}` (default `5433`)
as the owner role — either the Docker container or a native install.

## 6. Running the Project

### Full local environment (infrastructure + all services)

```bash
make up        # builds and starts postgres, redis, api, worker, web
```

Wait for the API to be ready, then verify:

```bash
curl http://localhost:8080/v1/health   # -> ok
```

- API: http://localhost:8080 (health: `GET /v1/health`)
- Web: http://localhost:3000

### Running services individually on the host (fast iteration)

These targets load `.env` and connect to PostgreSQL/Redis on `localhost`
(PostgreSQL on `localhost:${POSTGRES_PORT}`, default `5433`).

```bash
# Start infrastructure containers
make infra

# Run migrations
make migrate

# Start the API
make run-api

# Start the outbox worker
make run-worker

# Start the frontend dev server
make frontend-dev
```

Or run the frontend directly:

```bash
cd frontend
npm run dev
```

Stop everything with `make down`, tail logs with `make logs`.

## 7. Testing and Verification

| Command | What it does | Requires |
|---|---|---|
| `make test` | Quick check: backend unit tests + frontend ESLint | nothing extra |
| `make test-unit` | Backend unit tests only | nothing (no database) |
| `make test-integration` | RLS end-to-end tests against a real PostgreSQL on `localhost:5433` (creates `tirek_test` via `make test-db`) | PostgreSQL + Redis running |
| `make test-all` | Full test suite: unit + integration + frontend lint + typecheck | PostgreSQL + Redis running |
| `make verify` | Everything CI checks: `test-all`, `go vet`, backend build, frontend typecheck/lint/build, golangci-lint | PostgreSQL + Redis, golangci-lint installed |
| `make lint` | golangci-lint (backend) + ESLint (frontend) | golangci-lint installed |
| `make build` | `go build ./...` (backend) + `next build` (frontend) | — |
| `make generate` | Regenerate SQL data access with `sqlc` (requires `sqlc`) | sqlc installed |

`make verify` is the full relevant verification suite and mirrors what CI
runs. Run it before committing.

## 8. Common Development Workflow

1. Start infrastructure — `make infra`
2. Configure `.env` — `cp .env.example .env` and fill in values (or keep your existing file)
3. Run migrations — `make migrate`
4. Start the API — `make run-api`
5. Start the worker — `make run-worker`
6. Start the frontend — `make frontend-dev`
7. Run tests before committing — `make test` (quick) or `make verify` (full)

## 9. Troubleshooting

| Problem | Fix |
|---|---|
| `go: not found` | Install Go 1.24.x and ensure it is on `PATH` (`go version`). |
| Wrong Go version (build errors about `go.mod` / toolchain) | Install Go ≥ 1.24; `go version` must report 1.24 or newer. |
| `node: not found` or build errors in `frontend/` | Install Node.js 20+ (`node --version`). |
| `npm install` / dependency errors | Delete `frontend/node_modules` and `frontend/package-lock.json` then run `npm install` again. |
| `Cannot connect to the Docker daemon` | Start the Docker daemon (systemd: `systemctl start docker`, or your platform's service) then retry `docker ps`. |
| Port already in use (`0.0.0.0:5432` bind error) | The Docker PostgreSQL maps to a configurable host port so it never needs `5432`. Confirm `.env` sets `POSTGRES_PORT=5433` (or another free port) and that `docker-compose.yml` uses `"${POSTGRES_PORT:-5433}:5432"`. If `5433` is also taken, pick another port: `POSTGRES_PORT=5434 docker compose up -d --force-recreate postgres` and update `DATABASE_URL` in `.env`. |
| Docker networking issues (services cannot reach `postgres`/`redis`) | Containers must connect via the Compose service names, not `localhost`. The compose file already sets `postgres://…@postgres:5432/…` and `redis://redis:6379/0`. `localhost` is only for host-side processes (see [PostgreSQL ports](#5-database)). |
| `DATABASE_URL` connection failure (`password authentication failed` / `connection refused`) | Confirm PostgreSQL is running and reachable at the URL in `.env` — host backend uses `localhost:${POSTGRES_PORT}` (default `5433`), containers use `postgres:5432`. The `tirek_app` role is created by `infra/postgres-init/01_roles.sql` on a fresh container volume; for an existing volume run `docker compose down -v` (drops data) or provision the role manually. |
| Migration failure / permission denied when migrating | Migrations must run as the owner role (`tirek`), not `tirek_app`. Use `make migrate` (container) or `make migrate-local` (host); `tirek_app` has no DDL privileges by design. |
| `invalid configuration: JWT_ACCESS_SECRET must be at least 32 bytes` | `.env` is missing or has placeholder secrets. `cp .env.example .env` and set real values (`openssl rand -base64 48`). |
| RLS permission errors in the API / tests | The API runs as `tirek_app` (RLS-enforced, low privilege). Do not point it at a superuser role; re-provision roles with a fresh `infra/postgres-init` volume. |
| API starts but `/v1/health` fails to connect | Redis or PostgreSQL is down — start them (`make infra`) and check `make logs`. |
| Missing environment variables (empty `DATABASE_URL`, `JWT_*`, …) | `.env` is not loaded or does not exist. `cp .env.example .env`, fill in secrets, then re-run the `make` target (the `make run-*` targets and `docker compose` both read `.env`). |

## Documentation

| Document | Content |
|---|---|
| [Architecture](docs/architecture/architecture.md) | System overview, modules, stack, diagrams |
| [Domain model](docs/architecture/domain-model.md) | Entities, relationships, state machines |
| [Database](docs/architecture/database.md) | Schema, constraints, indexes, RLS |
| [API](docs/architecture/api.md) | REST conventions, errors, idempotency, webhooks |
| [Financial architecture](docs/architecture/financial-architecture.md) | Money, ledger, payments, financing |
| [Security](docs/architecture/security.md) | AuthN/AuthZ, tenancy, OWASP |
| [Infrastructure](docs/architecture/infrastructure.md) | AWS topology, environments, deployments |
| [Decisions](docs/architecture/decisions/) | Architecture Decision Records (ADRs) |

## Principles

1. **Boring, proven technology.** Fast MVP, no experimental infrastructure.
2. **Money is never a float.** All monetary values are integer minor units.
3. **Modular monolith.** Clean module boundaries; extract services only when there is a real reason.
4. **Provider-agnostic payments and financing.** Tirek is a facilitator; regulated partners hold funds and take credit risk.
5. **Audit-first.** Every financial mutation is recorded in an immutable double-entry ledger plus audit log.
6. **Tenant isolation by default.** Row-Level Security enforced at the database level.

## Environments

- `local` — Docker Compose on your machine
- `staging` — AWS, mirrors production, connected to PSP sandbox
- `production` — AWS eu-central-1 (nearest AWS region to Kazakhstan), PSP live credentials

See [infrastructure.md](docs/architecture/infrastructure.md) for details.