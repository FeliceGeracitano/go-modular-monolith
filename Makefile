# Developer entry points. Run `make` to list them.

GOLANGCI_LINT_VERSION ?= v2.15.0
# Built with the local toolchain, so it always understands the go.mod version.
GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

DATABASE_URL      ?= postgres://app:app@localhost:5432/app?sslmode=disable
TEST_DATABASE_URL ?= $(DATABASE_URL)
export DATABASE_URL TEST_DATABASE_URL

.DEFAULT_GOAL := help

.PHONY: help
help: ## List the available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

.PHONY: up
up: ## Start PostgreSQL in the background
	docker compose up -d --wait postgres

.PHONY: down
down: ## Stop PostgreSQL and delete its data
	docker compose down -v

.PHONY: run
run: ## Migrate the database, then run the server
	go run ./cmd/server

.PHONY: migrate
migrate: ## Apply pending migrations and exit
	go run ./cmd/server migrate

.PHONY: build
build: ## Build the server binary into bin/
	CGO_ENABLED=0 go build -trimpath -o bin/server ./cmd/server

.PHONY: test
test: ## Run every test, including integration tests (needs `make up`)
	go test -race -count=1 -timeout=120s ./...

.PHONY: test-unit
test-unit: ## Run the tests that need no infrastructure
	TEST_DATABASE_URL= go test -race -count=1 -timeout=60s ./...

.PHONY: arch
arch: ## Check the architecture rules only
	go test -count=1 ./internal/archtest/

.PHONY: lint
lint: ## Run the linters
	$(GOLANGCI_LINT) run ./...

.PHONY: fmt
fmt: ## Format the code
	$(GOLANGCI_LINT) fmt ./...

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	go mod tidy

.PHONY: check
check: tidy lint test ## Everything CI runs
