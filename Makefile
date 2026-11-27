# Developer entry points. Run `make` to list them.

ifneq (,$(wildcard .env))
include .env
export
endif

TOOLCHAIN := GOTOOLCHAIN=$(shell go env GOVERSION)

GOLANGCI_LINT_VERSION ?= v2.15.0
GOLANGCI_LINT := $(TOOLCHAIN) go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOVULNCHECK   := $(TOOLCHAIN) go run golang.org/x/vuln/cmd/govulncheck@latest

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE   ?= $(notdir $(CURDIR)):$(VERSION)

.DEFAULT_GOAL := help

.PHONY: help
help: ## List available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

##@ Run

.PHONY: run
run: ## Run the server locally on :8080
	go run .

.PHONY: up-observability
up-observability: ## Start a local Grafana stack for traces and metrics (localhost:3000)
	docker compose --profile observability up -d --wait

.PHONY: down
down: ## Stop local Docker services
	docker compose --profile observability down -v

##@ Develop

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
test: ## Run every test with the race detector
	go test -race -count=1 -timeout=120s ./...

.PHONY: cover
cover: ## Run every test and report coverage
	go test -race -count=1 -timeout=120s -coverprofile=coverage.out -covermode=atomic -coverpkg=./app/...,./modules/...,./pkg/... ./...
	@go tool cover -func=coverage.out | tail -1

.PHONY: arch
arch: ## Check architecture rules only
	go test -count=1 ./test/arch/

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
build: ## Build the server binary into bin/server
	CGO_ENABLED=0 go build -trimpath -o bin/server .

.PHONY: image
image: ## Build the container image
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE) .
