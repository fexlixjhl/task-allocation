SHELL := /bin/bash
.DEFAULT_GOAL := help

GO        ?= go
SPECTRAL  ?= npx --yes @stoplight/spectral-cli
AGENTSYNC := $(GO) -C tools/agentsync run . -root ../..

DB_URL    ?= postgres://taskalloc:taskalloc-dev-only@localhost:5432/taskalloc?sslmode=disable
GOOSE     := $(GO) -C tools/gen tool goose -dir ../../backend/migrations postgres "$(DB_URL)"

.PHONY: help generate verify lint test \
        agents agents-check toolchain-check generate-check \
        generate-api generate-sql generate-client \
        spec-lint spec-ruleset-selftest gen-selftest \
        backend-lint frontend-lint backend-test frontend-test \
        migrate-up migrate-down migrate-status migrate-create

## --- Interfaz pública: estos tres targets los invocan los agentes ---

help: ## Lista los targets disponibles
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) \
	  | sort \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

generate: agents generate-api generate-sql generate-client ## Regenera todo lo derivado

verify: lint test ## Todo lo que debe pasar antes de marcar un PR ready

lint: agents-check toolchain-check generate-check spec-lint backend-lint frontend-lint ## Solo los linters

test: gen-selftest backend-test frontend-test ## Solo los tests

## --- Arnés de agentes y coherencia del entorno ---

agents: ## Regenera los adaptadores de agente de ambos runtimes
	@$(AGENTSYNC)

agents-check: ## Verifica el arnés sin escribir; falla si está desactualizado
	@$(AGENTSYNC) -check

toolchain-check: ## Verifica que la versión de Go coincide en las tres declaraciones
	@./tools/toolchain-check.sh

## --- Generación derivada del contrato ---

generate-api: ## Servidor y tipos Go desde el contrato
	@if [ -f api/openapi.yaml ]; then \
	  $(GO) -C tools/gen tool oapi-codegen -config ../../api/oapi-codegen.yaml ../../api/openapi.yaml; \
	  echo "make: servidor Go generado"; \
	else \
	  echo "make: sin api/openapi.yaml todavía, generate-api omitido"; \
	fi

generate-sql: ## Acceso a datos tipado desde las consultas SQL
	@if [ -n "$$(ls -A backend/queries 2>/dev/null | grep -v '^\.gitkeep$$')" ]; then \
	  $(GO) -C tools/gen tool sqlc -f ../../backend/sqlc.yaml generate; \
	  echo "make: acceso a datos generado"; \
	else \
	  echo "make: sin consultas en backend/queries, generate-sql omitido"; \
	fi

generate-client: ## Cliente TypeScript desde el contrato
	@if [ -f api/openapi.yaml ] && [ -f frontend/package.json ]; then \
	  cd frontend && npm run --silent generate:api; \
	  echo "make: cliente TypeScript generado"; \
	else \
	  echo "make: sin spec o sin frontend, generate-client omitido"; \
	fi

generate-check: ## Falla si el código generado no está al día con sus fuentes
	@$(MAKE) --no-print-directory generate >/dev/null
	@if ! git diff --quiet -- \
	     backend/internal/adapters/http/gen \
	     backend/internal/adapters/postgres/gen \
	     frontend/src/api \
	     .claude .github/agents; then \
	  echo "ERROR: hay código generado desactualizado:"; \
	  git diff --name-only -- backend/internal/adapters/http/gen \
	     backend/internal/adapters/postgres/gen frontend/src/api .claude .github/agents; \
	  echo "Ejecuta 'make generate' y commitea el resultado."; \
	  exit 1; \
	fi
	@echo "make: código generado al día"

## --- Linters ---

spec-lint: ## Lint del contrato con el ruleset propio
	@$(MAKE) --no-print-directory spec-ruleset-selftest
	@if [ -f api/openapi.yaml ]; then \
	  $(SPECTRAL) lint api/openapi.yaml --ruleset api/.spectral.yaml --fail-severity=error; \
	else \
	  echo "make: sin api/openapi.yaml todavía, spec-lint omitido"; \
	fi

spec-ruleset-selftest: ## Verifica que el ruleset de Spectral hace lo que dice
	@./api/testdata/selftest.sh

backend-lint:
	@if [ -n "$$(find backend -name '*.go' -print -quit 2>/dev/null)" ]; then \
	  $(GO) -C backend vet ./...; \
	else \
	  echo "make: sin código Go todavía, backend-lint omitido"; \
	fi

frontend-lint:
	@if [ -f frontend/package.json ]; then \
	  echo "make: eslint pendiente de configurar (PR-2c)"; \
	else \
	  echo "make: sin frontend/package.json todavía, frontend-lint omitido"; \
	fi

## --- Tests ---

gen-selftest: ## Verifica la cadena de generación contra el fixture
	@./api/testdata/gen-selftest.sh

backend-test:
	@if [ -n "$$(find backend -name '*_test.go' -print -quit 2>/dev/null)" ]; then \
	  $(GO) -C backend test ./...; \
	else \
	  echo "make: sin tests de Go todavía, backend-test omitido"; \
	fi
	@$(GO) -C tools/agentsync test ./...

frontend-test:
	@if [ -f frontend/package.json ]; then \
	  echo "make: vitest pendiente de configurar (PR-2c)"; \
	else \
	  echo "make: sin tests de frontend todavía, frontend-test omitido"; \
	fi

## --- Migraciones: efectos laterales sobre una base de datos. ---
## --- Fuera de generate y de verify a propósito.                ---

migrate-up: ## Aplica las migraciones pendientes
	@$(GOOSE) up

migrate-down: ## Revierte la última migración
	@$(GOOSE) down

migrate-status: ## Estado de las migraciones
	@$(GOOSE) status

migrate-create: ## Crea una migración vacía: make migrate-create NAME=crear_slots
	@test -n "$(NAME)" || { echo "uso: make migrate-create NAME=descripcion"; exit 1; }
	@$(GOOSE) create $(NAME) sql