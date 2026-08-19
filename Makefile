SHELL := /bin/bash
.DEFAULT_GOAL := help

GO        ?= go
SPECTRAL  ?= npx --yes @stoplight/spectral-cli
AGENTSYNC := $(GO) -C tools/agentsync run . -root ../..

.PHONY: help generate verify lint test \
        agents agents-check \
        generate-api generate-sql generate-client \
        spec-lint backend-lint frontend-lint \
        backend-test frontend-test \
        spec-ruleset-selftest

## --- Interfaz pública: estos tres targets los invocan los agentes ---

help: ## Lista los targets disponibles
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) \
	  | sort \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

generate: agents generate-api generate-sql generate-client ## Regenera todo lo derivado

verify: lint test ## Todo lo que debe pasar antes de marcar un PR ready

lint: agents-check spec-lint backend-lint frontend-lint ## Solo los linters

test: backend-test frontend-test ## Solo los tests

## --- Arnés de agentes ---

agents: ## Regenera los adaptadores de agente de ambos runtimes
	@$(AGENTSYNC)

agents-check: ## Verifica el arnés sin escribir; falla si está desactualizado
	@$(AGENTSYNC) -check

## --- Generación derivada del contrato (se configura en PR-2) ---

generate-api:
	@if [ -f api/openapi.yaml ]; then \
	  echo "make: oapi-codegen pendiente de configurar (PR-2)"; \
	else \
	  echo "make: sin api/openapi.yaml todavía, generate-api omitido"; \
	fi

generate-sql:
	@if [ -n "$$(ls -A backend/queries 2>/dev/null | grep -v '^\.gitkeep$$')" ]; then \
	  echo "make: sqlc pendiente de configurar (PR-2)"; \
	else \
	  echo "make: sin consultas en backend/queries, generate-sql omitido"; \
	fi

generate-client:
	@if [ -f api/openapi.yaml ] && [ -f frontend/package.json ]; then \
	  echo "make: openapi-typescript pendiente de configurar (PR-2)"; \
	else \
	  echo "make: sin spec o sin frontend, generate-client omitido"; \
	fi

## --- Linters (se configuran en PR-2) ---

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
	  echo "make: eslint pendiente de configurar (PR-2)"; \
	else \
	  echo "make: sin frontend/package.json todavía, frontend-lint omitido"; \
	fi

## --- Tests ---

backend-test:
	@if [ -n "$$(find backend -name '*_test.go' -print -quit 2>/dev/null)" ]; then \
	  $(GO) -C backend test ./...; \
	else \
	  echo "make: sin tests de Go todavía, backend-test omitido"; \
	fi
	@$(GO) -C tools/agentsync test ./...

frontend-test:
	@if [ -f frontend/package.json ]; then \
	  echo "make: vitest pendiente de configurar (PR-2)"; \
	else \
	  echo "make: sin tests de frontend todavía, frontend-test omitido"; \
	fi
