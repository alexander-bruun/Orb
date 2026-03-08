# Tauri Car Integration Plugin — Technical Architecture

Version: 0.1
Date: 2026-03-07

## Purpose

This document describes the technical architecture for the `tauri-plugin-car` project (Android Auto + Apple CarPlay). It expands the high-level design into concrete cross-platform bridging patterns, thread and lifecycle models, state synchronization, media session architecture, testing guidance, and an actionable implementation task list suitable for dropping into the repo.

A open source rust project has already been made strictly for the android auto support: <https://github.com/uglyoldbob/android-auto> let's use that as the ground works.

Also take a look at <https://github.com/advplyr/audiobookshelf-app> for inspiration, they seem to use Vue and are able to create a Android Auto app integration.

Android also has a public example repository available: <https://github.com/android/car-samples>

Feel free to clone all 3 repositories to get deep insight into them without needing to perform http queries.

**Audience:** plugin authors, native engineers (Kotlin/Swift), Rust engineers, and JS/TS integrators.

## Goals and Scope

- Provide a robust, extensible plugin that exposes a single JavaScript API to Tauri guests and implements platform-native integration for Android Auto (Android) and CarPlay (iOS).
- Focus on media apps initially: now-playing, playback control, and media browsing.
- Keep the Rust core authoritative for app state and event routing; native adapters translate platform events to/from that core.

Out-of-scope: rendering arbitrary UIs inside car systems, unsupported app categories, or workarounds for platform restrictions.

## High-level Components

- JS guest: `guest-js/index.ts` — JS/TS API surface, event listeners, typed invoke wrappers.
- Rust plugin core: `src/lib.rs`, `src/commands.rs`, `src/models.rs`, `src/mobile.rs` — command routing, serialization, event emission, and platform dispatch.
- Android backend (Kotlin): `android/` — CarAppService, Session, MediaSession, UI screens.
- iOS backend (Swift): `ios/` — CarPlay scene delegate, interface controller, templates, now-playing integration.

Diagram (conceptual):

Tauri Web App (JS)
  ↕ (tauri invoke / events)
Rust Plugin Core (single source-of-truth)
  ↕ (internal channels / serde JSON)
Native Adapters — Android (Kotlin) / iOS (Swift)
  ↔ Car Systems (Android Auto / CarPlay)

## Data Models (shared)

Use serde-compatible structs in Rust and JSON shapes in JS. Keep models stable and additive.

- Track
  - id: string
  - title: string
  - artist?: string
  - album?: string
  - duration?: u64 (seconds or ms, consistent across layers)
  - artwork_url?: string

- MediaItem
  - id: string
  - title: string
  - subtitle?: string
  - playable: bool
  - children?: MediaItem[]

Keep wire protocol explicit: ISO timestamp formats if needed and units documented.

## JS API (guest-js)

Expose a minimal, stable API that maps to Rust commands and listens for events:

- car.onCarConnected(handler)
- car.onCarDisconnected(handler)
- car.setNowPlaying(track)
- car.onPlay(handler), onPause, onNext, onPrevious
- car.setMediaRoot(mediaRoot)
- car.requestPlay(itemId)

Implementation notes

- Wrap `invoke` calls with typed request/response shapes and surface simple Promise-based APIs.
- Use an event emitter inside guest-js that subscribes to plugin-emitted events (e.g., `car:play`).

## Rust Core: responsibilities and patterns

Responsibilities

- Central command handlers (annotated with `#[tauri::command]`) for guest->native actions.
- Maintain authoritative plugin state (connected status, playback state, media tree cache).
- Serialize models to/from JSON and forward events to the guest.
- Offer platform dispatchors (cfg/target_os) to call into native backends.

Concurrency and thread model

- The Rust core runs on the Tauri app runtime (async). Use async-friendly primitives (Tokio) where present.
- Use a single authoritative state object protected by `tokio::sync::RwLock` or `parking_lot::RwLock` (if synchronous code must access it) to avoid deadlocks.
- Event emission should be non-blocking: spawn tasks for long work and use `tokio::sync::mpsc` or `async-broadcast` to fan-out events.

Event flow examples

1) JS calls `setNowPlaying(track)` → tauri `invoke` → Rust `set_now_playing` command → update state → serialize and forward to native adapter → native updates platform now-playing UI.

2) User taps Play in car → native receives event → native adapter sends event to Rust core over the adapter bridge → Rust core validates and emits `car:play` event to JS.

