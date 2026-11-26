# 0005. Modules declare their dependencies

- Status: Accepted
- Date: 2026-11-26

## Context

Restricting modules to each other's contracts keeps implementations private
but says nothing about which modules may depend on which. With many modules,
dependencies added one convenient import at a time grow into a graph nobody
designed, including cycles that make modules impossible to reason about or
extract separately.

## Decision

Every module has a `module.json` naming the modules whose contract it uses.
The architecture tests fail when code imports an undeclared contract, when a
declared dependency is unused, and when the declared graph has a cycle.
`docs/MODULES.md` is generated from the same data and checked in CI.

## Consequences

- A new dependency between modules is always a visible line in a diff, owned
  by the module's reviewers.
- The dependency graph is documented, current and acyclic by construction.
- Two modules that need each other must resolve it by design: an event in
  one direction, or a third module that orchestrates both.
- One more file per module to keep honest, which the tests do for us.
