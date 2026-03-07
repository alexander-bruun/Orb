# Tauri Development

Orb uses [Tauri v2](https://v2.tauri.app/) for native desktop and mobile builds. The frontend is a SvelteKit static site wrapped in a Tauri WebView shell.

## Project Structure

```
web/
  src-tauri/
    tauri.conf.json       # Tauri config (window, bundle, build commands)
    Cargo.toml            # Rust dependencies
    src/
      main.rs             # Desktop entry point
      lib.rs              # Shared entry point (desktop + mobile)
      desktop.rs          # Desktop-only: tray, mDNS, Discord RPC
    gen/                  # Generated (gitignored)
      android/            # Android project (created by `tauri android init`)
      ios/                # iOS project (created by `tauri ios init`)
  icons/                  # App icons for all platforms
scripts/
  android.env             # Android SDK environment variables
  ios.env                 # iOS environment variables
  android-build.sh        # Build Android APK/AAB
  android-sign.sh         # Configure signing (--dev, --generate, --configure)
  ios-build.sh            # Build iOS IPA (macOS only)
  ios-sign.sh             # Configure signing (--dev, --auto, --manual)
  dev-keystore.jks        # Dev signing keystore (generated, gitignored)
```

## Dependencies

### All Platforms

| Dependency | Install |
|---|---|
| Rust (via rustup) | `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \| sh` |
| Bun | `curl -fsSL https://bun.sh/install \| bash` |
| Node.js | Not required (bun replaces it) |

### Desktop (Linux)

```bash
sudo apt-get install -y \
  libwebkit2gtk-4.1-dev \
  libappindicator3-dev \
  librsvg2-dev \
  patchelf
```

On Fedora:

```bash
sudo dnf install -y \
  webkit2gtk4.1-devel \
  libappindicator-gtk3-devel \
  librsvg2-devel \
  patchelf
```

### Desktop (macOS)

Install Xcode or Xcode Command Line Tools.

### Desktop (Windows)

Install [Microsoft C++ Build Tools](https://visualstudio.microsoft.com/visual-cpp-build-tools/) with the "Desktop development with C++" workload.

### Android

| Dependency | Install |
|---|---|
| Android Studio or SDK | [developer.android.com](https://developer.android.com/studio) or `snap install androidsdk` |
| Java 17+ | Bundled with Android Studio, or `sudo dnf install java-21-openjdk-devel` |
| Android SDK Platform | Via SDK Manager: `platforms;android-34` (or newer) |
| Android Build Tools | Via SDK Manager: `build-tools;34.0.0` |
| Android NDK | Via SDK Manager: `ndk;27.0.12077973` |
| Android Platform Tools | Via SDK Manager: `platform-tools` |
| Rust Android targets | `rustup target add aarch64-linux-android armv7-linux-androideabi i686-linux-android x86_64-linux-android` |

Set your environment in `scripts/android.env`:

```bash
ANDROID_HOME="$HOME/Android/Sdk"
ANDROID_SDK_ROOT="$ANDROID_HOME"
NDK_HOME="$ANDROID_HOME/ndk/27.0.12077973"
JAVA_HOME="/usr/lib/jvm/java-21-openjdk"
PATH="$HOME/.cargo/bin:$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools:$PATH"
```

### iOS (macOS only)

| Dependency | Install |
|---|---|
| Xcode | Mac App Store |
| Cocoapods | `brew install cocoapods` |
| Rust iOS targets | `rustup target add aarch64-apple-ios x86_64-apple-ios aarch64-apple-ios-sim` |

## Quick Start

### Desktop

```bash
# Install frontend deps
make web-install

# Development (hot-reload)
cd web && bunx tauri dev

# Release build
make tauri-build
```

### Android

```bash
# First time: initialize Android project + dev signing
make tauri-android-init

# Build signed APK + AAB
make tauri-android-build
```

The APK is output at:
`web/src-tauri/gen/android/app/build/outputs/apk/universal/release/app-universal-release.apk`

### iOS

```bash
# First time: initialize iOS project
make tauri-ios-init

# Build IPA
make tauri-ios-build
```

## Signing

### Android Dev Signing

`make tauri-android-dev-sign` generates a `scripts/dev-keystore.jks` with a dummy certificate and patches `build.gradle.kts` automatically. This is called by `make tauri-android-init` so you usually don't need to run it manually.

### Android Release Signing

```bash
# Generate a release keystore (interactive, prompts for passwords)
scripts/android-sign.sh --generate

# Configure Gradle to use it
scripts/android-sign.sh --configure
```

Keep `release-keystore.jks` safe and never commit it. To export for CI:

```bash
base64 -w0 release-keystore.jks
```

Then set these GitHub secrets:
- `ANDROID_KEY_BASE64` - base64-encoded keystore
- `ANDROID_KEY_ALIAS` - key alias (default: `upload`)
- `ANDROID_KEY_PASSWORD` - key password

### iOS Dev Signing

`make tauri-ios-dev-sign` uses Xcode's automatic signing. Ensure your Apple ID is added in Xcode > Settings > Accounts.

### iOS Release Signing

**Option A: App Store Connect API (recommended for CI)**

```bash
export APPLE_API_ISSUER="issuer-id"
export APPLE_API_KEY="key-id"
export APPLE_API_KEY_PATH="/path/to/AuthKey.p8"
scripts/ios-sign.sh --auto
```

**Option B: Certificate + Provisioning Profile**

```bash
export IOS_CERTIFICATE="<base64-p12>"
export IOS_CERTIFICATE_PASSWORD="password"
export IOS_MOBILE_PROVISION="<base64-mobileprovision>"
scripts/ios-sign.sh --manual
```

## Architecture Notes

### Desktop vs Mobile Code

Desktop-only Rust code (system tray, mDNS discovery, Discord RPC) lives in `src/desktop.rs` and is gated behind `#[cfg(desktop)]`. The mobile build compiles only `lib.rs` which provides a minimal Tauri app without these features.

### Platform Detection (Frontend)

`web/src/lib/utils/platform.ts` exports:

- `isTauri()` - true when running in any Tauri shell
- `isNative()` - alias for `isTauri()`
- `nativePlatform()` - returns `'ios'`, `'android'`, or `'web'`

### Generated Files

The `web/src-tauri/gen/` directory is gitignored. It's created by `tauri android init` / `tauri ios init` and can be safely deleted and regenerated. The `make tauri-android-init` target automatically re-applies dev signing after regeneration.

## CI/CD

The release workflow (`.github/workflows/release.yml`) builds all targets on tag push:

- **Desktop**: Linux (`.deb`, `.AppImage`), macOS (`.dmg`), Windows (`.exe`) via `build-tauri`
- **Android**: APK + AAB via `build-tauri-android` (uses dummy keystore or secrets)
- **iOS**: IPA via `build-tauri-ios` on `macos-latest` (optional signing via Apple API secrets)

All artifacts are uploaded to the GitHub release.
