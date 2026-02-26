# Orb: Self-Hosted Lossless Music Streaming: Design Document

> Version 2.0 | Status: Draft

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Streaming Protocol](#streaming-protocol)
4. [Service Definitions](#service-definitions)
5. [Data Model (Postgres)](#data-model-postgres)
6. [File Storage](#file-storage)
7. [Library Ingestion Pipeline](#library-ingestion-pipeline)
8. [API Reference](#api-reference)
9. [Frontend](#frontend)
10. [KeyVal Layer](#keyval-layer)
11. [HA & Horizontal Scaling](#ha--horizontal-scaling)
12. [Repository Layout](#repository-layout)
13. [Local Development](#local-development)
14. [Docker & Bundling](#docker--bundling)
15. [Task List](#task-list)
16. [Risks & Mitigations](#risks--mitigations)

---

## Overview

Orb is a self-hosted, lossless music streaming platform — a personal Spotify backed by your own audio library. It streams FLAC, WAV, and other lossless formats at full fidelity (up to 32-bit / 192kHz) to a browser client using HTTP range requests and client-side WASM decoding. Multiple users each have their own library, playlists, and queue.

### What Orb is

- A media server that indexes and serves your local audio files
- A multi-user platform: each account has its own library, playlists, and playback queue
- A high-fidelity streaming client that decodes 24-bit FLAC in the browser at full depth
- A self-hosted, fully open alternative to Spotify / Tidal / Navidrome

### What Orb is not

- A transcoding service (no quality reduction, ever)
- A listen-together or social playback application
- A podcast or video platform
- A cloud storage service (it serves files you already have on disk)

### Goals

- Stream lossless FLAC/WAV at full bit depth (up to 32-bit / 192kHz) to the browser
- Multi-user: accounts, individual libraries, playlists, persistent queue per user
- Library ingestion: scan local directories, extract ID3/Vorbis tags, embed album art
- Horizontally scalable: N API replicas, shared Postgres + object storage
- Clean SvelteKit UI with Melt UI + Tailwind: top bar, left sidebar, bottom media bar, album art

---

## Architecture

```text
 Browser (SvelteKit)
┌─────────────────────────────────────────────────────────────┐ 
│  Web Audio API + libflac.js WASM decoder                    │ 
│  HTTP range requests for audio chunks                       │ 
│  REST API calls for library, search, playlists, queue       │ 
└───────────────────┬─────────────────────────────────────────┘ 
                    │ HTTPS                                     
         ┌──────────▼───────────┐                               
         │    Load Balancer     │                               
         └──────────┬───────────┘                               
                    │                                           
       ┌────────────▼──────────────┐                            
       │      API Service          │  ← stateless, N replicas   
       │    (Go, net/http)         │                            
       │                           │                            
       │  /auth     user auth      │                            
       │  /library  browse         │                            
       │  /stream   range serve    │                            
       │  /playlists               │                            
       │  /queue    playback state │    ┌────────┐              
       └──┬───────────────┬────────┘    │        │              
          │  ┌────────────┼─────────────┤ Ingest │              
    ┌─────▼──▼───┐  ┌─────▼──────────┐  │        │              
    │  Postgres  │  │  Object Store  │  └────┬───┘              
    │ (pgBouncer │  │ (MinIO / local ├───────┘                  
    │ + replica) │  │   disk)        │                          
    └────────────┘  └────────────────┘                          
          │                                                     
    ┌─────▼───────┐                                             
    │   KeyVal    │                                             
    │  (Valkey)   │                                             
    │  sessions   │                                             
    │  + cache    │                                             
    └────────────-┘                                             
```

### Key decisions

**Single API service, not microservices.** The original architecture split signaling and SFU into separate services because they had fundamentally different responsibilities and scaling characteristics. A music streaming API does not have that problem — library browsing, auth, and file serving all scale the same way (stateless HTTP, shared DB). One service with clean internal packages is the right call. The HA story (N replicas behind a load balancer) is identical.

**HTTP range requests for streaming.** The API serves audio files with `Accept-Ranges: bytes` and `Content-Range` headers. The browser fetches chunks on demand. No WebSockets, no custom protocol, no server-side transcoding. Seeking is free — the client just requests a different byte range. This is how every serious lossless web player works.

**Client-side FLAC decoding via WASM.** Browsers cannot natively decode 24-bit FLAC at full depth through the `<audio>` element. The SvelteKit client uses `libflac.js` (Emscripten-compiled libFLAC) to decode chunks into 32-bit float PCM, fed directly into the Web Audio API. This preserves full bit depth with zero quality loss. 16-bit FLAC and MP3 fall back to native `<audio>` decoding.

**Object storage for audio files.** Audio files are stored in an S3-compatible object store (MinIO for self-hosted). The API never loads full files into memory — it proxies range requests through to the object store, or serves directly from the filesystem for local-disk deployments. Metadata, album art, and the library index live in Postgres.

---

## Streaming Protocol

### Why HTTP range requests

| Protocol | 24-bit FLAC | Seeking | Transcoding required | Complexity |
| --- | --- | --- | --- | --- |
| HTTP range requests | ✅ Full fidelity | ✅ Free (byte offset) | ❌ None | Low |
| HLS | ❌ Lossy (AAC) | ✅ | ✅ ffmpeg required | High |
| WebSockets + PCM | ✅ | ⚠️ Complex | ❌ | High |
| Native `<audio>` FLAC | ⚠️ 16-bit only | ✅ | ❌ | None |

HTTP range requests win on every axis that matters for lossless: no transcoding, no quality loss, trivial seeking, standard HTTP caching, works behind any CDN or reverse proxy.

### Server-side implementation

The API exposes `GET /stream/:track_id` with full range request support:

```text
Client:  GET /stream/abc123
         Range: bytes=0-262143

Server:  HTTP/1.1 206 Partial Content
         Content-Type: audio/flac
         Content-Range: bytes 0-262143/48302156
         Accept-Ranges: bytes
         Content-Length: 262144
         Cache-Control: private, max-age=3600
         X-Orb-Bit-Depth: 24
         X-Orb-Sample-Rate: 96000
```

The handler resolves the file location from Postgres, opens the file or proxies to object storage, seeks to the byte offset, copies exactly the requested bytes, and closes. It never buffers the full file in memory. The custom `X-Orb-*` headers tell the client which decoding path to use (WASM vs native).

### Client-side decoding pipeline

```text
HTTP range request (256KB chunks)
        │
        ▼
  libflac.js WASM decoder
  (decodes FLAC frames to PCM)
        │
        ▼
  AudioBuffer (32-bit float, Web Audio API)
        │
        ▼
  AudioBufferSourceNode → AudioContext destination
```

The client maintains a ring buffer of decoded PCM, pre-fetching the next chunk before the current one finishes. Seeking triggers a new range request at the byte offset corresponding to the target timestamp (derivable from a pre-computed seek table stored in Postgres during ingest).

For MP3 and 16-bit FLAC, the client uses a native `<audio>` element — no WASM needed. The WASM path activates only for 24-bit+ FLAC and WAV.

---

## Service Definitions

### `services/api`

The single backend service. Stateless — all state in Postgres, KeyVal, and object storage. Scale by running N replicas behind a load balancer.

Internal packages:

```text
services/api/internal/
├── auth/        # JWT issuance + validation, sessions in KeyVal
├── library/     # Browse artists, albums, tracks; full-text search
├── stream/      # HTTP range request handler, object store proxy
├── playlist/    # CRUD for playlists and playlist tracks
├── queue/       # Per-user playback queue, write-through cache
└── user/        # Account management
```

### `pkg/store`

### `pkg/objstore`

Abstraction over storage backends:

```go
type ObjectStore interface {
    Put(ctx context.Context, key string, r io.Reader, size int64) error
    GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error)
    Delete(ctx context.Context, key string) error
    Exists(ctx context.Context, key string) (bool, error)
}
```

Implementations: `LocalFS` (direct disk reads) and `S3` (MinIO or AWS S3). The stream handler calls `GetRange` and never knows which backend is active.

### `cmd/ingest`

Standalone CLI that scans a directory tree, extracts metadata, stores audio files in the object store, and writes track/album/artist records to Postgres. Idempotent — safe to re-run after adding new files.

---

## Data Model (Postgres)

### Schema

```sql
-- db/schema.sql — Atlas source of truth

CREATE TABLE users (
    id            TEXT        PRIMARY KEY,
    username      TEXT        NOT NULL UNIQUE,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

CREATE TABLE artists (
    id            TEXT        PRIMARY KEY,
    name          TEXT        NOT NULL,
    sort_name     TEXT        NOT NULL,
    mbid          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE albums (
    id            TEXT        PRIMARY KEY,
    artist_id     TEXT        REFERENCES artists(id) ON DELETE SET NULL,
    title         TEXT        NOT NULL,
    release_year  INT,
    label         TEXT,
    cover_art_key TEXT,
    mbid          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tracks (
    id            TEXT        PRIMARY KEY,
    album_id      TEXT        REFERENCES albums(id) ON DELETE SET NULL,
    artist_id     TEXT        REFERENCES artists(id) ON DELETE SET NULL,
    title         TEXT        NOT NULL,
    track_number  INT,
    disc_number   INT         NOT NULL DEFAULT 1,
    duration_ms   INT         NOT NULL,
    file_key      TEXT        NOT NULL,
    file_size     BIGINT      NOT NULL,
    format        TEXT        NOT NULL,    -- flac | wav | mp3
    bit_depth     INT,                    -- 16 | 24 | 32 (NULL for MP3)
    sample_rate   INT         NOT NULL,
    channels      INT         NOT NULL DEFAULT 2,
    bitrate_kbps  INT,
    seek_table    JSONB,                  -- precomputed frame offsets for seeking
    fingerprint   TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Per-user library ownership
CREATE TABLE user_library (
    user_id       TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id      TEXT        NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, track_id)
);

CREATE TABLE playlists (
    id            TEXT        PRIMARY KEY,
    user_id       TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT        NOT NULL,
    description   TEXT,
    cover_art_key TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE playlist_tracks (
    playlist_id   TEXT        NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    track_id      TEXT        NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position      INT         NOT NULL,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (playlist_id, track_id)
);

CREATE TABLE queue_entries (
    id            BIGSERIAL   PRIMARY KEY,
    user_id       TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id      TEXT        NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position      INT         NOT NULL,
    source        TEXT,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE play_history (
    id                BIGSERIAL   PRIMARY KEY,
    user_id           TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id          TEXT        NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    played_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration_played_ms INT        NOT NULL
);

CREATE INDEX tracks_album_id_idx       ON tracks(album_id);
CREATE INDEX tracks_artist_id_idx      ON tracks(artist_id);
CREATE INDEX albums_artist_id_idx      ON albums(artist_id);
CREATE INDEX user_library_user_id_idx  ON user_library(user_id);
CREATE INDEX playlist_tracks_pl_idx    ON playlist_tracks(playlist_id, position);
CREATE INDEX queue_entries_user_idx    ON queue_entries(user_id, position);
CREATE INDEX play_history_user_idx     ON play_history(user_id, played_at DESC);

-- Full-text search
ALTER TABLE tracks  ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', coalesce(title, ''))) STORED;
ALTER TABLE albums  ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', coalesce(title, ''))) STORED;
ALTER TABLE artists ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', coalesce(name, ''))) STORED;

CREATE INDEX tracks_search_idx  ON tracks  USING GIN(search_vector);
CREATE INDEX albums_search_idx  ON albums  USING GIN(search_vector);
CREATE INDEX artists_search_idx ON artists USING GIN(search_vector);
```

### Atlas migration workflow

```bash
# Edit db/schema.sql, then:
make migrate-diff    # prompts for name, generates versioned migration file
make migrate-up      # applies pending migrations
make migrate-down    # rolls back one migration
make migrate-status  # shows applied vs pending
```

Never write migration SQL by hand. Always edit `schema.sql` and let Atlas generate the diff.

---

## File Storage

### Backends

**`LocalFS`** — audio files on a mounted disk. Suitable for single-server deployments. Range requests served via `os.File.ReadAt`. Not horizontally scalable without a shared network volume.

**`S3`** — audio files in MinIO (self-hosted) or AWS S3. All API replicas read from the same bucket. Range requests proxied via `GetObject` with the `Range` header forwarded.

### Cover art

Extracted during ingest from embedded tags or folder-level `cover.jpg`. Stored at `covers/{album_id}.jpg` in the object store. Served at `GET /covers/:album_id` with `Cache-Control: public, max-age=86400`.

### Object key structure

```text
audio/
  {artist_id}/{album_id}/{track_id}.flac
  {artist_id}/{album_id}/{track_id}.mp3

covers/
  {album_id}.jpg
  playlist/{playlist_id}.jpg
```

---

## Library Ingestion Pipeline

`cmd/ingest` scans a directory, extracts metadata, and populates the database. Idempotent — re-running only processes new or changed files.

### Stages

```text
Directory scan → filter audio files (.flac .wav .mp3 .aiff)
      ↓
Extract metadata (github.com/dhowden/tag — pure Go, no cgo)
  title, artist, album, track number, disc, year
  bit depth, sample rate, channels, duration
  embedded cover art
      ↓
Parse seek table from FLAC frame headers (for accurate seeking)
      ↓
Upsert artist → album → track in Postgres
      ↓
Copy audio file to object store (skip if already present by hash)
      ↓
Extract + normalize cover art → object store
```

### CLI flags

```bash
orb-ingest \
  --dir /music \
  --db $DATABASE_URL \
  --store-backend local \      # local | s3
  --store-root /data/audio \
  --user-id <uuid> \           # assign tracks to this user's library
  --recursive \
  --dry-run
```

---

## API Reference

All endpoints require a `Authorization: Bearer <jwt>` header except `/auth/*`.

### Auth

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/auth/register` | Create account |
| `POST` | `/auth/login` | Returns JWT + refresh token |
| `POST` | `/auth/refresh` | Exchange refresh token for new JWT |
| `POST` | `/auth/logout` | Revoke session in KeyVal |

### Library

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/library/tracks` | Paginated user track list |
| `GET` | `/library/albums` | Paginated album list |
| `GET` | `/library/artists` | Paginated artist list |
| `GET` | `/library/albums/:id` | Album detail + tracks |
| `GET` | `/library/artists/:id` | Artist detail + albums |
| `GET` | `/library/tracks/:id` | Track detail incl. audio properties |
| `POST` | `/library/tracks/:id` | Add track to user library |
| `DELETE` | `/library/tracks/:id` | Remove track from library |
| `GET` | `/library/search?q=` | Full-text search: tracks, albums, artists |
| `GET` | `/library/recently-played` | Recent play history |

### Streaming

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/stream/:track_id` | Stream audio with range request support |
| `GET` | `/covers/:album_id` | Album cover art (JPEG) |
| `GET` | `/covers/playlist/:id` | Playlist cover art |

### Playlists

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/playlists` | List user playlists |
| `POST` | `/playlists` | Create playlist |
| `GET` | `/playlists/:id` | Playlist detail + tracks |
| `PATCH` | `/playlists/:id` | Update name / description |
| `DELETE` | `/playlists/:id` | Delete playlist |
| `POST` | `/playlists/:id/tracks` | Add track |
| `DELETE` | `/playlists/:id/tracks/:track_id` | Remove track |
| `PUT` | `/playlists/:id/tracks/order` | Reorder tracks |

### Queue

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/queue` | Get current queue |
| `PUT` | `/queue` | Replace queue |
| `DELETE` | `/queue` | Clear queue |
| `POST` | `/queue/next` | Insert track to play next |
| `POST` | `/queue/last` | Append track to end |

### Health

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/healthz` | Liveness |
| `GET` | `/readyz` | Readiness (Postgres + KeyVal reachable) |

---

## Frontend

### Stack

| Layer | Choice | Rationale |
| --- | --- | --- |
| Framework | **SvelteKit** | Compiles to vanilla JS, excellent reactivity for audio state |
| UI Primitives | **Melt UI** | Headless, accessible, Radix-equivalent for Svelte |
| Styling | **Tailwind CSS v4** | Utility-first, pairs with Melt UI unstyled primitives |
| Audio decoding | **libflac.js** (WASM) | Full 24-bit FLAC decoding in browser, zero quality loss |
| HTTP client | Browser `fetch` | Range requests via `headers: { Range: 'bytes=...' }` |
| Build | **Vite** (via SvelteKit) | Fast HMR, native ESM |

### UI Layout

```text
┌──────────────────────────────────────────────────────────────┐
│  Top Bar                                                     │
│  [orb]        [🔍 Search...]           [User ▾]  [⚙]        │
├──────────────┬───────────────────────────────────────────────┤
│              │                                               │
│  Sidebar     │                                               │
│              │           Content Area                        │
│  Library     │                                               │
│  Playlists   │   (Album grid / Artist / Playlist /           │
│  Recently    │    Search results / Track list)               │
│  Played      │                                               │
│              │                                               │
│  ──────────  │                                               │
│              │                                               │
│  [Album Art] │                                               │
│              │                                               │
├──────────────┴───────────────────────────────────────────────┤
│  Bottom Bar                                                  │
│  [◀◀] [▶] [▶▶]   ━━━━━●━━━━━━━   🔊 ━━●━━   24bit · 96kHz │
└──────────────────────────────────────────────────────────────┘
```

**Top bar** (~56px): Orb wordmark, global search (Melt UI Combobox), user avatar menu (Melt UI DropdownMenu), settings icon.

**Left sidebar** (~240px): Navigation — Library, Playlists, Recently Played. Pinned to the bottom-left: album art for the currently playing track, track title, artist name. Clicking the album art opens a full now-playing detail overlay.

**Content area**: Fluid. Renders the current route — album grid, artist detail, playlist, search results.

**Bottom bar** (~72px): Previous / play-pause / next. Seek bar (Melt UI Slider). Volume (Melt UI Slider). Format badge showing current bit depth and sample rate (`24bit · 96kHz`).

### Component structure

```text
web/ui/src/
├── lib/
│   ├── components/
│   │   ├── layout/
│   │   │   ├── TopBar.svelte
│   │   │   ├── Sidebar.svelte
│   │   │   ├── BottomBar.svelte
│   │   │   └── NowPlayingExpanded.svelte
│   │   ├── media/
│   │   │   ├── AlbumArt.svelte          # sidebar bottom-left
│   │   │   ├── SeekBar.svelte           # Melt UI Slider
│   │   │   ├── VolumeControl.svelte     # Melt UI Slider
│   │   │   ├── PlaybackControls.svelte
│   │   │   └── FormatBadge.svelte       # "24bit · 96kHz"
│   │   ├── library/
│   │   │   ├── AlbumGrid.svelte
│   │   │   ├── AlbumCard.svelte
│   │   │   ├── TrackList.svelte
│   │   │   └── TrackRow.svelte
│   │   └── playlist/
│   │       ├── PlaylistCard.svelte
│   │       └── PlaylistHeader.svelte
│   ├── stores/
│   │   ├── player.ts     # playback state, queue, position
│   │   ├── auth.ts       # JWT, user session
│   │   └── library.ts    # browse state, search results
│   ├── audio/
│   │   ├── engine.ts     # unified audio interface
│   │   ├── flac-decoder.ts  # libflac.js WASM wrapper → Web Audio API
│   │   ├── streamer.ts   # HTTP range requests + chunk ring buffer
│   │   └── native.ts     # fallback <audio> for MP3 / 16-bit FLAC
│   └── api/
│       ├── client.ts     # fetch wrapper, JWT injection, error handling
│       ├── library.ts
│       ├── playlists.ts
│       └── queue.ts
└── routes/
    ├── +layout.svelte    # App shell
    ├── +page.svelte      # Home: recently played
    ├── login/+page.svelte
    ├── library/+page.svelte
    ├── library/albums/[id]/+page.svelte
    ├── artists/[id]/+page.svelte
    ├── playlists/+page.svelte
    ├── playlists/[id]/+page.svelte
    └── search/+page.svelte
```

### Audio engine

The audio engine is framework-agnostic — no Svelte imports. The `player` store calls into it; components only read from the store.

```typescript
// src/lib/audio/engine.ts
export class AudioEngine {
    private ctx: AudioContext;
    private gainNode: GainNode;

    async play(trackId: string, bitDepth: number, sampleRate: number): Promise<void> {
        if (bitDepth > 16) {
            // WASM path: libflac.js → Web Audio API
        } else {
            // Native <audio> fallback
        }
    }

    seek(positionSeconds: number): void { /* issues new range request at byte offset */ }
    setVolume(gain: number): void { this.gainNode.gain.value = gain; }
}
```

### Player store

```typescript
// src/lib/stores/player.ts
import { writable, derived } from 'svelte/store';

export const currentTrack  = writable<Track | null>(null);
export const playbackState = writable<'idle'|'loading'|'playing'|'paused'>('idle');
export const positionMs    = writable(0);
export const queue         = writable<Track[]>([]);

export const formattedFormat = derived(currentTrack, ($t) =>
    $t ? `${$t.bitDepth}bit · ${($t.sampleRate / 1000)}kHz` : ''
);
```

---

## KeyVal Layer

Valkey (BSD-licensed, Redis-protocol-compatible) handles sessions and hot caches. Client: `go-redis/v9` with Sentinel.

### Key schema (`pkg/kvkeys`)

```go
func Session(userID string) string      { return "session:" + userID }
func RefreshToken(token string) string  { return "refresh:" + token }
func TrackMeta(trackID string) string   { return "track:meta:" + trackID }
func UserQueue(userID string) string    { return "queue:" + userID }
func LoginAttempts(ip string) string    { return "ratelimit:login:" + ip }
```

### Usage patterns

**Sessions**: write `session:{user_id}` on login with TTL = JWT expiry. Delete on logout for forced revocation. Check presence for revoked-token detection.

**Track metadata cache**: cache `{file_key, bit_depth, sample_rate}` for 1 hour per track. Avoids a Postgres round-trip on every streaming chunk request. Invalidated on ingest re-run.

**Queue cache**: full queue as JSON, write-through to Postgres. Read from KeyVal on every track-end event; write to both KeyVal and Postgres on every queue mutation.

---

## HA & Horizontal Scaling

**API**: stateless, scale by replica count. No sticky sessions.

**Object storage**: LocalFS requires a shared network volume for multi-replica. S3/MinIO is inherently scalable — all replicas share the same bucket.

**Postgres**: primary + streaming replica. pgBouncer transaction pooling (1000 client connections → 25 Postgres connections). Read-heavy queries (browse, search) routable to replica.

**KeyVal**: Valkey with 3-node Sentinel. Automatic failover, transparent to application via `go-redis/v9` `NewFailoverClient`.

**Ingest**: one-shot job, not a long-running service. Safe to run while API serves traffic. Run as a Docker one-off container or cron job.

---

## Repository Layout

```text
orb/
├── go.work
├── go.work.sum
├── Makefile
├── README.md
│
├── pkg/
│   ├── store/
│   │   ├── go.mod
│   │   ├── store.go
│   │   ├── db.go                    # Atlas generated
│   │   └── store_test.go
│   ├── objstore/                    # ObjectStore interface + LocalFS + S3
│   ├── objstore/                    # ObjectStore interface + LocalFS + S3
│   │   ├── go.mod
│   │   ├── objstore.go
│   │   ├── local.go
│   │   └── s3.go
│   └── kvkeys/
│       ├── go.mod
│       └── keys.go
│
├── services/
│   └── api/
│       ├── go.mod
│       ├── cmd/main.go
│       └── internal/
│           ├── auth/
│           ├── library/
│           ├── stream/
│           ├── playlist/
│           ├── queue/
│           └── user/
│
├── cmd/
│   └── ingest/
│       ├── go.mod
│       └── main.go
│
├── db/
│   ├── schema.sql
│   ├── atlas.hcl
│   ├── migrations/
│   └── queries/
│       ├── users.sql
│       ├── tracks.sql
│   │   ├── db.go                    # Atlas generated
│       ├── artists.sql
│       ├── playlists.sql
│       ├── queue.sql
│       └── history.sql
│
├── docker/
│   ├── api.Dockerfile
│   ├── ingest.Dockerfile
│   ├── ui.Dockerfile
│   ├── nginx.conf
│   ├── postgres/
│   │   ├── primary.conf
│   │   └── replica.conf
│   └── valkey/
│       └── sentinel.conf
│
├── docker-compose.yml
├── docker-compose.dev.yml
│
├── web/ui/                          # SvelteKit + Melt UI + Tailwind
│   ├── package.json
│   ├── svelte.config.js             # adapter-static
│   ├── vite.config.js
│   ├── tailwind.config.js
│   └── src/
│       ├── lib/
│       │   ├── audio/
│       │   ├── components/
│       │   ├── stores/
│       │   └── api/
│       └── routes/
│
└── scripts/
    ├── ingest_local.sh
    └── load_test.sh
```

---

## Local Development

### Prerequisites

```bash
# Go 1.26+      https://go.dev/dl/
# Node 22+      https://nodejs.org/
# Docker + Compose v2

curl -sSf https://atlasgo.sh | sh
```

### Environment variables

| Variable | Default (local) | Used by |
| --- | --- | --- |
| `DATABASE_URL` | `postgres://orb:orb@localhost:5432/orb?sslmode=disable` | API, ingest |
| `KV_SENTINEL_ADDRS` | `localhost:26379` | API |
| `KV_SENTINEL_MASTER` | `mymaster` | API |
| `STORE_BACKEND` | `local` | API, ingest |
| `STORE_ROOT` | `./data/audio` | API, ingest (local) |
| `STORE_BUCKET` | `orb-audio` | API, ingest (s3) |
| `S3_ENDPOINT` | `http://localhost:9000` | API, ingest (s3) |
| `JWT_SECRET` | `dev-secret-change-in-prod` | API |
| `HTTP_PORT` | `8080` | API |

### Option A — Native services, infrastructure in Docker

```bash
make dev-db                                    # start Postgres, Valkey, MinIO
make migrate-up                                # apply schema

### Database schema & migrations

**Migration workflow:**
- Edit `db/schema.sql` (single authoritative schema file)
- Run `make migrate-diff` to generate a new migration (Atlas auto-generates versioned migration files)
- Run `make migrate-up` to apply pending migrations
- Run `make migrate-status` to view applied/pending migrations

**Never write migration SQL by hand.** Always edit `schema.sql` and let Atlas generate the diff.

**References:**
- [Atlas documentation](https://atlasgo.io/)
- See `README.md` for updated workflow and migration commands.
make dev-ingest DIR=/path/to/music USER_ID=x  # index a music directory
make dev-api                                   # start API server
make dev-ui                                    # start SvelteKit dev server
```

### Option B — Full Docker Compose

```bash
docker compose up --build        # start everything
docker compose down -v           # tear down + wipe data
docker compose up --build api    # rebuild one service
```

### Makefile

```makefile
DATABASE_URL       ?= postgres://orb:orb@localhost:5432/orb?sslmode=disable
KV_SENTINEL_ADDRS  ?= localhost:26379
KV_SENTINEL_MASTER ?= mymaster
STORE_BACKEND      ?= local
STORE_ROOT         ?= ./data/audio
HTTP_PORT          ?= 8080
DIR                ?= ./music
USER_ID            ?=

.PHONY: dev-db dev-api dev-ui dev-ingest \
        migrate-up migrate-down migrate-diff migrate-status \
        generate test lint

dev-db:
 docker compose -f docker-compose.dev.yml up -d

dev-api:
 cd services/api && \
 DATABASE_URL=$(DATABASE_URL) KV_SENTINEL_ADDRS=$(KV_SENTINEL_ADDRS) \
 KV_SENTINEL_MASTER=$(KV_SENTINEL_MASTER) STORE_BACKEND=$(STORE_BACKEND) \
 STORE_ROOT=$(STORE_ROOT) JWT_SECRET=dev-secret-change-in-prod \
 HTTP_PORT=$(HTTP_PORT) go run ./cmd/main.go

dev-ui:
 cd web/ui && npm run dev

dev-ingest:
 cd cmd/ingest && \
 DATABASE_URL=$(DATABASE_URL) STORE_BACKEND=$(STORE_BACKEND) \
 STORE_ROOT=$(STORE_ROOT) \
 go run . --dir $(DIR) --user-id $(USER_ID) --recursive

migrate-up:     ; atlas migrate apply --env local
migrate-down:   ; atlas migrate down --env local --amount 1
migrate-status: ; atlas migrate status --env local
migrate-diff:
 @read -p "Migration name: " name; \
 atlas migrate diff --env local --name $$name

test:
 go test ./...

lint:
 golangci-lint run ./...
```

---

## Docker & Bundling

### api.Dockerfile

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.work go.work.sum ./
COPY pkg/ ./pkg/
COPY services/api/ ./services/api/
WORKDIR /src/services/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /bin/api ./cmd/main.go

FROM gcr.io/distroless/static-debian12
COPY --from=builder /bin/api /api
ENTRYPOINT ["/api"]
```

### ingest.Dockerfile

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.work go.work.sum ./
COPY pkg/ ./pkg/
COPY cmd/ingest/ ./cmd/ingest/
WORKDIR /src/cmd/ingest
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /bin/ingest .

FROM gcr.io/distroless/static-debian12
COPY --from=builder /bin/ingest /ingest
ENTRYPOINT ["/ingest"]
```

### ui.Dockerfile

```dockerfile
FROM node:20-alpine AS builder
WORKDIR /app
COPY web/ui/package*.json ./
RUN npm ci
COPY web/ui/ ./
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/build /usr/share/nginx/html
COPY docker/nginx.conf /etc/nginx/conf.d/default.conf
```

### docker-compose.yml (full stack)

```yaml
services:
  postgres-primary:
    image: postgres:16-alpine
    environment: { POSTGRES_USER: orb, POSTGRES_PASSWORD: orb, POSTGRES_DB: orb }
    volumes: [postgres_primary_data:/var/lib/postgresql/data]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U orb"]
      interval: 5s
      retries: 5

  postgres-replica:
    image: postgres:16-alpine
    environment: { PGUSER: replicator, PGPASSWORD: replicator, PRIMARY_HOST: postgres-primary }
    volumes: [postgres_replica_data:/var/lib/postgresql/data]
    depends_on: { postgres-primary: { condition: service_healthy } }

  pgbouncer:
    image: pgbouncer/pgbouncer:latest
    environment:
      DATABASES_HOST: postgres-primary
      POOL_MODE: transaction
      MAX_CLIENT_CONN: "1000"
      DEFAULT_POOL_SIZE: "25"
    ports: ["5432:5432"]
    depends_on: { postgres-primary: { condition: service_healthy } }

  migrate:
    image: arigaio/atlas:latest
    volumes: [./db/migrations:/migrations]
    command: migrate apply --url "postgres://orb:orb@postgres-primary:5432/orb?sslmode=disable" --dir "file:///migrations"
    depends_on: { postgres-primary: { condition: service_healthy } }
    restart: on-failure

  valkey-primary:
    image: valkey/valkey:7-alpine
    command: valkey-server --appendonly yes
    volumes: [valkey_data:/data]

  valkey-replica:
    image: valkey/valkey:7-alpine
    command: valkey-server --replicaof valkey-primary 6379
    depends_on: [valkey-primary]

  valkey-sentinel-1: &sentinel
    image: valkey/valkey:7-alpine
    command: valkey-sentinel /etc/valkey/sentinel.conf
    volumes: [./docker/valkey/sentinel.conf:/etc/valkey/sentinel.conf:ro]
  valkey-sentinel-2: *sentinel
  valkey-sentinel-3: *sentinel

  minio:
    image: minio/minio:latest
    command: server /data --console-address ":9001"
    environment: { MINIO_ROOT_USER: orb, MINIO_ROOT_PASSWORD: orbsecret }
    volumes: [minio_data:/data]
    ports: ["9000:9000", "9001:9001"]

  api:
    build: { context: ., dockerfile: docker/api.Dockerfile }
    environment:
      DATABASE_URL: postgres://orb:orb@pgbouncer:5432/orb?sslmode=disable
      KV_SENTINEL_ADDRS: valkey-sentinel-1:26379,valkey-sentinel-2:26379,valkey-sentinel-3:26379
      KV_SENTINEL_MASTER: mymaster
      STORE_BACKEND: s3
      STORE_BUCKET: orb-audio
      S3_ENDPOINT: http://minio:9000
      S3_ACCESS_KEY: orb
      S3_SECRET_KEY: orbsecret
      JWT_SECRET: "${JWT_SECRET}"
      HTTP_PORT: "8080"
    ports: ["8080:8080"]
    depends_on: { migrate: { condition: service_completed_successfully } }
    restart: unless-stopped

  ui:
    build: { context: ., dockerfile: docker/ui.Dockerfile }
    ports: ["3000:80"]

volumes:
  postgres_primary_data:
  postgres_replica_data:
  valkey_data:
  minio_data:
```

### docker-compose.dev.yml (infrastructure only)

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment: { POSTGRES_USER: orb, POSTGRES_PASSWORD: orb, POSTGRES_DB: orb }
    ports: ["5432:5432"]
    volumes: [postgres_dev_data:/data]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U orb"]
      interval: 5s

  valkey:
    image: valkey/valkey:7-alpine
    ports: ["6379:6379"]

  valkey-sentinel:
    image: valkey/valkey:7-alpine
    command: valkey-sentinel /etc/valkey/sentinel.conf
    ports: ["26379:26379"]
    volumes: [./docker/valkey/sentinel.conf:/etc/valkey/sentinel.conf:ro]
    depends_on: [valkey]

  minio:
    image: minio/minio:latest
    command: server /data --console-address ":9001"
    environment: { MINIO_ROOT_USER: orb, MINIO_ROOT_PASSWORD: orbsecret }
    ports: ["9000:9000", "9001:9001"]
    volumes: [minio_dev_data:/data]

volumes:
  postgres_dev_data:
  minio_dev_data:
```
