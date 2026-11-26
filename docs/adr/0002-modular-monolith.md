# 0002. Build a modular monolith

- Status: Accepted
- Date: 2026-11-26

## Context

A new service needs clear internal boundaries so that it stays changeable as
it grows, but it does not yet have the team size, traffic or operational
maturity that justify a distributed system. Microservices buy independent
deployment at the price of network calls, partial failure and distributed
data from day one. An unstructured monolith is cheap to start and expensive
to keep.

## Decision

We will build one deployable binary from one Go module, organised as
business modules under `internal/modules/`. Each module keeps its
implementation private (`internal/`), exposes a small contract package
(`<name>api`) as its only public surface, and owns its data. Shared code is
split into a pure `kernel` and a technical `platform`. The boundaries are
enforced by the compiler and by `internal/archtest`.

## Consequences

- One build, one deploy, in-process calls and real transactions inside a
  module.
- Modules can be extracted into services later along seams that already
  exist: the contract becomes a client, the schema becomes a database.
- Boundaries cost some ceremony: contract packages, adapters between
  modules, no shortcuts through another module's tables.
- Rejected: microservices from the start (operational cost without a present
  need) and a layered monolith without module boundaries (coupling by
  default).
