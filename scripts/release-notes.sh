#!/usr/bin/env bash

set -euo pipefail

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  echo "usage: $0 vX.Y.Z" >&2
  exit 1
fi

if [[ ! "$VERSION" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$ ]]; then
  echo "error: invalid semantic version: $VERSION" >&2
  exit 1
fi
prerelease=${BASH_REMATCH[5]:-}
if [[ -n "$prerelease" ]]; then
  IFS='.' read -r -a identifiers <<<"$prerelease"
  for identifier in "${identifiers[@]}"; do
    if [[ "$identifier" =~ ^[0-9]+$ && ${#identifier} -gt 1 && "$identifier" == 0* ]]; then
      echo "error: invalid semantic version: $VERSION" >&2
      exit 1
    fi
  done
fi

NOVER="${VERSION#v}"
CHANGELOG="$(dirname "$0")/../CHANGELOG.md"
HEADING_PREFIX="## [${NOVER}] - "

if ! awk -v prefix="$HEADING_PREFIX" '
  index($0, prefix) == 1 { found = 1 }
  END { exit found ? 0 : 1 }
' "$CHANGELOG"; then
  echo "error: CHANGELOG.md has no '## [${NOVER}]' section" >&2
  exit 1
fi

PREV=$(git tag --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+([-.][a-zA-Z0-9.]+)?$' | grep -v "^${VERSION}$" | head -1 || true)

cat <<EOF
## ${VERSION}

EOF

awk -v prefix="$HEADING_PREFIX" '
  index($0, prefix) == 1 {flag=1; next}
  flag && /^## \[/ {flag=0}
  flag {print}
' "$CHANGELOG"

if [ -n "$PREV" ]; then
  echo
  echo "**Full Changelog**: https://github.com/kbukum/gokit/compare/${PREV}...${VERSION}"
fi
