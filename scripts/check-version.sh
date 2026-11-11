#!/bin/sh
# Checks that VERSION is X.Y.Z and CHANGELOG.md has a "## X.Y.Z - date" section; with a tag
# argument (v1.2.3) also that the tag matches.   sh scripts/check-version.sh [vX.Y.Z]
set -eu
cd "$(dirname "$0")/.."
v=$(tr -d ' \n\r' < VERSION)
fail=0
echo "$v" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || { echo "✗ VERSION '$v' is not X.Y.Z"; exit 1; }
grep -q "^## $v " CHANGELOG.md || { echo "✗ CHANGELOG.md has no '## $v - date' section"; fail=1; }
if [ -n "${1:-}" ] && [ "$1" != "v$v" ]; then echo "✗ tag $1 does not match VERSION $v"; fail=1; fi
[ "$fail" -eq 0 ] && echo "✓ version $v"
exit "$fail"
