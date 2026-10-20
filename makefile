SHELL := /bin/bash
-include .env
export
-include scripts/make/*.mk

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := grimoire/cmd/internal/buildinfo
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).BuildDate=$(DATE)
COMPOSE := docker compose -f docker-compose.dev.yml
DIST    := backend/internal/infra/static/dist
REDOCLY := @redocly/cli@1.34.5
TEST_DATABASE_URL ?= postgres://grimoire:grimoire@localhost:5432/grimoire?sslmode=disable

.PHONY: help db-up db-down dev dev-db dev-backend dev-frontend build test vet test-integration test-e2e lint migrate

help: ## daftar target
	@grep -E "^[a-zA-Z_-]+:.*##" $(MAKEFILE_LIST) | sed "s/:.*##/ -/"

db-up: dev-db ## jalankan Postgres dev

db-down: ## hentikan Postgres dev
	$(COMPOSE) down

dev: dev-db ## Postgres + backend (:8080) + Vite (:5173)
	$(MAKE) -j2 dev-backend dev-frontend

dev-db:
	$(COMPOSE) up -d --wait db

dev-backend:
	cd backend && APP_ENV=development RUN_MIGRATIONS_ON_START=true COOKIE_SECURE=false \
	  DEV_VITE_URL=http://localhost:5173 go run ./cmd/http

dev-frontend:
	cd frontend && npm run dev

build: ## bin/grimoire (FE ter-embed)
	cd frontend && npm ci && npm run build
	rm -rf $(DIST)/* && cp -R frontend/dist/. $(DIST)/
	mkdir -p bin
	cd backend && CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o ../bin/grimoire ./cmd/cli
	-git checkout -- $(DIST)/index.html 2>/dev/null

test: ## unit test backend dan frontend
	cd backend && go test -race ./...
	cd frontend && npm test -- --run

vet: ## go vet
	cd backend && go vet ./...

test-integration: dev-db ## test integrasi (butuh Postgres)
	cd backend && TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -race -tags integration ./...

test-e2e: build ## Playwright terhadap bin/grimoire
	@set -e; \
	: "$${DATABASE_URL:?DATABASE_URL wajib}" "$${SESSION_SECRET:?SESSION_SECRET wajib}"; \
	HTTP_ADDR=127.0.0.1:8080 ./bin/grimoire & pid=$$!; \
	trap 'kill $$pid 2>/dev/null' EXIT; \
	for i in $$(seq 1 50); do curl -sf localhost:8080/healthz >/dev/null && break; sleep 0.2; done; \
	cd frontend && E2E_BASE_URL=http://127.0.0.1:8080 npx playwright test

lint: ## vet + golangci-lint + svelte-check + depcruise
	cd backend && go vet ./... && golangci-lint run
	cd frontend && npm run check && npx depcruise src

migrate: ## jalankan migrasi
	cd backend && go run ./cmd/cli migrate up
