#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
WEB_DIR="$PROJECT_ROOT/web"

# ── Source ios.env ────────────────────────────────────────────────────────────

if [ -f "$SCRIPT_DIR/ios.env" ]; then
    . "$SCRIPT_DIR/ios.env"
fi

# ── Platform check ────────────────────────────────────────────────────────────

if [ "$(uname)" != "Darwin" ]; then
    echo "ERROR: iOS builds require macOS with Xcode installed."
    echo "This script cannot run on $(uname)."
    exit 1
fi

# ── Pre-flight checks ────────────────────────────────────────────────────────

check_prereqs() {
    local missing=()
    ! command -v xcodebuild &>/dev/null && missing+=("Xcode")
    ! command -v rustup &>/dev/null && missing+=("Rust (rustup)")
    ! command -v bun &>/dev/null && missing+=("Bun")
    if [ ${#missing[@]} -gt 0 ]; then
        echo "ERROR: Missing prerequisites: ${missing[*]}"
        exit 1
    fi
}

check_rust_targets() {
    local targets
    targets=$(rustup target list --installed)
    if ! echo "$targets" | grep -q "aarch64-apple-ios"; then
        echo "Installing Rust iOS targets..."
        rustup target add aarch64-apple-ios x86_64-apple-ios aarch64-apple-ios-sim
    fi
}

# ── Init if needed ────────────────────────────────────────────────────────────

init_ios() {
    if [ ! -d "$WEB_DIR/src-tauri/gen/ios" ]; then
        echo "Initializing Tauri iOS target..."
        cd "$WEB_DIR" && bunx tauri ios init --ci
    fi
}

# ── Build ─────────────────────────────────────────────────────────────────────

build_ios() {
    echo "Building iOS IPA..."
    cd "$WEB_DIR"
    bunx tauri ios build
    echo ""
    echo "Build complete. Check src-tauri/gen/ios/ for artifacts."
}

# ── Main ──────────────────────────────────────────────────────────────────────

check_prereqs
check_rust_targets
init_ios
build_ios
