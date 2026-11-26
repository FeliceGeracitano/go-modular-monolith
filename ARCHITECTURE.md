# Architecture

This repository is a blueprint for a **modular monolith** in Go: one deployable
binary, one Go module, one database server, and inside it a set of business
modules that are as isolated from each other as separate services would be.

You get the operational simplicity of a monolith (one build, one deploy, local
function calls, real transactions) and keep the option of extracting a module
into its own service later, because the seams are already there.

The rules below are not aspirations. Each is enforced by the compiler, by
`internal/archtest`, or by the linter, and the last section says which.

## Contents

1. [The shape of the system](#1-the-shape-of-the-system)
2. [Dependency rules](#2-dependency-rules)
3. [Anatomy of a module](#3-anatomy-of-a-module)
4. [How modules talk to each other](#4-how-modules-talk-to-each-other)
5. [Data ownership and transactions](#5-data-ownership-and-transactions)
6. [Shared code: kernel and platform](#6-shared-code-kernel-and-platform)
7. [Cross-cutting conventions](#7-cross-cutting-conventions)
8. [Testing strategy](#8-testing-strategy)
9. [Recipes](#9-recipes)
10. [What is deliberately left out](#10-what-is-deliberately-left-out)
11. [Extracting a module into a service](#11-extracting-a-module-into-a-service)
12. [How the rules are enforced](#12-how-the-rules-are-enforced)

## 1. The shape of the system

```text
cmd/server/                 process entry point: flags, signals, exit code
internal/
  bootstrap/                composition root: builds everything, wires modules together
  kernel/                   shared *domain* vocabulary; pure, no I/O
    apperr/                   classified errors (invalid, not found, conflict, ...)
    clock/                    time as a dependency
    event/                    what an integration event is
    id/                       UUIDv7 identifiers
    money/                    amounts in minor units
  platform/                 shared *technical* capabilities; knows no business
    config/                   environment -> typed configuration
    eventbus/                 in-process event routing
    httpx/                    JSON, problem responses, middleware, server lifecycle
    logging/                  slog setup, request-scoped loggers
    module/                   the interface every module implements
    outbox/                   transactional outbox: publisher + relay
    postgres/                 pool, context-propagated transactions, migrations
  modules/                  the business, split by domain
    customers/
    orders/
    notifications/
  archtest/                 the rules in this document, as tests
```

Three kinds of code, with three different jobs:

| Layer | Answers the question | May depend on |
| --- | --- | --- |
| **modules** | What does the business do? | kernel, platform, other modules' *contracts* |
| **platform** | How do we talk to the outside world? | kernel |
| **kernel** | What words do all modules share? | nothing |

`bootstrap` and `cmd` sit above all three and are the only code allowed to see
everything.

The example domain is intentionally small: `customers` registers customers,
`orders` places orders for existing customers, `notifications` confirms placed
orders. Together they exercise every communication pattern the blueprint
supports. Delete them when you start a real service; keep the structure.

## 2. Dependency rules

```mermaid
flowchart TD
    cmd[cmd/server] --> bootstrap[internal/bootstrap]
    bootstrap --> modules
    bootstrap --> platform

    subgraph modules[internal/modules]
        orders[orders] -. contract only .-> customersapi[customers/customersapi]
        notifications[notifications] -. contract only .-> customersapi
        notifications -. contract only .-> ordersapi[orders/ordersapi]
    end

    modules --> platform[internal/platform]
    modules --> kernel[internal/kernel]
    platform --> kernel
```

1. **Dependencies point downwards.** `kernel` imports nothing from this
   repository. `platform` imports only `kernel`. Neither ever imports a module.
2. **A module's only public surface is its contract package**
   (`modules/<name>/<name>api`). Other modules may import that and nothing
   else from the module.
3. **Contracts are leaves.** A contract imports only the kernel and the
   standard library, so contracts can never form an import cycle and importing
   one drags in no implementation.
4. **Inside a module, dependencies point inwards:**
   `adapters -> app -> domain`. Domain and application code import no
   third-party packages and no platform packages; they reach the outside world
   through interfaces they declare themselves.
5. **Only the composition root sees concrete modules.** `internal/bootstrap`
   is the single place that imports `modules/<name>` and decides who gets
   which dependency.
6. **No junk drawers.** There is no `common`, `utils`, `helpers` or `shared`
   package. Shared code is named after the one thing it provides.

Go's `internal/` directories give rule 2 teeth for free: everything under
`modules/orders/internal/` is unimportable from outside `modules/orders/`, and
the compiler says so. The remaining rules are checked by `internal/archtest`.

## 3. Anatomy of a module

```text
modules/orders/
  module.go                 wiring: Deps in, module.Module out. Imported only by bootstrap.
  ordersapi/                PUBLIC contract: DTOs, interfaces, integration events
  migrations/               SQL owned by this module (schema "orders")
  internal/                 PRIVATE; the compiler keeps other modules out
    domain/                   entities, value objects, invariants. Pure.
    app/                      use cases + the ports (interfaces) they need
    adapters/
      httpapi/                inbound: HTTP -> use case
      pgstore/                outbound: repository port -> PostgreSQL
      customersacl/           outbound: port -> another module's contract
```

**`domain`** holds the rules that would still be true if there were no
database and no HTTP: what a valid order is, how a total is computed. It is
plain Go, trivially unit-tested, and has no idea how it is stored or served.

**`app`** holds the use cases (`PlaceOrder`, `GetOrder`). A use case loads and
saves through *ports*: small interfaces declared right there, next to the code
that calls them, describing what the use case needs in its own words
(`Repository`, `CustomerDirectory`, `EventPublisher`, `Transactor`). The app
layer decides transaction boundaries; it never knows what implements a port.

**`adapters`** connect ports to technology. Inbound adapters (HTTP handlers,
event subscribers) translate a transport message into a use-case call.
Outbound adapters (repositories, clients) implement ports. All SQL, JSON tags,
status codes and third-party imports live here.

**`<name>api`** is the contract other modules compile against. It contains
data and interfaces, never behaviour. Its types are separate from the domain
model on purpose: the domain must be free to change without breaking
consumers, and consumers must never see more than they were promised.

**`module.go`** assembles the above from a `Deps` struct and implements
`platform/module.Module` (name, migrations, routes, subscribers).

### Layers scale with the module, not the other way round

The layout above is for a module with real business rules. Do not apply it
mechanically. `notifications` has no `domain` package because it has no
invariants worth isolating; a module that is five functions can be one
package. Start with the least structure that keeps the dependency direction
right, and split a package when it has grown a boundary worth naming. What is
not negotiable is the outside of the module: a contract package, private
internals, its own schema.

## 4. How modules talk to each other

Modules never share tables, never call each other's internals, and never
reach each other through HTTP on localhost. There are exactly two ways.

### 4.1 Synchronous: a query through a contract

Use this when a module needs an answer *now* to do its own job.
`orders` must know that a customer exists before accepting an order:

```text
orders/internal/app            declares the port     CustomerDirectory.Exists(ctx, id)
orders/internal/adapters/
       customersacl            implements it using   customersapi.API.GetCustomer
customers/customersapi         declares the contract API
customers/module.go            implements it         on top of its own app.Service
bootstrap                      connects them         Customers: customersModule.API()
```

Three things to notice:

- **The consumer owns the interface.** `orders` asks only "does this customer
  exist?", the one question it has, not for the whole customers API. The port
  documents exactly what `orders` depends on and is trivial to fake in tests.
- **The adapter is an anti-corruption layer.** `customersacl` is the only
  file in `orders` that knows the customers contract exists. If that contract
  changes, one small file changes.
- **It is an in-process function call**, wired by hand in `bootstrap`. No
  service locator, no registry, no reflection.

Keep these calls to *queries*. If module A needs module B to *change state*
as part of A's use case, that is a workflow, and it belongs in an event
(below) or in a module that sits above both.

### 4.2 Asynchronous: integration events through the outbox

Use this when a module announces something that happened and does not care
who reacts. `orders` publishes `OrderPlaced`; `notifications` reacts:

```text
orders/ordersapi               defines the event     OrderPlaced ("orders.order_placed.v1")
orders/internal/app            publishes it          inside the transaction that saves the order
platform/outbox                stores it             INSERT into outbox.messages, same transaction
platform/outbox (relay)        delivers it           after commit, to every subscriber
notifications/.../subscribers  receives it           translates it into a use-case call
```

**Why an outbox?** Saving the order and announcing it must be atomic. Publish
before the commit and you may announce an order that never existed; publish
after and a crash loses the event. The outbox writes the event *with* the
order, in one transaction, and a relay delivers it afterwards.
`outbox.Publisher` refuses to run outside a transaction, so this cannot be
forgotten.

**What subscribers must assume:**

- Delivery is **at least once**. A handler may see the same event twice, so
  it must be idempotent. `notifications` stores each notification with a
  unique `event_id` and treats a duplicate as already done.
- Delivery is **asynchronous**. The publisher's request returns before any
  subscriber ran. Subscribers are eventually consistent with the publisher.
- **Ordering is not guaranteed** once retries are involved. Put what the
  handler needs in the event rather than relying on sequence.
- A handler that keeps failing is retried with backoff up to
  `OUTBOX_MAX_ATTEMPTS`, then left in `outbox.messages` with its last error
  for a human to look at. Monitor that table.

**Events are a published API.** The name carries a version
(`orders.order_placed.v1`). Add optional fields freely; for a breaking change
publish `v2` alongside `v1` until every subscriber has moved.

### 4.3 Choosing

| Situation | Use |
| --- | --- |
| I need data owned by another module to validate or answer a request | Synchronous query |
| Something happened that others may care about | Event |
| Another module must do work as a consequence of mine | Event |
| Two modules need each other | Events in one direction, or a third module that orchestrates both |
| I want a foreign key or a join across modules | Neither; see the next section |

When two modules start importing each other's contracts in both directions,
do not hide the cycle behind callbacks or a registry. Decide which one owns
the operation, turn the other direction into an event, or introduce a
higher-level module whose job is the workflow.

## 5. Data ownership and transactions

**One module, one schema.** `customers` owns the PostgreSQL schema
`customers`, `orders` owns `orders`, and so on. A module's SQL never names
another module's schema.

**No cross-module foreign keys or joins.** `orders.orders.customer_id` is a
plain `uuid`, not a reference. This is the single most important rule for
keeping modules separable: a foreign key is a compile-time dependency that
the compiler cannot see. Need data from two modules together? Ask each one
through its contract, or build a read model fed by events.

**Each module migrates itself.** Migrations live in the module, are embedded
in the binary, and have their own history table (`<schema>.goose_db_version`).
`server migrate` applies them under an advisory lock, so concurrent deploys
are safe. Migrations are forward-only in production: ship additive changes,
and remove columns one release after the code stopped using them.

**The application layer owns transactions.** A use case wraps its writes in
`tx.WithinTx(ctx, func(ctx) error { ... })`. The transaction travels in the
context; repositories call `db.Conn(ctx)` and get the transaction if there is
one, the pool if not. Repositories therefore never begin or commit anything
on their own account, and the same repository method works inside and outside
a transaction.

**A transaction never spans modules.** A use case writes its own module's
tables plus the outbox, and that is all. Consistency *between* modules is
eventual and travels by event. This is exactly the constraint you would have
with separate services, accepted early so that it never becomes a surprise.

A note on context: the transaction and the request-scoped logger ride in
`context.Context` because they are request-scoped *state*. Dependencies
(repositories, clients, other modules) never do; they are constructor
arguments.

## 6. Shared code: kernel and platform

Functionality used by more than one module is lifted up, but into one of two
places with very different rules.

**`internal/kernel`** is the shared domain vocabulary: types that mean the
same thing in every module. It is pure (no I/O, no logging, no third-party
imports beyond a short allow-list), tiny, and changes rarely, because a
change here is a change to every module at once.

**`internal/platform`** is technical capability: configuration, HTTP,
database, messaging. It may use third-party libraries freely, and it knows
nothing about customers or orders. Swapping PostgreSQL drivers or the event
transport should touch `platform` and adapters only.

### When to lift code up

Lifting is a one-way door, so ask first:

1. **Do the consumers agree on the meaning, or only on the syntax?** Two
   modules both having a `Customer` struct is not duplication; they mean
   different things by it. `money.Money` is shared because every module agrees
   what an amount is. A module that needs signed refunds or sub-cent prices
   should define its own type instead of loosening the shared one.
2. **Is it the third use?** Two similar pieces of code in two modules are
   cheaper than one wrong abstraction coupling both. Duplicate once; lift on
   the third occurrence, when the common shape is actually known.
3. **Does it have a name?** If the best name is `utils`, it is not one thing
   yet. `archtest` rejects junk-drawer names.
4. **Who owns it?** Shared code needs tests and an owner like any module.

What never gets lifted: business rules ("an order needs at least one line"),
persistence models, and anything that would make `kernel` or `platform`
import a module.

When several *services* built from this blueprint need the same library,
that is the moment for a separately versioned Go module, not before.

## 7. Cross-cutting conventions

**Errors.** Business code returns `*apperr.Error` values that carry a `Kind`
(invalid, not found, conflict, ...), a stable namespaced `Code`
(`orders.unknown_customer`) and a client-safe message. Each module declares
its errors as package-level values in its domain. `httpx.Error` maps kinds to
status codes and renders RFC 9457 problem documents. Anything unclassified is
logged in full and returned as an opaque 500, so internals never leak; a
deadline becomes 504 and a client cancellation 408. Wrap with `%w` and test
with `errors.Is`.

**Validation happens twice, on purpose.** The transport adapter rejects
malformed input (bad JSON, unknown fields, oversized bodies). The domain
enforces business invariants, because HTTP is not the only caller.

**HTTP.** Standard library `net/http` with method-and-path patterns. Each
module registers its routes under `/api/v1/<module>`. Request and response
structs live in the adapter and are distinct from domain types, so renaming a
domain field can never silently change the public API.

**Configuration.** Environment variables, read once at startup by
`platform/config`, validated all at once, and passed down as values. Modules
never read the environment. See `.env.example`.

**Logging.** `log/slog`, JSON by default. Middleware attaches a request ID
and a request-scoped logger to the context; use the `*Context` logging
methods. Access logs record the matched route pattern rather than raw paths
or query strings.

**Health.** `/healthz` is liveness and checks nothing, so a database outage
does not get the process restarted. `/readyz` checks dependencies and takes
the instance out of rotation while they are down.

**Lifecycle.** `cmd/server` turns SIGINT/SIGTERM into context cancellation.
`bootstrap.App.Run` supervises the HTTP server and the outbox relay; when
either stops, both shut down. The listener is bound before serving so a port
conflict fails startup, and in-flight requests get `HTTP_SHUTDOWN_TIMEOUT` to
finish. Whoever constructs a resource closes it: `bootstrap` opens the pool,
`bootstrap` closes it, modules only borrow.

**Dependency injection** is constructors and structs. `bootstrap/app.go` is
the dependency graph, written as ordinary code that the compiler checks.

**IDs and time.** IDs are UUIDv7, generated in the application so an entity
is complete before it is stored. Time comes from an injected `clock.Clock`,
never `time.Now()` in business code.

## 8. Testing strategy

| Level | What it proves | How | Speed |
| --- | --- | --- | --- |
| Domain | Business rules | Plain table-driven unit tests | Instant |
| Application | Use-case orchestration: what is stored, published, rejected | Hand-written fakes of the ports | Instant |
| Platform | Infrastructure behaves (outbox atomicity, retries) | Real PostgreSQL | Fast |
| End to end | The wired application works: HTTP in, SQL and events underneath | `bootstrap.New` + `httptest`, real PostgreSQL | Fast |
| Architecture | Nobody crossed a boundary | `internal/archtest` parses every import | Instant |

- **Fakes, not mocks.** Ports are small, so a fake is a few lines and tests
  assert on outcomes rather than on call sequences.
- **Real database for integration tests.** `pgtest` gives every test its own
  throwaway database, so tests are isolated and parallel-safe. They skip when
  `TEST_DATABASE_URL` is unset, which keeps `go test ./...` green anywhere.
- **Drive events deterministically.** Tests call `Relay().ProcessBatch`
  instead of sleeping and hoping the poller ran.
- Always run with `-race`.

## 9. Recipes

### Add a module

1. Create `internal/modules/<name>/` with `module.go`, and
   `internal/app` (plus `internal/domain` when there are real invariants).
2. If it stores data: add `migrations/00001_*.sql` creating tables in schema
   `<name>`, and an `adapters/pgstore` repository.
3. If it serves HTTP: add `adapters/httpapi` and register routes under
   `/api/v1/<name>`.
4. If other modules will use it: add `<name>api/` with the contract.
5. Implement `module.Module` and construct the module in
   `internal/bootstrap/app.go`, passing in what it needs.
6. Run `make check`. The architecture tests will tell you if a boundary
   was crossed.

### Call another module

1. Declare a port in *your* `app` package, phrased as the question you have.
2. Implement it in `adapters/<other>acl` on top of `<other>api`.
3. Accept `<other>api.API` in your `Deps` and wire it in `bootstrap`.

If the other module's contract lacks what you need, extend its `<name>api`
and its `module.go` facade. Never reach for its internals or its tables.

### Publish an event

1. Define the event struct and its versioned name in `<name>api`.
2. In the use case, call `events.Publish(ctx, evt)` **inside** `WithinTx`,
   next to the write it announces.

### React to an event

1. Add a use case in your `app` package taking your own input type.
2. Subscribe in `adapters/subscribers` with `eventbus.Typed`, translating the
   publisher's event into that input and passing `env.ID` along.
3. Make the use case idempotent on the event ID.

### Integrate an external service

Put the client in an outbound adapter of the module that owns the
integration, behind a port. Construct the underlying `http.Client` once in
`bootstrap` with explicit timeouts and inject it; the adapter borrows it and
never closes it. Keep provider payloads inside the adapter and translate to
domain values. If two providers share mechanics (bounded JSON reads, retry
policy), lift the *mechanics* into `platform`, never the provider's meaning.

## 10. What is deliberately left out

Each of these is a decision to make when a real need shows up, not a gap:

- **A router or web framework.** The standard library router covers methods
  and path parameters. Handlers are plain `http.Handler`s, so adopting chi or
  similar later changes route registration only.
- **A DI container** (Fx, Wire). Hand wiring is a page of code today. Revisit
  when `bootstrap` becomes a maintenance burden, and keep constructors usable
  without the container.
- **An ORM or query generator.** Repositories use SQL through pgx. sqlc fits
  well if hand-written scanning becomes tedious; it stays inside adapters.
- **Authentication and authorization.** Add authentication as `httpx`
  middleware that puts a principal in the context; keep authorization
  decisions in the application layer of each module.
- **Metrics and tracing.** OpenTelemetry belongs in `platform`, as middleware
  and as wrappers around the pool and the relay.
- **A message broker.** The in-process bus sits behind `outbox.Dispatcher`.
  Introduce a broker when a second process needs the events.
- **Pagination, API documentation, rate limiting, idempotency keys.** Add
  them to `httpx` once, when the first endpoint needs them.

## 11. Extracting a module into a service

The rules above exist so this stays a mechanical change:

1. The module already owns its schema: move the schema to its own database.
2. Its contract package becomes the client library: implement
   `<name>api.API` with an HTTP or gRPC client instead of the in-process
   facade. Consumers are untouched because they only ever saw the interface
   (now is when their ports need timeouts and failure handling).
3. Its events already flow through the outbox: point the relay's dispatcher
   at a broker and have subscribers consume from it.
4. Give it its own `cmd/` and `bootstrap`. `kernel` and `platform` become a
   shared library module.

What extraction cannot fix is hidden coupling: a cross-schema join, a shared
transaction, a struct passed between modules. That is why those are banned
from day one.

## 12. How the rules are enforced

| Rule | Enforced by |
| --- | --- |
| Module internals are private | Go compiler (`internal/` directories) |
| Kernel and platform never import modules | `internal/archtest` |
| Modules see each other only through contracts | `internal/archtest` |
| Contracts are leaf packages | `internal/archtest` |
| Domain and app are free of infrastructure and third-party code | `internal/archtest` |
| No `utils` / `common` packages | `internal/archtest` |
| No import cycles | Go compiler |
| Events are published transactionally | `outbox.Publisher` returns `ErrNoTransaction` otherwise |
| Schema-per-module, no cross-schema SQL | Code review (grep for another module's schema name) |
| Idiomatic, safe Go | `golangci-lint` (`.golangci.yml`) |

`internal/archtest/rules.go` is short and meant to be read: it is this
document in executable form. When a rule needs to change, change it there, in
a commit that says why, and update this file with it.
