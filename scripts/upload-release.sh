#!/bin/sh
# CI only (release-files job): uploads dist/release/ to the project's generic package registry
# under names that carry the tag (natlas-v1.0.3.apk), which the release links in .gitlab-ci.yml
# point at, and writes release-notes.md from this version's CHANGELOG.md section.
set -eu
cd "$(dirname "$0")/.."
: "${CI_COMMIT_TAG:?run from a tag pipeline}" "${CI_JOB_TOKEN:?}" "${PKG_URL:?}"
V=$(tr -d ' \n\r' < VERSION)
T=$CI_COMMIT_TAG

echo "Uploading to the package registry ($T)"
for pair in "natlas-$V.apk:natlas-$T.apk" "SHA256SUMS:SHA256SUMS"; do
  f=${pair%%:*}; n=${pair#*:}
  code=$(curl --silent --show-error --retry 3 --output /dev/null --write-out '%{http_code}' \
    --header "JOB-TOKEN: $CI_JOB_TOKEN" --upload-file "dist/release/$f" "$PKG_URL/$n")
  case "$code" in
    2??) printf '✓ %-28s %6s\n' "$n" "$(du -h "dist/release/$f" | cut -f1)" ;;
    *) printf '✗ %s — HTTP %s (a file with this name may already exist: delete it in Deploy → Package registry)\n' "$n" "$code"; exit 1 ;;
  esac
done

# this version's section of CHANGELOG.md ("## X.Y.Z - date")
awk -v v="$V" '/^## /{p = index($0 " ", "## " v " ") == 1; next} p' CHANGELOG.md > release-notes.md
[ -s release-notes.md ] || echo "Natlas $V" > release-notes.md
echo "✓ release notes from CHANGELOG.md ($(grep -c . release-notes.md) lines)"
