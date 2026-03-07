# Feature Ideas

- [ ] **"Radio" mode** — infinite queue seeded from a track or artist using the existing `recommend` service, auto-fetching more tracks as the queue drains.
- [ ] **Track ratings (1–5 stars)** — finer-grained than a binary favorite; enables weighted recommendations.
- [ ] **Auto-playlist (smart playlist)** — saved filter rules (genre = "Jazz", year > 2000, play count < 5) that dynamically populate a playlist on open. These should be shown on the home page, and refresh once a week, and once a day when it makes sense.
- [ ] **Playlist collaboration** — allow other users on the same instance to add/remove tracks from a shared playlist (extends the existing listen party infrastructure).
- Integrations: webhooks for events & notifications.
- Collaborative playlists and shareable invite links with permissions.
- On-the-fly transcoding / bitrate limiting for bandwidth-constrained devices.
- Look for any usage of unicode icons, and refactor them to use proper icons. THey dont render the same on all devices.

- [ ] **Advanced admin panel & statistics** — create an admin UI where site admins can view system statistics and perform administrative actions. Core requirements the first user created is the default admin:

- Invite users via email (backend SMTP support, invite tokens, templates, queueing).
- User management (roles, activate/deactivate, quotas, API keys, rate limits).
- Dashboard: storage usage, total tracks/albums, active users, concurrent streams, ingest progress, play counts, error logs.
- Job control: start/stop/reschedule ingest jobs, re-scan library, re-fetch metadata, regenerate waveforms.
- Audit logs & activity feed for actions performed by admins/users.
- Backup & restore controls (database + object store snapshots), and manual/automatic export of playlists.
- Metrics and monitoring integration (Prometheus / Grafana endpoints) and health checks.
- Site settings UI: SMTP config, CDN / proxy settings.
- Admin tools: bulk metadata edit, artwork uploader, duplicate detector, and missing-artwork scanner.
- Audit logs, activity analytics (most-played, least-played, trending), and scheduled reports.
- admin tool to list albums without cover art and trigger a MusicBrainz re-fetch for them.
- stream ingest job status to the admin UI in real time instead of polling.
- the admin invite flow requires backend mail support (SMTP config, templates, queuing, retries). Add a backend mailer and an admin SMTP settings UI as part of the implementation plan.
