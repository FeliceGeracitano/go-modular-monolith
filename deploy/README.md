# Deploying

The service is built once into one container image and that same image is
promoted through every environment. Only configuration differs.

## Environments

| Environment | Deployed from | Purpose |
| --- | --- | --- |
| `local` | your machine | Development. `make run`. |
| `dev` | every push to `main` | Integration; always the latest `main`. |
| `staging` | a version tag (`v1.2.3`) | Release candidate, production-like. |
| `production` | the same tag, after staging | Live. Protect it with required reviewers. |

## Configuration

Configuration is environment variables only, validated at startup.

- **Non-secret settings** live in [`env/`](env/), one file per environment,
  reviewed like code. A test checks that every file is a valid configuration.
- **Secrets** (`OTEL_EXPORTER_OTLP_HEADERS`, API keys, future data-store
  credentials) never enter the repository. Store them in your platform's
  secret manager or in the matching GitHub environment, and inject them at
  deploy time.

## Process

The container image runs `/app/server`, which serves `/healthz` (liveness) and
`/readyz` (readiness) on `HTTP_ADDR`, and shuts down gracefully on `SIGTERM`.

## Pipeline

[`.github/workflows/cd.yml`](../.github/workflows/cd.yml) runs the checks,
publishes the image to GitHub Container Registry and then calls
[`scripts/deploy.sh`](../scripts/deploy.sh) once per environment.

The deploy step ships disabled, because where you deploy is your choice. To
turn it on:

1. Implement `scripts/deploy.sh` for your platform (Kubernetes, ECS, Cloud
   Run, Fly, a VM ...). It receives the environment name and the image.
2. Create the `dev`, `staging` and `production` environments in the
   repository settings, with their secrets and, for production, required
   reviewers.
3. Set the repository variable `DEPLOY_ENABLED` to `true`.
