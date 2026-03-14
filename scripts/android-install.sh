#!/usr/bin/env bash
set -euo pipefail

# android-install.sh
# Installs the built APK to a connected Android device via adb.
# Usage: ./scripts/android-install.sh [DEVICE_SERIAL]

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ADB="${ADB:-/mnt/c/Program Files (x86)/platform-tools/adb.exe}"
PACKAGE="com.orb.app"

# ── Find APK ─────────────────────────────────────────────────────────────────

# Prefer release (universal), then release (arm64), then debug.
APK=""
for candidate in \
    "$PROJECT_ROOT/web/src-tauri/gen/android/app/build/outputs/apk/universal/release/app-universal-release-signed.apk" \
    "$PROJECT_ROOT/web/src-tauri/gen/android/app/build/outputs/apk/universal/release/app-universal-release.apk" \
    "$PROJECT_ROOT/web/src-tauri/gen/android/app/build/outputs/apk/arm64/release/app-arm64-release.apk" \
    "$PROJECT_ROOT/web/src-tauri/gen/android/app/build/outputs/apk/debug/app-debug.apk"; do
    if [ -f "$candidate" ]; then
        APK="$candidate"
        break
    fi
done

if [ -z "$APK" ]; then
    echo "No APK found. Run 'make tauri-android-build' or 'scripts/android-build.sh' first." >&2
    exit 1
fi

# ── Find device ──────────────────────────────────────────────────────────────

DEVICE="${1:-}"
if [ -z "$DEVICE" ]; then
    DEVICE=$("$ADB" devices | sed -n '2p' | awk '{print $1}')
fi
if [ -z "$DEVICE" ]; then
    echo "No adb device found. Connect a device or provide serial as first arg." >&2
    exit 1
fi

# ── Install ──────────────────────────────────────────────────────────────────

echo "Installing $APK to device $DEVICE"
"$ADB" -s "$DEVICE" install -r "$APK"

echo ""
echo "Install complete. To view logs:"
echo "  \"$ADB\" -s $DEVICE logcat -v time | grep -Ei '$PACKAGE|tauri|exoplayer|mediasession|AndroidRuntime|ActivityManager' --line-buffered"
