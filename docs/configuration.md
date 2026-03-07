# Configuration

Orb is configured via environment variables. Important options:

- `DATABASE_URL` — Postgres DSN (default: postgres://orb:orb@postgres:5432/orb?sslmode=disable)
- `KV_MODE` — key-value mode: `standalone` or `sentinel` (default: `standalone`)
- `KV_ADDR` / `KV_SENTINEL_ADDRS` — address of Valkey/Redis or sentinel addresses
- `STORE_ROOT` — local object-store root (default: `/data/audio`)
- `JWT_SECRET` — secret for signing JWTs (change in production)
- `HTTP_PORT` — port API listens on (default: `8080`)
- `INGEST_DIRS` — comma-separated read-only paths to music folders mounted inside container (e.g. `/music/drive-1`)
- `INGEST_WATCH` — enable background ingest watcher (`true`/`false`)
- `DLNA_ENABLED` — enable DLNA server (`true`/`false`)
- `DLNA_PORT` — DLNA HTTP port (default: `9090`)
- `DLNA_IP` — advertise this IP for SSDP LOCATION header (optional)

Set variables in your compose file's `environment` section.
