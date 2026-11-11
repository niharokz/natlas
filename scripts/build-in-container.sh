#!/usr/bin/env bash
# Builds the Natlas APK inside natlas-builder (docker/builder/Dockerfile). Run by gtlbrunner:
# the GitLab release job on a vX.Y.Z tag, or `./build.sh natlas` on the server. Never uploads.
#
#   bash scripts/build-in-container.sh ci      Go vet + tests, engine for 3 CPUs, Android tests, signed APK
#   bash scripts/build-in-container.sh quick   no tests, 64-bit phones only, signed APK
#   bash scripts/build-in-container.sh debug   no key needed: debug APK (installs next to the release app)
#
# BUILD_CACHE  folder for the Go, Gradle and Android caches (gtlbrunner: the volume natlas-build-cache)
# KEY_DIR      folder with natlas.jks + keystore.env (gtlbrunner: keys/natlas, read-only)
#              keystore.env: NATLAS_KEYSTORE_PASSWORD=…  [NATLAS_KEY_ALIAS=natlas]  [NATLAS_KEY_PASSWORD=…]
# Result: dist/release/natlas-<version>.apk + SHA256SUMS
set -euo pipefail
cd "$(dirname "$0")/.."
MODE=${1:-ci}
C=${BUILD_CACHE:-/build-cache}
KEY_DIR=${KEY_DIR:-/keys}
OUT=dist/release
export HOME=$C/home GRADLE_USER_HOME=$C/gradle ANDROID_USER_HOME=$C/android \
       GOPATH=$C/go GOCACHE=$C/go-build GOFLAGS=-mod=vendor JAVA_TOOL_OPTIONS=-Duser.home=$C/home
mkdir -p "$HOME" "$GRADLE_USER_HOME" "$ANDROID_USER_HOME" "$GOPATH" "$GOCACHE"
T0=$SECONDS
took() { printf '   (%ss)\n' $((SECONDS - T0)); T0=$SECONDS; }   # time of the step just finished

V=$(tr -d ' \r\n' < VERSION)
[[ "$V" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "VERSION '$V' is not X.Y.Z" >&2; exit 1; }
case "$MODE" in
  ci|release) TESTS=1 ABIS="arm64-v8a armeabi-v7a x86_64" ;;
  quick)      TESTS=0 ABIS="arm64-v8a" ;;
  debug)      TESTS=1 ABIS="arm64-v8a armeabi-v7a x86_64" ;;
  *) echo "usage: $0 [ci|quick|debug]" >&2; exit 2 ;;
esac
echo "== natlas $V ($MODE) =="

# signing key: only these three settings are read from keystore.env (no shell eval of the file)
if [ "$MODE" != debug ]; then
  [ -f "$KEY_DIR/natlas.jks" ] && [ -f "$KEY_DIR/keystore.env" ] ||
    { echo "no signing key: $KEY_DIR needs natlas.jks and keystore.env (gtlbrunner: keys/natlas/)" >&2; exit 1; }
  while IFS='=' read -r k v; do
    v=${v%$'\r'}; v=${v#[\"\']}; v=${v%[\"\']}
    case "$k" in NATLAS_KEYSTORE_PASSWORD|NATLAS_KEY_ALIAS|NATLAS_KEY_PASSWORD) export "$k=$v" ;; esac
  done < "$KEY_DIR/keystore.env"
  [ -n "${NATLAS_KEYSTORE_PASSWORD:-}" ] || { echo "keystore.env has no NATLAS_KEYSTORE_PASSWORD" >&2; exit 1; }
  export NATLAS_KEYSTORE=$KEY_DIR/natlas.jks
fi

if [ "$TESTS" = 1 ]; then
  echo "== Go: vet + tests =="
  go vet ./...
  go test ./... | grep -v 'no test files'
  took
fi

echo "== engine (libnatlas.so per CPU) =="
LIBS=android/app/src/main/jniLibs
rm -rf "$LIBS"
engine() { # engine <abi> <env…>
  local abi=$1; shift
  mkdir -p "$LIBS/$abi"
  env CGO_ENABLED=0 "$@" go build -trimpath -ldflags "-s -w -X main.version=$V" -o "$LIBS/$abi/libnatlas.so" ./cmd/natlas
  echo "  $abi  $(du -h "$LIBS/$abi/libnatlas.so" | cut -f1)"
}
for abi in $ABIS; do
  case "$abi" in
    arm64-v8a)   engine arm64-v8a   GOOS=android GOARCH=arm64 ;;       # almost every phone
    armeabi-v7a) engine armeabi-v7a GOOS=linux   GOARCH=arm GOARM=7 ;; # older 32-bit phones (static binary)
    x86_64)      engine x86_64      GOOS=linux   GOARCH=amd64 ;;       # emulators, Chromebooks
  esac
done
took

echo "== APK =="
chmod +x android/gradlew
G=(./gradlew --no-daemon --console=plain --build-cache --parallel -PnatlasVersion="$V")
if [ "$MODE" = debug ]; then
  (cd android && "${G[@]}" testDebugUnitTest assembleDebug)
  APK=android/app/build/outputs/apk/debug/app-debug.apk NAME=natlas-$V-debug.apk
elif [ "$TESTS" = 1 ]; then
  (cd android && "${G[@]}" testReleaseUnitTest assembleRelease)
  APK=android/app/build/outputs/apk/release/app-release.apk NAME=natlas-$V.apk
else
  (cd android && "${G[@]}" assembleRelease)
  APK=android/app/build/outputs/apk/release/app-release.apk NAME=natlas-$V.apk
fi
[ "$MODE" = debug ] || apksigner verify --print-certs "$APK" | sed -n '1,2p'
rm -rf "$OUT" && mkdir -p "$OUT"
cp "$APK" "$OUT/$NAME"
(cd "$OUT" && sha256sum ./*.apk | sed 's#\./##' > SHA256SUMS)
took
echo "== done: $OUT/$NAME ($(du -h "$OUT/$NAME" | cut -f1)) in ${SECONDS}s =="
