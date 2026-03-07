#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
WEB_DIR="$PROJECT_ROOT/web"
ANDROID_GEN="$WEB_DIR/src-tauri/gen/android"

if [ -f "$SCRIPT_DIR/android.env" ]; then
    . "$SCRIPT_DIR/android.env"
fi

KEYSTORE_PROPS="$ANDROID_GEN/keystore.properties"

usage() {
    cat <<EOF
Usage: $0 [--dev | --generate | --configure]

  --dev        Set up dev signing (auto-generated keystore, no prompts)
  --generate   Generate a new release keystore (interactive)
  --configure  Write keystore.properties for Gradle signing

Environment variables (for --configure):
  KEYSTORE_FILE    Path to keystore (default: \$PROJECT_ROOT/release-keystore.jks)
  KEY_ALIAS        Key alias (default: upload)
  KEY_PASSWORD     Key password (prompted if empty)
  STORE_PASSWORD   Store password (prompted if empty)

After configuring, run scripts/android-build.sh to produce signed artifacts.
EOF
    exit 1
}

# ── Patch build.gradle.kts with signing config ────────────────────────────────

patch_gradle() {
    local BUILD_GRADLE="$ANDROID_GEN/app/build.gradle.kts"
    if [ ! -f "$BUILD_GRADLE" ]; then
        echo "build.gradle.kts not found — run 'make tauri-android-init' first."
        return
    fi
    if grep -q "signingConfigs" "$BUILD_GRADLE"; then
        return
    fi
    echo "Patching build.gradle.kts with signing config..."

    # Add import if missing
    if ! grep -q "java.io.FileInputStream" "$BUILD_GRADLE"; then
        sed -i '1s/^/import java.io.FileInputStream\n/' "$BUILD_GRADLE"
    fi

    # Insert signingConfigs block before buildTypes
    sed -i '/^[[:space:]]*buildTypes/i \
    signingConfigs {\
        create("release") {\
            val keystorePropertiesFile = rootProject.file("keystore.properties")\
            val keystoreProperties = Properties()\
            if (keystorePropertiesFile.exists()) {\
                keystoreProperties.load(FileInputStream(keystorePropertiesFile))\
            }\
            keyAlias = keystoreProperties["keyAlias"] as String\
            keyPassword = keystoreProperties["password"] as String\
            storeFile = file(keystoreProperties["storeFile"] as String)\
            storePassword = keystoreProperties["password"] as String\
        }\
    }' "$BUILD_GRADLE"

    # Wire signing into the release build type
    sed -i 's/getByName("release") {/getByName("release") {\n            signingConfig = signingConfigs.getByName("release")/' "$BUILD_GRADLE"

    # Allow cleartext HTTP in all build types (Orb connects to local servers over HTTP)
    sed -i 's/manifestPlaceholders\["usesCleartextTraffic"\] = "false"/manifestPlaceholders["usesCleartextTraffic"] = "true"/' "$BUILD_GRADLE"
}

# ── Dev signing (non-interactive) ─────────────────────────────────────────────

setup_dev() {
    local DEV_KEYSTORE="$SCRIPT_DIR/dev-keystore.jks"

    if [ ! -f "$DEV_KEYSTORE" ]; then
        echo "Generating dev keystore..."
        keytool -genkeypair -v -storetype JKS \
            -keystore "$DEV_KEYSTORE" \
            -storepass android -keypass android \
            -alias dev \
            -dname "CN=Orb Dev, OU=Dev, O=Orb, L=Earth, S=Earth, C=US" \
            -keyalg RSA -keysize 2048 -validity 10000
    fi

    mkdir -p "$ANDROID_GEN"
    cat > "$KEYSTORE_PROPS" <<EOF
keyAlias=dev
password=android
storeFile=$DEV_KEYSTORE
EOF

    patch_gradle
    echo "Dev signing configured."
}

# ── Generate release keystore (interactive) ───────────────────────────────────

generate_keystore() {
    local KEYSTORE_FILE="${KEYSTORE_FILE:-$PROJECT_ROOT/release-keystore.jks}"
    local KEY_ALIAS="${KEY_ALIAS:-upload}"

    if [ -f "$KEYSTORE_FILE" ]; then
        echo "Keystore already exists at: $KEYSTORE_FILE"
        read -rp "Overwrite? [y/N] " answer
        [ "$answer" != "y" ] && [ "$answer" != "Y" ] && exit 0
    fi

    echo "Generating release keystore..."
    keytool -genkey -v \
        -keystore "$KEYSTORE_FILE" \
        -keyalg RSA \
        -keysize 2048 \
        -validity 10000 \
        -alias "$KEY_ALIAS"

    echo ""
    echo "Keystore created at: $KEYSTORE_FILE"
    echo "IMPORTANT: Keep this file safe and never commit it to source control."
    echo ""
    echo "To export for CI, run:"
    echo "  base64 -w0 $KEYSTORE_FILE"
}

# ── Configure Gradle signing (release) ───────────────────────────────────────

configure_signing() {
    local KEYSTORE_FILE="${KEYSTORE_FILE:-$PROJECT_ROOT/release-keystore.jks}"
    local KEY_ALIAS="${KEY_ALIAS:-upload}"
    local KEY_PASSWORD="${KEY_PASSWORD:-}"
    local STORE_PASSWORD="${STORE_PASSWORD:-}"

    if [ ! -f "$KEYSTORE_FILE" ]; then
        echo "ERROR: Keystore not found at $KEYSTORE_FILE"
        echo "Run '$0 --generate' first, or set KEYSTORE_FILE."
        exit 1
    fi

    if [ -z "$KEY_PASSWORD" ]; then
        read -rsp "Key password: " KEY_PASSWORD
        echo
    fi
    if [ -z "$STORE_PASSWORD" ]; then
        STORE_PASSWORD="$KEY_PASSWORD"
    fi

    if [ ! -d "$ANDROID_GEN" ]; then
        echo "ERROR: $ANDROID_GEN does not exist. Run 'make tauri-android-init' first."
        exit 1
    fi

    cat > "$KEYSTORE_PROPS" <<EOF
keyAlias=$KEY_ALIAS
password=$KEY_PASSWORD
storeFile=$KEYSTORE_FILE
EOF

    patch_gradle
    echo "Release signing configured. Run: make tauri-android-build"
}

# ── Main ──────────────────────────────────────────────────────────────────────

case "${1:-}" in
    --dev)       setup_dev ;;
    --generate)  generate_keystore ;;
    --configure) configure_signing ;;
    *)           usage ;;
esac
