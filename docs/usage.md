# Usage

## Web UI

Open the UI at <http://localhost:3000> (or the host port you mapped to the container's port 80).

## API

When accessed through the combined container, nginx proxies the `/api/` prefix to the internal API process (stripping the prefix). You can also reach the API directly on port 8080 without the prefix.

Example endpoints (shown with the nginx `/api/` prefix as used from the browser):

- `GET /api/healthz` — liveness
- `GET /api/readyz` — readiness (checks Postgres and key-value store)
- `GET /api/version` — build version and commit SHA
- `GET /api/covers/{album_id}` — album cover image
- `GET /api/stream/{track_id}` — audio stream for a track (requires JWT)
- `GET /api/stream/{track_id}/index.m3u8` — HLS manifest for a track (requires JWT)
- `GET /api/stream/audiobook/{id}` — full audiobook stream (requires JWT)
- `POST /api/admin/ingest/scan` — trigger a library scan (requires admin JWT)
- `GET /api/admin/ingest/status` — ingest status (requires admin JWT)

When accessing the API directly on port 8080, omit the `/api/` prefix.

## DLNA / Discovery

The server exposes DLNA on port `9090` and uses SSDP on UDP `1900` for discovery.
