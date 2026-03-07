# Feature Ideas

## Playback & Discovery

- [ ] **Sleep timer** — stop playback after N minutes or at the end of the current track/album. Simple client-side timer, no backend needed.
- [ ] **Crossfade / gapless playback** — blend the tail of one track into the start of the next using the Web Audio API.
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

- [ ] **Artist bio / discography timeline** — pull extended artist info from MusicBrainz (client already exists) and display it on the artist detail page.
- [ ] **Waveform scrubber** — pre-generate waveform data during ingest and render it as the seek bar (audiowaveform or similar).
- [ ] **Themes / accent color picker** — `themeStore` already exists, just expose more customization.

## Ingest / Admin

- [ ] **Watch folder** — inotify-based watcher in the ingest service to auto-ingest new files dropped into the library directory, no manual re-scan needed.
- [ ] **Ingest progress WebSocket** — stream ingest job status to the admin UI in real time instead of polling.
- [ ] **Per-user upload** — let non-admin users upload their own tracks into a personal namespace.

- Make it possible to click lyrics, which should result in seeking the song to that location and playing from there.
- Rework the filter page, it's not very intuitive to have to expand the menu to search, and we should have a large search field on the page itself so we dont rely on the field in the top. The search field in the top bar, should show search results in a little modal showing the results. Not the search page itself. So they are basically simple search and advanced search.
