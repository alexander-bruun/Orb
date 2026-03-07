# Usage

## Web UI

Open the UI at <http://localhost:3000> (or the host port you mapped to the container's port 80).

## API

The public API is available under the `/api/` prefix. Example endpoints:

- `GET /api/healthz` — liveness
- `GET /api/readyz` — readiness (checks Postgres and key-value store)
- `GET /api/covers/{album_id}` — album cover image
- `GET /api/stream/{track_id}` — HLS stream for a track (requires JWT)

When using the combined container, nginx proxies `/api/` to the local API process.

## DLNA / Discovery

The server exposes DLNA on port `9090` and uses SSDP on UDP `1900` for discovery.