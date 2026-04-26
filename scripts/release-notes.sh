#!/usr/bin/env bash
# scripts/release-notes.sh — extract a CHANGELOG section for `gh release create`.
#
# Usage:
#   scripts/release-notes.sh v0.2.0 > /tmp/notes.md
#   gh release create v0.2.0 --notes-file /tmp/notes.md
set -euo pipefail

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  echo "usage: $0 vX.Y.Z" >&2
  exit 1
fi

NOVER="${VERSION#v}"
CHANGELOG="$(dirname "$0")/../CHANGELOG.md"

if ! grep -qE "^## \[${NOVER}\]" "$CHANGELOG"; then
  echo "error: CHANGELOG.md has no '## [${NOVER}]' section" >&2
  exit 1
fi

# Find previous tag for the diff link.
PREV=$(git tag --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | grep -v "^${VERSION}$" | head -1 || true)

cat <<EOF
## ${VERSION}

EOF

awk -v ver="$NOVER" '
  $0 ~ "^## \\[" ver "\\]" {flag=1; next}
  flag && /^## \[/ {flag=0}
  flag {print}
' "$CHANGELOG"

if [ -n "$PREV" ]; then
  echo
  echo "**Full Changelog**: https://github.com/kbukum/gokit/compare/${PREV}...${VERSION}"
fi
