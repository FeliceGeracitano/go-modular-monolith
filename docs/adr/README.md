# Architecture decision records

A decision record captures one significant decision: the situation, what was
decided, and what follows from it. Records are short, numbered and never
rewritten; a decision that changes gets a new record that supersedes the old
one. [ARCHITECTURE.md](../../ARCHITECTURE.md) describes the system as it is;
these explain why it is that way.

Write one when a choice is costly to reverse, when you picked between real
alternatives, or when a future reader is likely to ask "why on earth?". Copy
[template.md](template.md) to the next number.

| # | Decision | Status |
| --- | --- | --- |
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions | Accepted |
| [0002](0002-modular-monolith.md) | Build a modular monolith | Accepted |
| [0003](0003-schema-and-role-per-module.md) | One database schema and role per module | Accepted |
| [0004](0004-transactional-outbox.md) | Publish integration events through a transactional outbox | Accepted |
| [0005](0005-declared-module-dependencies.md) | Modules declare their dependencies | Accepted |
| [0006](0006-standard-library-first.md) | Standard library first, explicit wiring | Accepted |
| [0007](0007-one-image-process-roles.md) | One image, several process roles | Accepted |
