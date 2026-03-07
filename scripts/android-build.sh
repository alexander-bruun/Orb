#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
WEB_DIR="$PROJECT_ROOT/web"

# ── Source android.env ────────────────────────────────────────────────────────

if [ -f "$SCRIPT_DIR/android.env" ]; then
    . "$SCRIPT_DIR/android.env"
fi

# ── Pre-flight checks ────────────────────────────────────────────────────────

check_env() {
    local missing=()
    [ -z "${ANDROID_HOME:-}" ] && [ -z "${ANDROID_SDK_ROOT:-}" ] && missing+=("ANDROID_HOME or ANDROID_SDK_ROOT")
    [ -z "${JAVA_HOME:-}" ] && ! command -v java &>/dev/null && missing+=("JAVA_HOME or java in PATH")
    if [ ${#missing[@]} -gt 0 ]; then
        echo "ERROR: Missing prerequisites: ${missing[*]}"
        echo ""
        echo "Set variables in scripts/android.env or install Android SDK and set:"
        echo "  ANDROID_HOME=\$HOME/Android/Sdk"
        echo "  JAVA_HOME=/path/to/jbr"
        echo "  NDK_HOME=\$ANDROID_HOME/ndk/<version>"
        echo ""
        echo "Then add Rust Android targets:"
        echo "  rustup target add aarch64-linux-android armv7-linux-androideabi i686-linux-android x86_64-linux-android"
        exit 1
    fi
}

check_rust_targets() {
    local targets
    targets=$(rustup target list --installed)
    if ! echo "$targets" | grep -q "aarch64-linux-android"; then
        echo "Installing Rust Android targets..."
        rustup target add aarch64-linux-android armv7-linux-androideabi i686-linux-android x86_64-linux-android
    fi
}

# ── Init if needed ────────────────────────────────────────────────────────────

init_android() {
    if [ ! -d "$WEB_DIR/src-tauri/gen/android" ]; then
        echo "Initializing Tauri Android target..."
        cd "$WEB_DIR" && bunx tauri android init --ci
    fi
}

# ── Build ─────────────────────────────────────────────────────────────────────

build_android() {
    echo "Building Android APK and AAB..."
    cd "$WEB_DIR"
    bunx tauri android build --apk --aab
    echo ""
    echo "Build complete. Artifacts:"
    find src-tauri/gen/android/app/build/outputs -name "*.apk" -o -name "*.aab" 2>/dev/null || echo "  (check src-tauri/gen/android/app/build/outputs/)"
}

# ── Main ──────────────────────────────────────────────────────────────────────

check_env
check_rust_targets
init_android
build_android
