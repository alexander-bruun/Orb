#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

usage() {
    cat <<'EOF'
Usage:
  scripts/android-ci-secrets.sh --create [options]
  scripts/android-ci-secrets.sh --existing --keystore <path> [options]

Modes:
  --create                 Create a new JKS keystore and derive CI secret values.
  --existing               Use an existing JKS keystore and derive CI secret values.

Required with --existing:
  --keystore <path>        Path to an existing .jks file.

Options:
  --alias <name>           Key alias. Default: orb-release
  --password <value>       Keystore password (also used as key password).
  --out-keystore <path>    Output path for --create. Default: <repo>/release-keystore.jks
  --set-gh-secrets         Set GitHub secrets with gh CLI instead of only printing.
  --repo <owner/name>      Optional repo for gh secret set (defaults to current repo).
  --help                   Show this help.

Secrets produced:
  ANDROID_KEYSTORE_B64
  ANDROID_KEY_ALIAS
  ANDROID_KEYSTORE_PASSWORD

Examples:
  scripts/android-ci-secrets.sh --create --alias orb-release --password 'strong-pass'
  scripts/android-ci-secrets.sh --existing --keystore ./release-keystore.jks --alias orb-release --password 'strong-pass' --set-gh-secrets
EOF
}

mode=""
keystore_path=""
out_keystore="$PROJECT_ROOT/release-keystore.jks"
key_alias="orb-release"
keystore_password=""
set_gh_secrets="false"
gh_repo=""

while [ $# -gt 0 ]; do
    case "$1" in
        --create)
            mode="create"
            shift
            ;;
        --existing)
            mode="existing"
            shift
            ;;
        --keystore)
            keystore_path="${2:-}"
            shift 2
            ;;
        --out-keystore)
            out_keystore="${2:-}"
            shift 2
            ;;
        --alias)
            key_alias="${2:-}"
            shift 2
            ;;
        --password)
            keystore_password="${2:-}"
            shift 2
            ;;
        --set-gh-secrets)
            set_gh_secrets="true"
            shift
            ;;
        --repo)
            gh_repo="${2:-}"
            shift 2
            ;;
        --help|-h)
            usage
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage
            exit 1
            ;;
    esac
done

if [ -z "$mode" ]; then
    echo "Error: choose either --create or --existing." >&2
    usage
    exit 1
fi

if [ -z "$keystore_password" ]; then
    read -rsp "Keystore password (used for store and key): " keystore_password
    echo
fi

if [ "$mode" = "create" ]; then
    keystore_path="$out_keystore"
    if [ -f "$keystore_path" ]; then
        echo "Error: keystore already exists: $keystore_path" >&2
        echo "Use --existing to reuse it, or pass --out-keystore with another path." >&2
        exit 1
    fi
    mkdir -p "$(dirname "$keystore_path")"
    keytool -genkeypair -v -storetype JKS \
        -keystore "$keystore_path" \
        -storepass "$keystore_password" \
        -keypass "$keystore_password" \
        -alias "$key_alias" \
        -dname "CN=Orb Release, OU=Mobile, O=Orb, L=Copenhagen, S=Capital Region, C=DK" \
        -keyalg RSA -keysize 2048 -validity 10000
else
    if [ -z "$keystore_path" ]; then
        echo "Error: --keystore is required with --existing." >&2
        exit 1
    fi
    if [ ! -f "$keystore_path" ]; then
        echo "Error: keystore file not found: $keystore_path" >&2
        exit 1
    fi
fi

if base64 --help 2>/dev/null | rg -q -- "-w"; then
    android_keystore_b64="$(base64 -w 0 "$keystore_path")"
else
    android_keystore_b64="$(base64 "$keystore_path" | tr -d '\n')"
fi

if [ "$set_gh_secrets" = "true" ]; then
    if ! command -v gh >/dev/null 2>&1; then
        echo "Error: gh CLI is required for --set-gh-secrets." >&2
        exit 1
    fi

    repo_args=()
    if [ -n "$gh_repo" ]; then
        repo_args=(--repo "$gh_repo")
    fi

    gh secret set ANDROID_KEY_ALIAS "${repo_args[@]}" --body "$key_alias"
    gh secret set ANDROID_KEYSTORE_PASSWORD "${repo_args[@]}" --body "$keystore_password"
    gh secret set ANDROID_KEYSTORE_B64 "${repo_args[@]}" --body "$android_keystore_b64"
    echo "GitHub secrets updated."
fi

echo "Keystore path: $keystore_path"
echo ""
echo "Paste these into GitHub secrets if not using --set-gh-secrets:"
echo "ANDROID_KEY_ALIAS=$key_alias"
echo "ANDROID_KEYSTORE_PASSWORD=$keystore_password"
echo "ANDROID_KEYSTORE_B64=$android_keystore_b64"
