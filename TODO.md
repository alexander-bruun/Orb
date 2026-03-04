
# AI-Optimized TODO List (Elaborated & Expanded)

## 1. Refactor Track Title Display Logic

**Goal:** Exclude featured artists from the main track title and render all artist names (including featured) as individual clickable elements in the track row.
**Steps:**
    - Audit all UI components and backend endpoints that render track titles and artist names.
    - Update backend to return featured artist data as a separate field, not embedded in the title string.
    - Refactor frontend logic to parse and display main and featured artists as separate clickable elements.
    - Ensure clicking any artist navigates to their artist page (test navigation for all cases).

## 2. Add Flexible Image Resizing to Cover API

**Goal:** Support width/height parameters for cover images, maintaining aspect ratio, and ensure uniform card sizes on the home page.
**Steps:**
    - Extend the cover API to accept `width` and `height` query parameters.
    - Implement logic to maintain aspect ratio if only one dimension is provided (set the other to "auto").
    - Update frontend home page to always request covers with a fixed size for consistent card layout.

## 3. Robust Deployment & Ingest Leadership

**Goal:** Bundle backend and ingest in the same pod, with a leader election mechanism for ingest duties, and explore round-robin ingest distribution.
**Steps:**
    - Research Kubernetes pod bundling and leader election patterns (etcd, Redis, K8s primitives).
    - Implement a leader election mechanism (quorum) so only one pod performs ingest at a time.
    - Optionally, implement round-robin ingest distribution with enrichment limited to the primary pod (due to API rate limits).
    - Update deployment manifests and CI/CD pipeline to support the new strategy.
    - Add monitoring and alerting for leadership changes and ingest failures.

## 4. Movable & Toggleable Sound Visualizer

**Goal:** Allow users to toggle and reposition sound visualizer components in the UI.
**Steps:**
    - Design UI/UX for toggling and moving visualizers (drag-and-drop or position presets).
    - Implement state management for visualizer position and visibility (persist user preference).
    - Integrate with the audio engine for real-time audio data.
    - Ensure accessibility (keyboard navigation, ARIA labels) and responsiveness (mobile/desktop).

## 5. Spectrum Analyzer Widget

**Goal:** Provide a real-time spectrum analyzer for audio visualization.
**Steps:**
    - Research best practices for real-time spectrum analysis in web audio (Web Audio API, FFT).
    - Implement a performant spectrum analyzer using Canvas or WebGL.
    - Integrate with the audio engine to receive FFT data.
    - Add customization options (color, style, size, animation speed).
    - Benchmark performance on low-end devices.

## 6. Waveform Widget for Audio Visualization

**Goal:** Display audio waveforms for tracks, supporting large files and user interaction.
**Steps:**
    - Implement waveform rendering using decoded PCM data.
    - Optimize for performance with large tracks (virtualization, downsampling).
    - Allow zooming and scrolling for detailed navigation.
    - Integrate with the audio engine and UI.
    - Add accessibility features and keyboard shortcuts for navigation.

## 7. Discord Rich Presence Integration

**Goal:** Show current playback status in Discord for native desktop apps.
**Steps:**
    - Research Discord Rich Presence APIs and libraries for Tauri/Electron.
    - Implement or integrate a native module for Discord Rich Presence.
    - Update the app to send track, artist, and playback state to Discord.
    - Add user settings to enable/disable this feature.

## 8. Listen Party: 4-Digit Code Access

**Goal:** Add optional 4-digit code protection for Listen Party sessions.
**Steps:**
    - Update Listen Party backend to support code generation and validation.
    - Add UI controls for hosts to enable/disable code protection.
    - Display the code in the session UI and require it for joining if enabled.

## 9. CI/CD: Tauri Mobile Build Integration

**Goal:** Automate mobile builds (Android/iOS) in the CI pipeline.
**Steps:**
    - Update CI configuration to build Tauri mobile targets (Android APK, iOS IPA).
    - Set up secure storage for signing keys and certificates in GitHub Actions.
    - Automate APK/IPA artifact upload for each release.

## 10. Official Branding for App Icons

