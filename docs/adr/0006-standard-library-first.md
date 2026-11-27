# 0006. Standard library first, explicit wiring

- Status: Accepted
- Date: 2026-11-26

## Context

Frameworks decide the shape of a codebase and are expensive to remove. Go's
standard library now covers routing with methods and path parameters,
structured logging and graceful shutdown. Dependency injection containers
trade compile-time checking for convenience that a small graph does not
need.

## Decision

We will use `net/http`, `log/slog` and plain constructors, wiring the
application by hand in `internal/bootstrap`. Third-party libraries are added
only for capabilities the standard library lacks (OpenTelemetry, UUIDv7) and
stay confined to `platform`, adapters, and a tiny kernel allow-list.
Domain and application code import no third-party packages.

## Consequences

- Little to learn beyond Go itself, and few dependencies to keep patched.
- The wiring is ordinary code: readable top to bottom, checked by the
  compiler.
- Adopting a router or a DI container later touches route registration or
  `bootstrap` only, because business code never sees them.
- Some conveniences are written by hand (JSON helpers, middleware).
