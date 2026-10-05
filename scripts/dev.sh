#!/bin/sh
# Run Natlas locally against a throwaway copy of the example data:
#   sh scripts/dev.sh      then open http://localhost:8080  (login: demo / demo)
# The web files are served from disk, so UI edits show on reload.
set -eu
cd "$(dirname "$0")/.."
tmp="${TMPDIR:-/tmp}/natlas-dev"
rm -rf "$tmp" && mkdir -p "$tmp/data"
cp examples/data/* "$tmp/data/"
sed "s#^data_dir:.*#data_dir: $tmp/data#" natlas.example.yml > "$tmp/natlas.yml"

NATLAS_USERNAME=demo NATLAS_PASSWORD=demo \
NATLAS_SECRET_KEY=dev-only-secret-key-not-for-real-use-000 \
NATLAS_CONFIG="$tmp/natlas.yml" NATLAS_PLUGINS_DIR=plugins NATLAS_WEB_DIR=web \
NATLAS_COOKIE_SECURE=false NATLAS_ADDR=127.0.0.1:8080 \
exec go run -mod=vendor ./cmd/natlas
