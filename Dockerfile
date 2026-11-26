# syntax=docker/dockerfile:1

# One image holds every binary (server, api, worker, migrate) and is promoted
# unchanged through every environment. Pick the process with the entrypoint.

FROM golang:1.27 AS build
WORKDIR /src

# Dependencies first: this layer is rebuilt only when go.mod / go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags="-s -w -X $(go list -m)/internal/platform/buildinfo.version=${VERSION}" \
      -o /out/ ./cmd/...

# Distroless: no shell, no package manager, runs as an unprivileged user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /app/
EXPOSE 8080
ENTRYPOINT ["/app/server"]
