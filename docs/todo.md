# Feature Ideas

## Playback & Discovery

- [ ] **Sleep timer** — stop playback after N minutes or at the end of the current track/album. Simple client-side timer, no backend needed.
- [ ] **"Radio" mode** — infinite queue seeded from a track or artist using the existing `recommend` service, auto-fetching more tracks as the queue drains.
- [ ] **Smart shuffle** — weight shuffle by recency, play count, or genre similarity instead of pure random.

## Library Management

- [ ] **Duplicate detector** — surface tracks that share the same MusicBrainz recording ID or very similar title+duration, let the user pick which to keep.
- [ ] **Missing artwork scanner** — admin tool to list albums without cover art and trigger a MusicBrainz re-fetch for them.
- [ ] **BPM / tempo tagging** — read `BPM` from audio file tags during ingest and expose it as a filterable field; useful for workout playlists.
- [ ] **Track ratings (1–5 stars)** — finer-grained than a binary favorite; enables weighted recommendations.

## Playlists

- [ ] **Auto-playlist (smart playlist)** — saved filter rules (genre = "Jazz", year > 2000, play count < 5) that dynamically populate a playlist on open.
- [ ] **Playlist collaboration** — allow other users on the same instance to add/remove tracks from a shared playlist (extends the existing listen party infrastructure).
- [ ] **Export/import as M3U or JSPF** — useful for migrating to/from other players.

## Social / Multi-user

- [ ] **Activity feed** — per-user "now playing" and recent plays visible to other users on the instance, opt-in.
- [ ] **"Also listened to"** — show what other users on the instance played after the current track (collaborative filtering, fully local).

## UI / UX

- [ ] **Themes / accent color picker** — `themeStore` already exists, just expose more customization.

## Ingest / Admin

- [ ] **Watch folder** — inotify-based watcher in the ingest service to auto-ingest new files dropped into the library directory, no manual re-scan needed.
- [ ] **Ingest progress WebSocket** — stream ingest job status to the admin UI in real time instead of polling.
- [ ] **Per-user upload** — let non-admin users upload their own tracks into a personal namespace.

- Make it possible to click lyrics, which should result in seeking the song to that location and playing from there.
- Rework the filter page, it's not very intuitive to have to expand the menu to search, and we should have a large search field on the page itself so we dont rely on the field in the top. The search field in the top bar, should show search results in a little modal showing the results. Not the search page itself. So they are basically simple search and advanced search.

## Admin / Management

- [ ] **Advanced admin panel & statistics** — create an admin UI where site admins can view system statistics and perform administrative actions. Core requirements:
	- Invite users via email (backend SMTP support, invite tokens, templates, queueing).
	- User management (roles, activate/deactivate, quotas, API keys, rate limits).
	- Dashboard: storage usage, total tracks/albums, active users, concurrent streams, ingest progress, play counts, error logs.
	- Job control: start/stop/reschedule ingest jobs, re-scan library, re-fetch metadata, regenerate waveforms.
	- Audit logs & activity feed for actions performed by admins/users.
	- Backup & restore controls (database + object store snapshots), and manual/automatic export of playlists.
	- Metrics and monitoring integration (Prometheus / Grafana endpoints) and health checks.
	- Site settings UI: SMTP config, storage backends, object store credentials, CDN / proxy settings.
	- Security: 2FA support, SSO/LDAP/OAuth optional integrations, and access tokens for external apps.

## Brainstorm: Good features for a self-hosted Spotify-like app

- Support multiple storage backends (local FS, S3-compatible, MinIO) and remote object stores.
- Import/export playlists (Spotify/Apple/M3U/JSPF) and one-click Spotify playlist import.
- Collaborative playlists and shareable invite links with permissions.
- On-the-fly transcoding / bitrate limiting for bandwidth-constrained devices.
- DLNA / UPnP and Chromecast support; local network discovery (mDNS).
- Offline sync for mobile apps (download + DRM-free playback) and per-user storage quotas.
- Scrobbling (Last.fm), remote control (web + mobile + physical devices), and multi-room playback.
- Fine-grained recommendations and smart shuffle (weight by recency, rating, play count).
- Per-user customizable EQ, crossfade/gapless playback, and advanced playback settings.
- Audit logs, activity analytics (most-played, least-played, trending), and scheduled reports.
- Admin tools: bulk metadata edit, artwork uploader, duplicate detector, and missing-artwork scanner.
- Security and privacy: TLS, OAuth, API tokens, rate limiting, and optional public/ private instance modes.
- Integrations: webhooks for events, Prometheus metrics, LDAP/SSO, SMTP for invites & notifications.

*Example note:* the admin invite flow requires backend mail support (SMTP config, templates, queuing, retries). Add a backend mailer and an admin SMTP settings UI as part of the implementation plan.
