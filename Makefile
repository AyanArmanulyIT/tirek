# Tirek Makefile
# Local development orchestration. Every target is documented in README.md;
# run `make help` for a quick summary.

.PHONY: help up down logs ps infra \
        migrate migrate-local \
        run-api run-worker \
        frontend-install frontend-dev \
        test test-unit test-integration test-all verify \
        lint build generate test-db

COMPOSE := docker compose

## --- Full stack ------------------------------------------------------------

up:          ## Build and start everything (postgres, redis, api, worker, web)
	$(COMPOSE) up -d --build

down:        ## Stop and remove all containers
	$(COMPOSE) down

logs:        ## Tail logs for all services
	$(COMPOSE) logs -f

ps:          ## Show running containers
	$(COMPOSE) ps

infra:       ## Start only the infrastructure containers (postgres, redis)
	$(COMPOSE) up -d postgres redis

## --- Database ----------------------------------------------------------------

migrate:     ## Apply SQL migrations (runs as the owner role inside the migrate container)
	$(COMPOSE) run --rm migrate

migrate-local: ## Apply SQL migrations against the local PostgreSQL (owner role)
	cd backend && bash -c 'set -a; source ../.env; set +a; export DATABASE_URL="postgres://tirek:tirek@localhost:$${POSTGRES_PORT:-5433}/tirek?sslmode=disable"; exec go run ./cmd/migrate ./migrations'

test-db:     ## Create the tirek_test database if it does not exist yet
	@docker compose exec -T postgres createdb -U tirek tirek_test 2>/dev/null \
		|| bash -c 'set -a; source .env; set +a; PGPASSWORD=tirek createdb -U tirek -h localhost -p "${POSTGRES_PORT:-5433}" tirek_test' 2>/dev/null \
		|| true

## --- Run services on the host -----------------------------------------------
# These load .env (real secrets stay local) and connect to PostgreSQL and
# Redis on localhost — either the compose containers or a host install.

run-api:     ## Start the API on the host (http://localhost:8080)
	cd backend && bash -c 'set -a; source ../.env; set +a; exec go run ./cmd/api'

run-worker:  ## Start the outbox worker on the host
	cd backend && bash -c 'set -a; source ../.env; set +a; exec go run ./cmd/worker'

## --- Frontend ----------------------------------------------------------------

frontend-install: ## Install frontend dependencies (npm install)
	cd frontend && npm install

frontend-dev: ## Start the Next.js dev server (http://localhost:3000)
	cd frontend && npm run dev

## --- Tests -------------------------------------------------------------------

test:        ## Quick check: backend unit tests + frontend lint
	$(MAKE) test-unit
	cd frontend && npm run lint

test-unit:   ## Backend unit tests only (no database required)
	cd backend && go test $$(go list ./... | grep -v /internal/integration)

test-integration: ## RLS end-to-end tests against a real PostgreSQL on localhost:5433
	$(MAKE) test-db
	cd backend && go test -race -count=1 ./internal/integration/

test-all:    ## Full test suite: unit + integration + frontend lint/typecheck
	$(MAKE) test-unit
	$(MAKE) test-integration
	cd frontend && npm run lint && npm run typecheck

verify:      ## Everything CI checks: tests, vet, build, lint
	$(MAKE) test-all
	cd backend && go vet ./... && go build ./...
	cd frontend && npm run typecheck && npm run lint && npm run build
	cd backend && golangci-lint run ./...

## --- Lint / build / generate ------------------------------------------------

lint:        ## Run backend (golangci-lint) and frontend (eslint) linters
	cd backend && golangci-lint run ./...
	cd frontend && npm run lint

build:       ## Compile backend binaries and build the frontend
	cd backend && go build ./...
	cd frontend && npm run build

generate:    ## Regenerate SQL code with sqlc (requires sqlc, see README)
	cd backend && sqlc generate

help:        ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'