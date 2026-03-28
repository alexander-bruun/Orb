# Installation

## Docker Compose (recommended)

Orb requires a database and a key-value service; the easiest way to run everything is via the repository's compose file which starts Postgres, Valkey and the unified Orb container:

```bash
docker compose up -d --build
```

Notes:

- The compose file (`compose.yaml`) builds the unified image and starts Postgres and Valkey alongside the Orb service.
- The web UI is served by nginx inside the container on port 80, exposed to the host on port **3000** by default.
- The API process listens on port **8080** and is also exposed directly on the host.

| Host port | Service               |
| --------- | --------------------- |
| 3000      | Web UI (nginx)        |
| 8080      | API (direct)          |

## Local development with Compose

For a single-machine local setup (Postgres + Valkey + API+UI), use the included compose file:

```bash
docker compose -f compose.yaml up --build
```

This composes Postgres, Valkey, and the unified Orb container.

## From source (advanced)

- The project builds a static Go `api` binary and a web UI build. Use `make` targets provided in the repository (see `Makefile`).
- Building locally requires a Go toolchain compatible with the `go` version in `services/go.mod` and [Bun](https://bun.sh) for the UI.
