# go-modular-monolith

A starting point for Go services built as a **modular monolith**.

Create a new repository from it and you begin with the parts every service
needs already in place and already following good practice, so the only
thing left to write is your domain: one module per business area.

## What is included

- **Modular structure** with enforced boundaries: private internals, a small
  public contract and a dedicated database schema per module.
- **Go project basics**: configuration, structured logging, error handling,
  health probes, graceful shutdown.
- **Tracing and metrics** with OpenTelemetry.
- **Multiple environments** from one image: local, dev, staging, production.
- **Testing** at every level, including architecture rules checked as tests.
- **CI/CD**: lint, tests, vulnerability scan, container image, staged deploys.

## Start a new service

Create a repository from this template, then:

```bash
scripts/rename-module.sh github.com/your-org/your-service
```

```bash
make remove-examples
```

```bash
make module name=billing
```

The first command renames the Go module, the second removes the example
modules, the third creates and wires your first module.

## Run it

Requires Go 1.27+ and Docker.

```bash
make up
```

```bash
make run
```

The service listens on `:8080`. Run `make` to list every command.

## Learn more

| Document | What it covers |
| --- | --- |
| [ARCHITECTURE.md](ARCHITECTURE.md) | The rules, the reasoning and step-by-step recipes |
| [docs/MODULES.md](docs/MODULES.md) | The current modules and how they relate (generated) |
| [docs/adr](docs/adr/README.md) | Why the main decisions were made |
| [deploy/README.md](deploy/README.md) | Environments, configuration and the delivery pipeline |
| [.env.example](.env.example) | Every setting and its default |
