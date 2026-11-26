# 0004. Publish integration events through a transactional outbox

- Status: Accepted
- Date: 2026-11-26

## Context

When a module changes state and announces it, the two must agree. Publishing
before the commit can announce something that never happened; publishing
after it can lose the event in a crash. Either way other modules end up
permanently inconsistent.

## Decision

Modules publish integration events by inserting them into `outbox.messages`
inside the same database transaction as the state change. A relay delivers
committed events to subscribers afterwards, with retries and backoff.
Publishing outside a transaction is rejected at runtime.

## Consequences

- An event is delivered if and only if its transaction committed.
- Delivery is at least once and asynchronous, so every subscriber must be
  idempotent and tolerate lag and reordering.
- The in-process bus sits behind one interface; moving to a message broker
  later changes the relay, not the modules.
- The outbox table must be monitored: messages that exhaust their attempts
  stay there for a human.
- Rejected: direct in-process calls for side effects (couples modules and
  their failure modes) and a broker from day one (operational cost without a
  second consumer process).
