
# Orb

Orb is a self-hosted, lossless music streaming platform - a personal Spotify backed by your own audio library. Stream FLAC, WAV, and other lossless formats at full fidelity to a modern web client. Multi-user, playlists, and queue support.

![Orb Desktop UI](desktop.png)
<table align="center"><tr>
  <td><img src="mobile.png" alt="Orb Mobile UI" width="300"></td>
  <td><img src="mobile-lyrics.png" alt="Orb Mobile + Lyrics UI" width="300"></td>
  <td><img src="mobile-offline.png" alt="Orb Mobile Offline" width="300"></td>
</tr></table>

## ✨ Features

- **Library Management:** Automatic indexing, metadata extraction, album art embedding
- **Streaming:** Lossless FLAC/WAV/MP3 at full bit depth (up to 32-bit/192kHz), HTTP range requests, client-side WASM decoding
- **User Management:** Multi-user, individual libraries, playlists, persistent queue
- **Discovery:** Advanced search, favorites, recently played
- **UI:** SvelteKit frontend with Melt UI + Tailwind, responsive layout

## 📦 Supported Platforms

Orb is distributed exclusively as Docker images for Linux (amd64, arm64). Web UI is accessible from the browser, and can be installed as a PWA on mobile.

## 🚀 Quick Start

### Using Docker (Recommended)

```bash
docker compose -f docker-compose.local.yml up -d
```

## 🤝 Contributing

Contributions welcome! Report bugs, suggest features, or submit PRs.

## 📄 License

[MIT License](https://github.com/alexander-bruun/orb/blob/main/LICENSE)
