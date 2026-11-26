# Developer entry points. Run `make` to list them.

# Local settings live in .env (copy .env.example). Deployed environments set
# real environment variables instead; see deploy/env/.
ifneq (,$(wildcard .env))
include .env
export
endif

# Tools are built with the same Go toolchain as the service, so they always
# understand the language version in go.mod.
TOOLCHAIN := GOTOOLCHAIN=$(shell go env GOVERSION)

GOLANGCI_LINT_VERSION ?= v2.15.0
GOLANGCI_LINT := $(TOOLCHAIN) go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOVULNCHECK   := $(TOOLCHAIN) go run golang.org/x/vuln/cmd/govulncheck@latest

DATABASE_URL      ?= postgres://app:app@localhost:5432/app?sslmode=disable
TEST_DATABASE_URL ?= $(DATABASE_URL)
export DATABASE_URL TEST_DATABASE_URL

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE   ?= $(notdir $(CURDIR)):$(VERSION)

.DEFAULT_GOAL := help

.PHONY: help
help: ## List the available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

##@ Run

.PHONY: up
up: ## Start PostgreSQL in the background
	docker compose up -d --wait postgres

.PHONY: up-observability
up-observability: ## Also start a local Grafana stack for traces and metrics (localhost:3000)
	docker compose --profile observability up -d --wait

.PHONY: down
down: ## Stop everything and delete local data
	docker compose --profile observability down -v

.PHONY: run
run: ## Run everything in one process: migrate, API and worker
	go run ./cmd/server

.PHONY: run-api
run-api: ## Run only the HTTP API
	go run ./cmd/api

.PHONY: run-worker
run-worker: ## Run only the event worker
	go run ./cmd/worker

.PHONY: migrate
migrate: ## Apply pending migrations and exit
	go run ./cmd/migrate

##@ Develop

.PHONY: module
module: ## Create and wire a new module: make module name=billing
	@test -n "$(name)" || { echo "usage: make module name=<name>"; exit 2; }
	go run ./tools/newmodule -name "$(name)" -description "$(description)"

.PHONY: docs
docs: ## Regenerate docs/MODULES.md from the code
	go run ./tools/modulemap

.PHONY: remove-examples
remove-examples: ## Delete the example modules, leaving an empty application
	scripts/remove-examples.sh

.PHONY: fmt
fmt: ## Format the code
	$(GOLANGCI_LINT) fmt ./...

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	go mod tidy

##@ Verify

.PHONY: test
test: ## Run every test, including integration tests (needs `make up`)
	go test -race -count=1 -timeout=180s ./...

.PHONY: test-unit
test-unit: ## Run the tests that need no infrastructure
	TEST_DATABASE_URL= go test -race -count=1 -timeout=60s ./...

.PHONY: cover
cover: ## Run every test and report coverage
	go test -race -count=1 -timeout=180s -coverprofile=coverage.out -covermode=atomic -coverpkg=./internal/... ./...
	@go tool cover -func=coverage.out | tail -1

.PHONY: arch
arch: ## Check the architecture rules only
	go test -count=1 ./internal/archtest/

.PHONY: lint
lint: ## Run the linters
	$(GOLANGCI_LINT) run ./...

.PHONY: vuln
vuln: ## Check dependencies and the Go toolchain for known vulnerabilities
	$(GOVULNCHECK) ./...

.PHONY: check
check: tidy lint test ## What CI runs on every pull request

##@ Build

.PHONY: build
build: ## Build every binary into bin/
	CGO_ENABLED=0 go build -trimpath -o bin/ ./cmd/...

.PHONY: image
image: ## Build the container image
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE) .
