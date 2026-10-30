#!/usr/bin/env bash
# Releases Natlas: version, changelog, commit, tag, push. GitLab CI then builds the APK and the release.
#
#   sh scripts/release.sh "v1.0.2 demo login on the sign-in screen"
#
# The first word is the version tag (vX.Y.Z); the whole text is the commit and tag message.
# It sets VERSION, adds a CHANGELOG section if this version has none (edit it first for real notes),
# runs the tests (if Go is installed) and the public-repo check, commits everything, tags and pushes.
[ -n "${BASH_VERSION:-}" ] || exec bash "$0" "$@"
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

msg="${1:-}"
tag="${msg%% *}"
[[ "$tag" =~ ^v([0-9]+\.[0-9]+\.[0-9]+)$ ]] || { echo 'usage: scripts/release.sh "vX.Y.Z what changed"'; exit 2; }
version="${BASH_REMATCH[1]}"
note="${msg#"$tag"}"; note="${note# }"
[ -n "$note" ] || note="Natlas $version"

branch=$(git rev-parse --abbrev-ref HEAD)
[ "$branch" = main ] || { echo "on branch $branch - releases are made from main"; exit 1; }

git fetch -q --tags origin
if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
  echo "$tag already exists - pick the next version (or delete the tag if that release failed: git tag -d $tag && git push origin :refs/tags/$tag)"
  exit 1
fi
behind=$(git rev-list --count HEAD..origin/main 2>/dev/null || echo 0)
[ "$behind" = 0 ] || { echo "origin/main has $behind commit(s) you don't have - git pull first"; exit 1; }

echo "== $tag: $note =="

# version and changelog
echo "$version" > VERSION
if ! grep -q "^## $version " CHANGELOG.md; then
  awk -v h="## $version - $(date +%F)" -v n="- $note" '
    !done && /^## / { print h; print ""; print n; print ""; done = 1 }
    { print }' CHANGELOG.md > CHANGELOG.md.tmp && mv CHANGELOG.md.tmp CHANGELOG.md
  echo "added a CHANGELOG section for $version"
fi

# checks
if command -v go >/dev/null; then
  echo "== tests =="
  go vet ./... && go test ./... | grep -v "no test files"
else
  echo "(Go not installed here - CI runs the tests)"
fi
echo "== public-repo check =="
git add -A
sh scripts/check-public.sh

# commit, tag, push
git diff --cached --quiet || git commit -q -m "$tag $note"
git tag -a "$tag" -m "$note"
git push -q origin main
git push -q origin "$tag"

url=$( (git remote get-url --push --all origin | grep -m1 gitlab || git remote get-url origin) | sed -E 's#^git@([^:]+):#https://\1/#; s#\.git$##')
echo "== pushed $tag =="
echo "pipeline: $url/-/pipelines"
echo "release:  $url/-/releases (the APK appears when the pipeline is done)"
