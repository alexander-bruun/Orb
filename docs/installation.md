# Installation

## Docker Compose (recommended)

Orb requires a database and a key-value service; the easiest way to run everything is via the repository's compose file which starts Postgres, Valkey and the unified Orb container:

```bash
make docker-up
```

Notes:

- The compose file (`compose.yaml`) builds the unified image and starts Postgres and Valkey alongside the Orb service.
- The web UI is served by nginx inside the container on port 80, exposed to the host on port **3000** by default.
- The API process listens on port **8080** and is also exposed directly on the host.

| Host port | Service               |
| --------- | --------------------- |
| 3000      | Web UI (nginx)        |
| 8080      | API (direct)          |