**Goal:** Replace all placeholder icons with official Orb branding.
**Steps:**
    - Gather official branding assets in all required resolutions.
    - Update Tauri/Electron config files to use new icons for all platforms.
    - Verify correct icon display on all supported OSes (Windows, macOS, Linux, Android, iOS).
    - Add a branding guide for future asset updates.

## 11. Personal & Per-Genre Equalizer Profiles

**Goal:** Allow users to create personal and per-genre EQ profiles.
**Steps:**
    - Extend user settings backend and database schema to store EQ profiles.
    - Add UI for users to create, edit, and select EQ profiles.
    - Integrate EQ settings into the audio engine.
    - Implement per-genre profile selection logic.
    - Add import/export for EQ profiles.

## 12. Enhanced Tray Bar Controls (Native Apps)

**Goal:** Show playback controls in the tray bar window preview on hover.
**Steps:**
    - Update tray bar UI to show previous, pause/play, and next buttons below the image preview on hover.
    - Implement event handlers for playback actions.

## 13. Multi-Disc Album Support

**Goal:** Support albums with multiple discs and display them correctly in the UI.
**Steps:**
    - Update database and API to support disc numbers.
    - Refactor album detail UI to group tracks by disc.
    - Ensure correct ordering and display for multi-disc albums.

## 14. Polling Fallback for File System Monitoring

**Goal:** Ensure reliable file system monitoring if inotify is unavailable.
**Steps:**
    - Detect inotify support at runtime during ingest startup.
    - If unavailable, fall back to periodic polling of directories.
    - Make polling interval configurable.
    - Log and alert if fallback is active.

## 15. Admin Analytics Dashboard

**Goal:** Provide admins with user activity and listening history analytics.
**Steps:**
    - Track and visualize user listening patterns, most played tracks, and active sessions.
    - Provide export options (CSV, JSON).
    - Add role-based access control for analytics features.

## 16. User Following & Public Profiles

**Goal:** Enable user-to-user following and public profile pages.
**Steps:**
    - Allow users to follow each other and view public playlists and stats.
    - Add privacy controls for users.
    - Implement notifications for new followers.

## 17. Collaborative Playlist Editing

**Goal:** Allow multiple users to add/remove tracks in shared playlists.
**Steps:**
    - Update playlist backend and permissions model.
    - Add UI for inviting collaborators and managing permissions.
    - Add activity log for playlist changes.

## 18. Podcast Support (Optional)

**Goal:** Add podcast ingestion and playback as a toggleable feature.
**Steps:**
    - Add podcast ingestion and playback support.
    - Update UI to browse and play podcasts.
    - Add user settings to enable/disable podcast features.

## 19. Advanced Search Filters

**Goal:** Enable users to filter and sort search results by genre, year, bitrate, etc.
**Steps:**
    - Extend search API and UI to support filtering and sorting.
    - Add UI for saving and reusing custom search filters.

## 20. Two-Factor Authentication (2FA)

**Goal:** Add 2FA for user accounts using TOTP apps.
**Steps:**
    - Integrate with TOTP apps (Google Authenticator, Authy).
    - Add UI for setup and recovery.
    - Add backup codes and recovery options.

## 21. Per-User Streaming Quality Controls

**Goal:** Allow users to set preferred streaming quality and bandwidth limits.
**Steps:**
    - Allow users to set preferred streaming quality (bitrate, sample rate).
    - Enforce limits in the streaming API.
    - Add adaptive streaming for fluctuating network conditions.

## 22. Offline Sync/Download Support

**Goal:** Enable offline playback for mobile and desktop apps.
**Steps:**
    - Allow users to mark albums/playlists for offline use.
    - Implement secure local storage and DRM (if desired).
    - Add UI for managing offline content and storage usage.

### 23. Smart Radio & Autoplay

**Goal:** Automatically generate a radio station or autoplay queue based on user taste and listening history.
**Steps:**
    - Implement a recommendation engine using collaborative filtering or content-based methods.
    - Add "Start Radio" and "Autoplay" buttons to track/album/artist pages.
    - Continuously update the queue as tracks finish.

### 24. Implement local & global radio support.
