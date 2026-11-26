# 0007. One image, several process roles

- Status: Accepted
- Date: 2026-11-26

## Context

Request handling and background event delivery have different load profiles
and need to scale separately, and migrations must run once per release
rather than on every instance start. Splitting the code base to achieve that
would give up the benefits of a monolith. Building a different artifact per
environment makes "works in staging" meaningless.

## Decision

We will build one container image containing four binaries from the same
code: `server` (everything in one process), `api`, `worker` and `migrate`.
Every process contains every module; a role only switches inbound adapters
on. The same image is promoted through every environment by digest, and all
differences between environments are environment variables.

## Consequences

- A small deployment runs `server`; a larger one runs `migrate` as a release
  step and scales `api` and `worker` independently, with no code change.
- What reaches production is byte-for-byte what was tested.
- Processes cannot host a subset of modules: the outbox is one queue, so a
  worker without some subscribers would mark their events delivered. Running
  modules in separate processes means extracting them (see ARCHITECTURE.md).
- Configuration mistakes are the remaining per-environment risk, so the
  committed settings files are validated by a test.
