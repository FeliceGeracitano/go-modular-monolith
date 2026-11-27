# go-modular-monolith

> **Monolith simplicity, microservice boundaries** — flat, idiomatic Go with compiler-enforced module isolation and zero data-layer lock-in.

[![CI](https://github.com/FeliceGeracitano/go-modular-monolith/actions/workflows/ci.yml/badge.svg)](https://github.com/FeliceGeracitano/go-modular-monolith/actions/workflows/ci.yml)
![Go 1.27+](https://img.shields.io/badge/Go-1.27%2B-00ADD8?logo=go&logoColor=white)
![Stdlib First](https://img.shields.io/badge/HTTP-net%2Fhttp-0f172a)
![OpenTelemetry](https://img.shields.io/badge/OpenTelemetry-OTLP-F5A800?logo=opentelemetry&logoColor=white)
![Zero Infra](https://img.shields.io/badge/DB-bring%20your%20own-10b981)

**Contents:** [Architecture](#architecture) · [Highlights](#highlights) · [Quick Start](#quick-start) · [Start a New Service](#start-a-new-service) · [Sharing Between Modules](#sharing-between-modules) · [Docs](#docs) · [Inspiration & References](#inspiration--references)

---

## Architecture

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/architecture-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="docs/assets/architecture-light.svg">
  <img alt="go-modular-monolith architecture overview" src="docs/assets/architecture-light.svg" width="100%">
</picture>

```text
main.go         entry point
app/            wires pkg and modules together (hand-wired DI)
modules/        one folder per domain (<name>api/ public contract + internal/ private code)
pkg/            reusable non-domain packages (apperr, clock, config, eventbus, httpx, id, outbox, telemetry)
test/           e2e/ (full-app tests) and arch/ (AST dependency & cycle rules)
```

---

## Highlights

- **Hard module boundaries** — each domain exposes only `<name>api`; private code lives under `modules/<name>/internal/` (enforced by the Go compiler).
- **Executable architecture rules** — `test/arch` checks imports, `module.json` declarations, acyclic dependencies, and `CODEOWNERS` in CI.
- **Sync + async examples included** — ships with runnable example modules showing sync contract queries and async outbox fan-out (remove them anytime with `make remove-examples`).
- **Bring your own data layer** — no database, ORM, or migrations baked in; modules use in-memory stores behind repository ports so `make run` and `make test` need Go only.
- **Stdlib-first & observable** — `net/http` + OpenAPI 3.1 (`/docs`) + RFC 9457 errors, `log/slog`, and OpenTelemetry OTLP traces spanning HTTP → outbox → subscriber.

---

## Quick Start

Requires **Go 1.27+**.

```bash
make run    # HTTP API + outbox relay on :8080
make test   # unit, e2e, and architecture tests (-race)
```

Once running, open **`http://localhost:8080/docs`** for the interactive OpenAPI UI (or fetch `http://localhost:8080/openapi.yaml`) to explore and try the endpoints locally.

Run `make` to see all targets (including `up-observability` for local Grafana traces on `:3000`).

---

## Start a New Service

Create a repository from this template, then rename the Go module and remove the example domains:

```bash
scripts/rename-module.sh github.com/your-org/your-service
make remove-examples
```

Add your first domain under `modules/<name>/`, wire it in [app/modules.go](app/modules.go), and add its owner line to [.github/CODEOWNERS](.github/CODEOWNERS).

---

## Sharing Between Modules

> *"A little copying is better than a little dependency."* — Go Proverbs

1. **Domain data owned by a module** → expose a public contract in `modules/<name>/<name>api/` (e.g. `modules/billing/billingapi/`), declare it in `"uses"` in `module.json`, wire in `app/modules.go`. Break two-way needs with an async event.
2. **Small type/helper in 2 modules** → copy it first so each domain stays free to evolve.
3. **Stable, business-agnostic primitive in 3+ modules** → promote to a single-purpose `pkg/<name>` (never `utils` or `common`).

---

## Docs

- [ARCHITECTURE.md](ARCHITECTURE.md) — rules, patterns, and how to add a database or extract a service
- [docs/openapi.yaml](docs/openapi.yaml) — OpenAPI 3.1 spec (interactive UI served at `GET /docs`)
- [deploy/README.md](deploy/README.md) — environments and CI/CD pipeline

---

## Inspiration & References

- [Organizing a Go module](https://go.dev/doc/modules/layout) — official Go layout guidance (`internal/`, starting flat)
- [Go Proverbs](https://go-proverbs.github.io/) — Rob Pike (*"A little copying is better than a little dependency"*)
- [Standard Package Layout](https://www.gobeyond.dev/standard-package-layout/) — Ben Johnson (explicit wiring, narrow ports, domain-first design)
- [How I write HTTP services in Go after 13 years](https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/) — Mat Ryer (stdlib `net/http`, composition root, testable handlers)
- [MonolithFirst](https://martinfowler.com/bliki/MonolithFirst.html) — Martin Fowler (build clean module seams before extracting services)
- [RFC 9457: Problem Details for HTTP APIs](https://www.rfc-editor.org/rfc/rfc9457) — structured error responses
