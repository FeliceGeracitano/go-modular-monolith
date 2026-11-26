# go-modular-monolith

A template for modular Go projects.

Start a new service from this repository and you get a working **modular
monolith**: a single binary whose business code is split into modules by
domain, with enforced boundaries between them and shared functionality lifted
into a small common layer.

The rules and the reasoning behind them are in
[ARCHITECTURE.md](ARCHITECTURE.md).

## What you get

- **Modules split by domain.** Each module keeps its internals private,
  exposes a small contract package, and owns its database schema and
  migrations.
- **Shared code in one place.** `kernel` holds shared domain types;
  `platform` holds shared infrastructure (HTTP, PostgreSQL, events, config).
- **Two ways for modules to talk.** Direct calls through a contract, and
  events delivered through a transactional outbox.
- **Boundaries that are enforced.** Architecture tests fail the build when an
  import crosses a line it should not.
- **Production basics.** Structured logging, request IDs, consistent error
  responses, health probes, graceful shutdown.
- **Tooling.** Makefile, Dockerfile, docker compose, linter config, CI.

Three small example modules (`customers`, `orders`, `notifications`) show the
patterns in working code. Replace them with your own.

## Use this template

1. Create your repository from this one (**Use this template** on GitHub, or
   copy the files).
2. Rename the Go module:

   ```bash
   scripts/rename-module.sh github.com/your-org/your-service
   ```

3. Remove the example modules from `internal/modules/` and their wiring in
   `internal/bootstrap/app.go`.
4. Add your own modules by following the
   [recipes](ARCHITECTURE.md#9-recipes).

Keep `internal/kernel`, `internal/platform`, `internal/archtest` and
`ARCHITECTURE.md`, and evolve them with your service.

## Run it

Requires Go 1.26+ and Docker.

Start PostgreSQL:

```bash
make up
```

Migrate the database and start the server on `:8080`:

```bash
make run
```

Try it from another terminal. Register a customer:

```bash
curl -s -X POST localhost:8080/api/v1/customers \
  -d '{"name":"Ada Lovelace","email":"ada@example.com"}'
```

Place an order, using the `id` from the response:

```bash
curl -s -X POST localhost:8080/api/v1/orders \
  -d '{"customer_id":"<id>","currency":"EUR","lines":[{"sku":"BOOK-1","quantity":2,"unit_price":1250}]}'
```

The server log then shows `notification sent`: the `orders` module published
an event and the `notifications` module reacted to it.

## Project layout

```text
cmd/server/            entry point
internal/
  bootstrap/           wires everything together; the only code that sees every module
  kernel/              shared domain types (errors, IDs, money, clock)
  platform/            shared infrastructure (HTTP, PostgreSQL, events, config)
  modules/             business modules, one directory per domain
    customers/
    orders/
    notifications/
  archtest/            the architecture rules, as tests
```

Inside a module:

```text
modules/orders/
  module.go            wiring
  ordersapi/           public contract: the only package other modules may import
  migrations/          SQL for the module's own schema
  internal/            private to the module
    domain/            business rules
    app/               use cases
    adapters/          HTTP handlers, repositories, clients
```

## Commands

| Command | Description |
| --- | --- |
| `make up` | Start PostgreSQL |
| `make down` | Stop PostgreSQL and delete its data |
| `make run` | Migrate, then run the server |
| `make migrate` | Apply migrations and exit |
| `make test` | Run all tests (needs `make up`) |
| `make test-unit` | Run the tests that need no database |
| `make arch` | Check the architecture rules |
| `make lint` | Run the linters |
| `make fmt` | Format the code |
| `make check` | Run everything CI runs |

## Configuration

The server is configured through environment variables. Only `DATABASE_URL`
is required; every setting and its default is listed in
[.env.example](.env.example).

## Example API

| Endpoint | Description |
| --- | --- |
| `POST /api/v1/customers` | Register a customer |
| `GET /api/v1/customers/{id}` | Fetch a customer |
| `POST /api/v1/orders` | Place an order |
| `GET /api/v1/orders/{id}` | Fetch an order |
| `GET /healthz` | Liveness probe |
| `GET /readyz` | Readiness probe |
