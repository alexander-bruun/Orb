<div align="center">
  <img src="https://raw.githubusercontent.com/alexander-bruun/orb/main/web/static/icon.svg" alt="Orb" width="96" />
  <br/>
  <img src="https://raw.githubusercontent.com/alexander-bruun/orb/main/web/static/logo.svg" alt="Orb" height="36" />
  <p>Self-hosted, lossless music streaming. Your library, your server, full fidelity.</p>
  <img alt="GitHub Release" src="https://img.shields.io/github/v/release/alexander-bruun/orb">
  <img alt="GitHub commit activity" src="https://img.shields.io/github/commit-activity/m/alexander-bruun/orb">
  <img alt="GitHub License" src="https://img.shields.io/github/license/alexander-bruun/orb">
  <img alt="GitHub Sponsors" src="https://img.shields.io/github/sponsors/alexander-bruun">
</div>

---

![Orb Desktop UI](https://raw.githubusercontent.com/alexander-bruun/orb/main/docs/desktop.png)

<table align="center"><tr>
  <td><img src="https://raw.githubusercontent.com/alexander-bruun/orb/main/docs/mobile.png" alt="Orb Mobile UI" width="300"></td>
  <td><img src="https://raw.githubusercontent.com/alexander-bruun/orb/main/docs/mobile-lyrics.png" alt="Orb Mobile + Lyrics" width="300"></td>
  <td><img src="https://raw.githubusercontent.com/alexander-bruun/orb/main/docs/mobile-offline.png" alt="Orb Mobile Offline" width="300"></td>
</tr></table>

---

## What is Orb?

Orb is a self-hosted music server built for audiophiles who want Spotify-level convenience without sacrificing audio quality or privacy. Point it at your library, spin up Docker, and stream FLAC, WAV, and other lossless formats at full bit depth from any browser — or install the native app on desktop and mobile.

## Features

- **Lossless streaming** — FLAC, WAV, AIFF, and more at up to 32-bit/192kHz via HTTP range requests and client-side WASM decoding
- **Library management** — Automatic indexing, metadata extraction, MusicBrainz enrichment, and embedded album art
- **Multi-user** — Individual libraries, playlists, and persistent queues per user
- **Discovery** — Advanced search, favorites, recently played, similarity-based recommendations, and autoplay radio
- **Listen Along** — Share a real-time listening session with guests via a shareable link
- **Native apps** — Desktop (Linux, macOS, Windows) and mobile (Android, iOS) via Tauri v2
- **PWA** — Installable from the browser on any device

## Quick Start

```bash
docker compose up -d
```

Then open `http://localhost:8080` in your browser.

See [Installation](installation.md) for full setup instructions, environment variables, and configuration options.

## Contributing

Contributions are welcome. Open an issue to report a bug or suggest a feature, or submit a pull request directly on [GitHub](https://github.com/alexander-bruun/orb).

## License

Orb is released under the [MIT License](https://github.com/alexander-bruun/orb/blob/main/LICENSE).
