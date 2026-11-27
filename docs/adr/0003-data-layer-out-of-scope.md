# 0003. Keep the data layer out of the template

- Status: Accepted
- Date: 2026-11-27

## Context

Different Go services built from this template need different storage
technologies: PostgreSQL, SQLite, DynamoDB, Redis, external APIs, or no local
database at all. Baking a specific database driver and SQL migration engine
into the base template forces every new service to inherit vendor-specific
infrastructure and local Docker dependencies before writing any domain code.

## Decision

The template does not include a database or migration layer. Every module
declares its own persistence ports in `internal/app` and ships with a
thread-safe in-memory adapter (`internal/adapters/memstore`). Asynchronous
integration events use the outbox pattern (`internal/platform/outbox`) backed
by a storage-agnostic `outbox.Store` interface with an in-memory default
(`outbox.NewMemoryStore`) and a background `outbox.Relay` dispatching to
`internal/platform/eventbus`.

When a real service adopts a database, each module swaps `memstore` for a real
outbound adapter and `bootstrap` swaps `outbox.MemoryStore` for a persistent
`outbox.Store` without changing domain or application code.

## Consequences

- Zero external infrastructure required to run or test (`make run`, `make test`).
- Both sync (contract ACL) and async (outbox + relay + idempotent subscriber)
  patterns work out of the box in pure Go.
- Minimal `go.mod` dependency footprint with no database or vendor lock-in.
- Durability and atomic state+event commits are plugged in once a concrete
  store is chosen, using the seams already present.
