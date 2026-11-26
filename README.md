# go-modular-monolith

A blueprint for building services in Go as a **modular monolith**: one binary,
with business modules split by domain, strict boundaries between them, and
shared functionality lifted into a small kernel and platform layer.

Read [ARCHITECTURE.md](ARCHITECTURE.md) for the rules and the reasoning. This
file is about getting it running.

## What is in the box

- Three example modules (`customers`, `orders`, `notifications`) showing a
  synchronous cross-module query and an asynchronous integration event.
- Transactional outbox, so events are never lost and never announced for a
  rolled-back transaction.
- One PostgreSQL schema and one migration history per module.
- Standard library HTTP with RFC 9457 error responses, request IDs,
  structured logging, health probes and graceful shutdown.
- Architecture rules enforced as tests (`internal/archtest`).
- Makefile, Dockerfile, docker compose, golangci-lint config and a CI workflow.

Dependencies are deliberately few: `pgx`, `goose`, `uuid`, `x/sync`.

## Quick start

Requires Go 1.26+ and Docker.

```bash
make up
```

```bash
make run
```

The server migrates the database and listens on `:8080`. In another terminal:

```bash
curl -s -X POST localhost:8080/api/v1/customers \
  -d '{"name":"Ada Lovelace","email":"ada@example.com"}'
```

Use the returned `id` to place an order:

```bash
curl -s -X POST localhost:8080/api/v1/orders \
  -d '{"customer_id":"<id>","currency":"EUR","lines":[{"sku":"BOOK-1","quantity":2,"unit_price":1250}]}'
```

Within a second the server log shows `notification sent`: `orders` stored the
order and an `OrderPlaced` event in one transaction, the outbox relay
delivered the event, and `notifications` reacted to it.

Alternatively, run everything in containers:

```bash
docker compose up --build
```

## Everyday commands

| Command | What it does |
| --- | --- |
| `make up` / `make down` | Start PostgreSQL / stop it and delete its data |
| `make run` | Migrate, then run the server |
| `make migrate` | Apply migrations and exit (use this as a deploy step) |
| `make test` | All tests with `-race`, including integration tests |
| `make test-unit` | Only the tests that need no database |
| `make arch` | Only the architecture rules |
| `make lint` / `make fmt` | Lint / format |
| `make check` | Everything CI runs |

Integration tests create a throwaway database per test on the server named by
`TEST_DATABASE_URL`, and are skipped when it is unset. Configuration is
documented in [.env.example](.env.example).

## API

| Method and path | Description |
| --- | --- |
| `POST /api/v1/customers` | Register a customer |
| `GET /api/v1/customers/{id}` | Fetch a customer |
| `POST /api/v1/orders` | Place an order for an existing customer |
| `GET /api/v1/orders/{id}` | Fetch an order |
| `GET /healthz` | Liveness |
| `GET /readyz` | Readiness (checks PostgreSQL) |

Errors are `application/problem+json` with a stable `code`, for example
`customers.email_taken` or `orders.unknown_customer`.

## Starting a new service from this blueprint

1. Create the new repository from this one (mark this repository as a
   template in its GitHub settings, then "Use this template"), or copy the
   files.
2. Rename the Go module:

   ```bash
   scripts/rename-module.sh github.com/your-org/your-service
   ```

3. Delete the example modules under `internal/modules/` and their wiring in
   `internal/bootstrap/app.go`, then add your own following
   [the recipes](ARCHITECTURE.md#9-recipes).
4. Keep `internal/kernel`, `internal/platform`, `internal/archtest` and
   `ARCHITECTURE.md`, and evolve them with the service.

## Layout

```text
cmd/server/          entry point
internal/bootstrap/  composition root: the only code that sees every module
internal/kernel/     shared domain vocabulary (pure)
internal/platform/   shared technical capabilities (HTTP, Postgres, events, config)
internal/modules/    business modules, one directory per domain
internal/archtest/   the architecture rules, as tests
```
