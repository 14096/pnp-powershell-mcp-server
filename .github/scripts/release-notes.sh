#!/usr/bin/env bash
# Usage: release-notes.sh <tag> -> prints the CHANGELOG.md section for that tag.
set -euo pipefail
version="${1#v}"
notes="$(awk -v v="$version" '
  /^## \[/ { if (found) exit; if (index($0, "## [" v "]") == 1) { found = 1; next } }
  found { print }
' CHANGELOG.md)"
if [ -z "$(echo "$notes" | tr -d '[:space:]')" ]; then
  echo "No CHANGELOG.md entry found for version $version" >&2
  exit 1
fi
echo "$notes"
