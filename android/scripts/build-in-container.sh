#!/usr/bin/env bash
# Runs inside the builder container (see build.sh):
# Go checks → engine for each phone CPU → Android unit tests → APK in android/dist/.
# NATLAS_QUICK=1 skips the checks; NATLAS_ABIS picks the phone CPUs (default: all three).
set -euo pipefail
MODE="${1:-release}"
QUICK="${NATLAS_QUICK:-0}"
ABIS="${NATLAS_ABIS:-arm64-v8a armeabi-v7a x86_64}"
cd /src
took() { echo "   ($((SECONDS - T0))s)"; T0=$SECONDS; } # time of the step just finished
T0=$SECONDS

VERSION=$(tr -d ' \r\n' < VERSION)
[ -n "$VERSION" ] || { echo "can't read the version from VERSION" >&2; exit 1; }
echo "== natlas $VERSION ($MODE) =="

if [ "$QUICK" = 1 ]; then
  echo "== quick build: Go and Android tests skipped =="
else
  echo "== Go: vet + tests =="
  go vet ./...
  go test ./...
  took
fi

echo "== engine (libnatlas.so per CPU) =="
LIBS=android/app/src/main/jniLibs
rm -rf "$LIBS"
engine() { # engine <abi> <env…>
  local abi=$1; shift
  mkdir -p "$LIBS/$abi"
  env CGO_ENABLED=0 "$@" go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$LIBS/$abi/libnatlas.so" ./cmd/natlas
  echo "  $abi  $(du -h "$LIBS/$abi/libnatlas.so" | cut -f1)"
}
for abi in $ABIS; do
  case "$abi" in
    arm64-v8a)   engine arm64-v8a   GOOS=android GOARCH=arm64 ;;       # almost every phone
    armeabi-v7a) engine armeabi-v7a GOOS=linux   GOARCH=arm GOARM=7 ;; # older 32-bit phones (static binary)
    x86_64)      engine x86_64      GOOS=linux   GOARCH=amd64 ;;       # emulators, Chromebooks
    *) echo "unknown ABI in NATLAS_ABIS: $abi" >&2; exit 2 ;;
  esac
done
took

echo "== APK =="
cd android
chmod +x gradlew
GRADLE=(./gradlew --no-daemon --console=plain --build-cache --parallel -PnatlasVersion="$VERSION")
if [ "$MODE" = "debug" ]; then
  if [ "$QUICK" = 1 ]; then "${GRADLE[@]}" assembleDebug; else "${GRADLE[@]}" testDebugUnitTest assembleDebug; fi
  APK=app/build/outputs/apk/debug/app-debug.apk
  OUT="dist/natlas-$VERSION-debug.apk"
else
  if [ "$QUICK" = 1 ]; then "${GRADLE[@]}" assembleRelease; else "${GRADLE[@]}" testReleaseUnitTest assembleRelease; fi
  APK=app/build/outputs/apk/release/app-release.apk
  OUT="dist/natlas-$VERSION.apk"
  apksigner verify --print-certs "$APK" | sed -n '1,2p'
fi
mkdir -p dist
cp "$APK" "$OUT"
took
echo "== done: android/$OUT ($(du -h "$OUT" | cut -f1)) in ${SECONDS}s =="
