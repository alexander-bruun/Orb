#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
WEB_DIR="$PROJECT_ROOT/web"

if [ -f "$SCRIPT_DIR/ios.env" ]; then
    . "$SCRIPT_DIR/ios.env"
fi

usage() {
    cat <<EOF
Usage: $0 [--dev | --auto | --manual]

  --dev      Set up for local development signing (automatic via Xcode)
  --auto     Use App Store Connect API for automatic signing
             Requires: APPLE_API_ISSUER, APPLE_API_KEY, APPLE_API_KEY_PATH
  --manual   Use certificate + provisioning profile
             Requires: IOS_CERTIFICATE, IOS_CERTIFICATE_PASSWORD, IOS_MOBILE_PROVISION
EOF
    exit 1
}

# ── Dev signing (Xcode automatic) ────────────────────────────────────────────

setup_dev() {
    if [ "$(uname)" != "Darwin" ]; then
        echo "iOS dev signing requires macOS with Xcode."
        echo "On Linux, iOS builds are only available via CI (--auto or --manual)."
        exit 0
    fi

    echo "iOS dev signing uses Xcode's automatic signing."
    echo "Ensure your Apple ID is added in Xcode > Settings > Accounts."
    echo ""
    echo "To build: make tauri-ios-build"
}

# ── Automatic signing (App Store Connect API) ─────────────────────────────────

setup_auto() {
    local missing=()
    [ -z "${APPLE_API_ISSUER:-}" ] && missing+=("APPLE_API_ISSUER")
    [ -z "${APPLE_API_KEY:-}" ] && missing+=("APPLE_API_KEY")
    [ -z "${APPLE_API_KEY_PATH:-}" ] && missing+=("APPLE_API_KEY_PATH")
    if [ ${#missing[@]} -gt 0 ]; then
        echo "ERROR: Missing env vars: ${missing[*]}"
        exit 1
    fi

    echo "Automatic signing configured via App Store Connect API."
    echo "Tauri will use these environment variables during build:"
    echo "  APPLE_API_ISSUER=$APPLE_API_ISSUER"
    echo "  APPLE_API_KEY=$APPLE_API_KEY"
    echo "  APPLE_API_KEY_PATH=$APPLE_API_KEY_PATH"
    echo ""
    echo "Run: scripts/ios-build.sh"
}

# ── Manual signing (certificate + provisioning profile) ──────────────────────

setup_manual() {
    local missing=()
    [ -z "${IOS_CERTIFICATE:-}" ] && missing+=("IOS_CERTIFICATE")
    [ -z "${IOS_CERTIFICATE_PASSWORD:-}" ] && missing+=("IOS_CERTIFICATE_PASSWORD")
    [ -z "${IOS_MOBILE_PROVISION:-}" ] && missing+=("IOS_MOBILE_PROVISION")
    if [ ${#missing[@]} -gt 0 ]; then
        echo "ERROR: Missing env vars: ${missing[*]}"
        exit 1
    fi

    if [ -n "${CI:-}" ]; then
        echo "Setting up CI keychain..."
        KEYCHAIN_PATH="${RUNNER_TEMP:-/tmp}/app-signing.keychain-db"
        KEYCHAIN_PASSWORD="$(openssl rand -base64 32)"

        security create-keychain -p "$KEYCHAIN_PASSWORD" "$KEYCHAIN_PATH"
        security set-keychain-settings -lut 21600 "$KEYCHAIN_PATH"
        security unlock-keychain -p "$KEYCHAIN_PASSWORD" "$KEYCHAIN_PATH"

        CERT_PATH="${RUNNER_TEMP:-/tmp}/certificate.p12"
        echo "$IOS_CERTIFICATE" | base64 -d > "$CERT_PATH"
        security import "$CERT_PATH" -P "$IOS_CERTIFICATE_PASSWORD" \
            -A -t cert -f pkcs12 -k "$KEYCHAIN_PATH"
        security set-key-partition-list -S apple-tool:,apple: \
            -k "$KEYCHAIN_PASSWORD" "$KEYCHAIN_PATH"
        security list-keychain -d user -s "$KEYCHAIN_PATH"

        PROVISION_PATH="${RUNNER_TEMP:-/tmp}/profile.mobileprovision"
        echo "$IOS_MOBILE_PROVISION" | base64 -d > "$PROVISION_PATH"
        mkdir -p ~/Library/MobileDevice/Provisioning\ Profiles/
        cp "$PROVISION_PATH" ~/Library/MobileDevice/Provisioning\ Profiles/

        echo "Certificate and provisioning profile installed."
    else
        echo "Manual signing configured for local development."
        echo "Import your .p12 certificate into Keychain Access manually."
        echo "Place your .mobileprovision in ~/Library/MobileDevice/Provisioning Profiles/"
    fi

    echo ""
    echo "Run: scripts/ios-build.sh"
}

# ── Main ──────────────────────────────────────────────────────────────────────

case "${1:-}" in
    --dev)    setup_dev ;;
    --auto)   setup_auto ;;
    --manual) setup_manual ;;
    *)        usage ;;
esac
