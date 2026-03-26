# Configuration

Orb is configured via environment variables. Important options:

## Core

- `DATABASE_URL` — Postgres DSN (default: `postgres://orb:orb@postgres:5432/orb?sslmode=disable`)
- `KV_MODE` — key-value mode: `standalone` or `sentinel` (default: `standalone`)
- `KV_ADDR` — address of Valkey/Redis (default: `localhost:6379`; used when `KV_MODE=standalone`)
- `KV_SENTINEL_ADDRS` — comma-separated sentinel addresses (default: `localhost:26379`; used when `KV_MODE=sentinel`)
- `KV_SENTINEL_MASTER` — sentinel master name (default: `mymaster`)
- `STORE_ROOT` — local object-store root (default: `./data/audio`)
- `LOG_FILE` — path to server log file used by the admin log viewer (default: `./data/orb.log`; Orb writes logs to both stdout and this file)
- `PG_DUMP_BIN` — optional path/name for `pg_dump` used by Admin backup (default: `pg_dump`)
- `PG_RESTORE_BIN` — optional path/name for `pg_restore` used by Admin restore (default: `pg_restore`)
- `JWT_SECRET` — secret for signing JWTs (change in production)
- `HTTP_PORT` — port API listens on (default: `8080`)

## Music Ingest

- `MUSIC_DIRS` — comma-separated read-only paths to music folders mounted inside container (e.g. `/music/drive-1`)
- `INGEST_WATCH` — enable background ingest watcher (`true`/`false`)
- `INGEST_EXCLUDE` — comma-separated glob patterns to exclude from scanning
- `INGEST_SIMILARITY` — compute audio similarity via chromaprint during ingest (default: `true`)
- `INGEST_ENRICH` — fetch MusicBrainz metadata enrichment during ingest (default: `true`)
- `INGEST_WAVEFORM` — generate waveform data during ingest (default: `true`)
- `INGEST_LYRICS` — fetch lyrics during ingest (default: `true`)
- `INGEST_POLL_INTERVAL` — how often the background watcher polls for changes (default: `30s`)
- `INGEST_STABLE_TIME` — how long a file must be stable before it is ingested (default: `10s`)

## Audiobook Ingest

- `AUDIOBOOK_DIRS` — comma-separated read-only paths to audiobook folders mounted inside container (e.g. `/audiobooks/drive-1`)
- `AUDIOBOOK_EXCLUDE` — comma-separated glob patterns to exclude from scanning
- `AUDIOBOOK_ENRICH` — fetch metadata enrichment during audiobook ingest (default: `true`)
- `AUDIOBOOK_POLL_INTERVAL` — how often the audiobook watcher polls for changes (default: `30s`)
- `AUDIOBOOK_STABLE_TIME` — how long a file must be stable before it is ingested (default: `10s`)

## Discovery

- `MDNS_ENABLED` — enable mDNS server discovery (default: `true`)
- `SERVER_NAME` — display name advertised via mDNS (default: auto-detected hostname)

## DLNA / UPnP

> **Note:** DLNA support is experimental and may not work with all renderers or network setups.

- `DLNA_ENABLED` — enable DLNA server (default: `true`)
- `DLNA_PORT` — DLNA HTTP port (default: `9090`)
- `DLNA_NAME` — DLNA server display name (default: `Orb Music Server`)
- `DLNA_IP` — advertise this IP for SSDP LOCATION header (optional; auto-detected if unset)

## Chromecast

- `CAST_BASE_URL` — base URL the cast proxy advertises to cast devices (default: auto-detected LAN IP + HTTP_PORT)

Set variables in your compose file's `environment` section.
