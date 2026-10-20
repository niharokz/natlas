#!/usr/bin/env bash
# Builds the Natlas Android app with Docker - nothing else to install.
#
#   ./android/build.sh keystore   create your signing key (once; keep it backed up)
#                                 in $KEYS_HOME/.natlas-keys (KEYS_HOME defaults to ~/.local/share/keys)
#   ./android/build.sh            signed release APK  → android/dist/natlas-<version>.apk
#   ./android/build.sh debug      debug APK (no key)  → android/dist/natlas-<version>-debug.apk
#   ./android/build.sh quick      signed release APK, no tests, 64-bit phones only (fastest)
#
# Settings come from android/.env (copy android/.env.example).
set -euo pipefail
cd "$(dirname "$0")/.."                       # repo root

ENV_FILE=android/.env
if [ -f "$ENV_FILE" ]; then set -a; . "$ENV_FILE"; set +a; fi
# Signing keys live with the other apps' keys: $KEYS_HOME/.natlas-keys
KEYS_HOME="${KEYS_HOME:-${XDG_DATA_HOME:-$HOME/.local/share}/keys}"
KEYS_DIR="${NATLAS_KEYSTORE_DIR:-$KEYS_HOME/.natlas-keys}"
[ -f "$KEYS_DIR/natlas.jks" ] || [ ! -f "$HOME/.natlas-keys/natlas.jks" ] || {
  echo "Your key is still in ~/.natlas-keys. Move it: mkdir -p \"$KEYS_HOME\" && mv ~/.natlas-keys \"$KEYS_DIR\""; exit 1; }
KEY_FILE="$KEYS_DIR/natlas.jks"
IMAGE=natlas-android-builder

mkdir -p android/.cache/home android/dist "$KEYS_DIR"
echo "== builder image =="
# The Dockerfile copies nothing, so it is sent alone (no build context): sending
# android/ would upload the multi-GB caches in android/.cache on every run.
docker build -t "$IMAGE" - < android/Dockerfile

# Runs a command in the builder as you (files stay yours), with caches kept in android/.cache.
run() {
  docker run --rm -t --user "$(id -u):$(id -g)" \
    -v "$PWD":/src -v "$KEYS_DIR":/keys -w /src \
    -e HOME=/src/android/.cache/home \
    -e JAVA_TOOL_OPTIONS=-Duser.home=/src/android/.cache/home \
    -e GRADLE_USER_HOME=/src/android/.cache/gradle \
    -e ANDROID_USER_HOME=/src/android/.cache/android \
    -e GOPATH=/src/android/.cache/go -e GOCACHE=/src/android/.cache/go-build -e GOFLAGS=-mod=vendor \
    -e NATLAS_KEYSTORE_PASSWORD -e NATLAS_KEY_ALIAS -e NATLAS_KEY_PASSWORD \
    "$@"
}

case "${1:-release}" in
  keystore)
    [ -f "$KEY_FILE" ] && { echo "A key already exists at $KEY_FILE - keep using it (updates must be signed with the same key)."; exit 1; }
    [ -n "${NATLAS_KEYSTORE_PASSWORD:-}" ] || { echo "Set NATLAS_KEYSTORE_PASSWORD in $ENV_FILE first."; exit 1; }
    run "$IMAGE" keytool -genkeypair -v -keystore /keys/natlas.jks -alias "${NATLAS_KEY_ALIAS:-natlas}" \
      -keyalg RSA -keysize 4096 -validity 36500 -dname "CN=natlas" \
      -storepass:env NATLAS_KEYSTORE_PASSWORD -keypass:env NATLAS_KEYSTORE_PASSWORD
    echo "Key created: $KEY_FILE - back it up (e.g. into your password manager)."
    ;;
  release)
    [ -f "$KEY_FILE" ] || { echo "No signing key yet. Run: ./android/build.sh keystore"; exit 1; }
    run -e NATLAS_KEYSTORE=/keys/natlas.jks -e NATLAS_QUICK -e NATLAS_ABIS "$IMAGE" bash android/scripts/build-in-container.sh release
    ;;
  debug)
    run -e NATLAS_QUICK -e NATLAS_ABIS "$IMAGE" bash android/scripts/build-in-container.sh debug
    ;;
  quick)
    [ -f "$KEY_FILE" ] || { echo "No signing key yet. Run: ./android/build.sh keystore"; exit 1; }
    run -e NATLAS_KEYSTORE=/keys/natlas.jks -e NATLAS_QUICK=1 -e NATLAS_ABIS="${NATLAS_ABIS:-arm64-v8a}" "$IMAGE" bash android/scripts/build-in-container.sh release
    ;;
  *)
    echo "usage: $0 [keystore|release|debug|quick]"; exit 2 ;;
esac