Event names and payloads should be documented and stable. Use a small, consistent prefix (e.g., `car:`) for event names.

## Native Bridges (detailed)

Design goals

- Keep native backends thin: translate platform APIs/events into the Rust-defined wire protocol.
- Ensure native code runs UI interactions on the platform UI thread.
- Avoid duplicating app logic: forward required decisions to Rust/JS.

Common patterns

- Messaging format: JSON using shared model shapes. Rust uses `serde` to (de)serialize.
- Transport: call into platform code from Rust via platform-specific plugin APIs built into the Tauri mobile/embedding system (or, if embedding, via FFI / JNI wrappers that Tauri's Android/iOS bindings provide).
- Events from native → Rust: use the platform-to-Rust callback path that the Tauri mobile plugin exposes (or invoke a Rust function via a binding). Ensure callbacks are marshalled to background worker threads immediately and then forwarded to the Rust async runtime.

Android (Kotlin) specifics

- Use AndroidX Car App Library (`androidx.car.app`) for UI templates.
- Components:
  - `CarAppService` subclass to register the app
  - `Session`/`Screen` classes to provide templates (e.g., `ListTemplate`, `PaneTemplate`) and to react to UI actions
  - `MediaSession` + `MediaBrowserService` to integrate with Android media framework and expose playback controls to other devices
- Threading:
  - Platform callbacks (templates, onClick) execute on Android main/UI thread. Bridge work must post to a background executor for interop with Rust.
  - Use `Handler(Looper.getMainLooper())` or `runOnUiThread` when sending UI updates from background tasks.
- Lifecycle: CarAppService lifecycle is controlled by car connection state; always ensure the Rust core is notified on connect/disconnect.

iOS (Swift) specifics

- Use `CarPlay.framework` and MPNowPlaying/AVFoundation stack where appropriate.
- Components:
  - `CPTemplateApplicationSceneDelegate` / `CarPlaySceneDelegate` (scene-based CarPlay entry)
  - `CPInterfaceController` to set templates (`CPListTemplate`, `CPNowPlayingTemplate`)
  - `NowPlaying` integration: update MPNowPlayingInfoCenter and route commands through CarPlay templates
- Threading:
  - UI interactions occur on the main thread. Use `DispatchQueue.main.async` for UI updates and `DispatchQueue.global().async` for background work.
- Permissions and entitlements:
  - CarPlay requires entitlements (`com.apple.developer.carplay-audio`) and App Store approval for CarPlay functionality. Document here and gate CarPlay features behind build-time flags.

Bridge implementation notes

- Provide a minimal native-to-Rust glue: a small set of exported functions in Rust (via `#[no_mangle]` or the Tauri mobile plugin interface) that native code calls with JSON payloads.
- On Android, prefer using the Tauri Android plugin host and `PluginApi` to send events rather than raw JNI if the environment provides it.
- On iOS, bridge through the Tauri plugin entrypoints and call Rust async handlers from Swift using the provided runtime helper.

## State synchronization

Principles

- Single source-of-truth: Rust core holds canonical state (playback status, current track, connection status).
- Native backends keep minimal local state necessary for UI rendering and forward authoritative changes to Rust.
- JS reads from Rust (via `invoke` or events) and may request state updates.

Algorithms

- For frequently updated fields (e.g., playback position), use a delta update strategy: native reports periodic updates (position, buffer) and Rust aggregates/schedules broadcast to JS to avoid event storms.
- For media browsing, Rust can cache media trees (in-memory) and serve paginated requests to native backends; large lists should be requested lazily.

Concurrency primitives

- Use `RwLock` around state to allow many readers with exclusive writers.
- Use bounded channels for event forwarding to avoid unbounded memory usage.

Conflict resolution

- If both native and JS attempt to change state simultaneously, Rust uses last-writer-wins with timestamps, but exposes hooks for app logic to resolve conflicts (e.g., always prefer user-initiated JS actions).

## Media session architecture

Responsibilities

- Accept `setNowPlaying(track)` from JS and update both native now-playing UI and system media session frameworks.
- Surface native playback controls (play/pause/seek/next/previous) as events back to the JS app.

Android details

- Use `MediaSessionCompat` or `MediaSession` along with `MediaBrowserServiceCompat` for older compat layers if needed.
- When a NowPlaying update arrives, update `MediaMetadata` and `PlaybackState`.
- Forward media button events to the Rust core.

iOS details

- Update `MPNowPlayingInfoCenter.default().nowPlayingInfo` from Rust model.
- Implement `MPRemoteCommandCenter` or respond to CarPlay template actions and forward them to Rust.

Artwork handling

- Native backends should fetch artwork from a URL or accept a binary blob pushed by Rust. Prefer URL-based artwork (cached locally) to avoid large cross-boundary transfers.

## Error handling and observability

- Return typed errors from Rust commands (structured with codes and messages).
- Emit diagnostic events (e.g., `car:warn`, `car:error`) for platform-specific issues.
- Log useful context on native side and forward high-level errors to Rust for guest consumption.

## Testing strategy

Android

- Use the Android Auto Desktop Head Unit (DHU) for integration testing.
- Use emulator + `adb` to simulate media actions.
- Unit test the Kotlin bridge components with Robolectric / instrumentation tests for lifecycle.

iOS

- Use Xcode CarPlay simulator and unit-test Swift bridge code.
- Test entitlements and scene delegate behavior.

End-to-end

- A Tauri example app in `/examples/media-player` (or the repo example) should include a debug mode that simulates car connect/disconnect to verify event flows.

CI

- Run Rust unit tests and TypeScript type checks on PRs; native tests run on macOS or CI runners with simulator support when possible.

## Security and privacy

- Avoid shipping sensitive user data in logs. Limit event payloads to necessary metadata.
- Ensure URLs (artwork) are validated and fetched over HTTPS.

## Implementation Task List (concrete)

Phase 1 — Foundation

- Clone the Tauri plugin repository: <https://github.com/tauri-apps/plugins-workspace/tree/v2/plugins> to ~/blade/plugins-workspace
- Analyze all other plugins, to fully understand the capabilities and how people develop plugins for Tauri
- Create plugin scaffold: `tauri-plug-car/` in the Orb git repository with Rust crate and `guest-js/` package.
- Add README and this architecture doc: [tauri-plugin-car/TECHNICAL_ARCHITECTURE.md](tauri-plugin-car/TECHNICAL_ARCHITECTURE.md)

Phase 2 — JS API

- Implement `guest-js/index.ts`:
  - typed `setNowPlaying`, `setMediaRoot` invokes
  - event emitter wrapper for `car:play`, `car:pause`, `car:connected`

Phase 3 — Rust Core

- Files to implement: `src/lib.rs`, `src/commands.rs`, `src/models.rs`, `src/mobile.rs`.
  - `commands.rs`: tauri commands for guest actions (set_now_playing, set_media_root, request_play)
  - `models.rs`: `Track`, `MediaItem`, `PlaybackState` with serde derives
  - `mobile.rs`: platform dispatch (cfg-target) and event emitter helpers

Phase 4 — Android backend (stub)

- Add `android/src/main/java/CarPlugin.kt` with plugin registration and stub methods to receive JSON messages and emit back basic lifecycle events.
- Implement `CarAppService.kt` and a minimal `CarSession.kt` that can respond to a test list and playback commands.

Phase 5 — iOS backend (stub)

- Add `ios/CarPlugin.swift` with entrypoints and `CarPlaySceneDelegate.swift` stub that reports connect/disconnect and user actions to Rust.

Phase 6 — Integration + Example

- Create `examples/media-player/` that uses `guest-js` and demonstrates setNowPlaying and playback actions.

Phase 7 — Iterate

- Replace stubs with full native platform implementations per earlier paragraphs.

## Non-functional requirements

- API stability: semver for the plugin; prefer additive changes to models.
- Performance: keep event round-trips < 300ms for interactive controls.

## Risks & Mitigations

- Apple Approval: gate CarPlay feature behind build-time flags and provide fallbacks in the example app. Document entitlement steps.
- Lifecycle mismatches: ensure explicit connect/disconnect events and idempotent initialization in native adapters.

## Next Steps (immediate coding tasks)

1. Create the plugin scaffold (run `npx @tauri-apps/cli plugin new car --android --ios`).
2. Implement the Rust core skeleton files listed above.
3. Implement `guest-js/index.ts` with typed invocations and event emitter.
4. Create Android and iOS native stubs that compile and emit a basic `car:connected` event.
5. Add the `examples/media-player` demo that toggles now-playing state.

If you want, I can now implement the Rust skeleton and the `guest-js/index.ts` file in this repo and add the example app scaffolding — tell me which tasks to start coding first.

---
Generated and added: [tauri-plugin-car/TECHNICAL_ARCHITECTURE.md](tauri-plugin-car/TECHNICAL_ARCHITECTURE.md)
