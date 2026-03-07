# Installation

## Docker Compose (recommended)

Orb requires a database and a key-value service; the easiest way to run everything is via the repository's compose file which starts Postgres, Valkey and the unified Orb container:

```bash
docker compose up -d --build
```

Notes:

- The compose file will build the unified image and start Postgres and Valkey alongside the Orb service.
- The web UI is served by the container and is reachable on the host port you map to container port 80 (the compose file maps it to `3000` by default).

## Local development with Compose

For a single-machine local setup (Postgres + Valkey + API+UI), use the included compose file:

```bash
docker compose -f docker-compose.yml up --build
```

This composes Postgres, Valkey, and the unified Orb container.

## From source (advanced)

- The project builds a static Go `api` binary and a web UI build. Use `make` targets provided in the repository (see `Makefile`).
- Building locally requires a Go toolchain compatible with the `go` version in `services/go.mod` and Node for the UI.
