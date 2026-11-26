# 0003. One database schema and role per module

- Status: Accepted
- Date: 2026-11-26

## Context

Shared tables are the coupling that code review misses. A join or foreign
key across modules is invisible to the compiler, and it is what makes a
module impossible to extract later. "Please do not query other modules'
tables" does not survive deadlines.

## Decision

Each module owns one PostgreSQL schema, named after it, with its own
migration history. Migrations create a role `<module>_rw` that can use that
schema only. At runtime each module receives its own connection pool whose
sessions run as that role. There are no foreign keys or joins across
schemas; other modules' data is reached through their contract or copied via
events.

## Consequences

- A query against another module's tables fails with "permission denied",
  in tests and in production.
- Modules cannot starve each other of connections, because pools are
  separate.
- The service opens more connections in total; size `DATABASE_MAX_CONNS`
  with the number of modules in mind.
- The user that runs migrations must be allowed to create roles, and
  PostgreSQL 16 or newer is required.
- Reports that need data from several modules need a read model or several
  calls, never a cross-schema join.
