#!/usr/bin/env bash
# Fail if any dependency ships under a license outside the allow-list.
# Runs go-licenses per module (modules are discovered, never hardcoded) and
# ignores first-party github.com/kbukum/gokit packages, which resolve through
# local `replace` directives and carry the repo's own license.
set -euo pipefail

# Run from the repo root regardless of the caller's cwd, so module discovery
# and the per-module `cd` below resolve against the checkout, not $PWD.
cd "$(dirname "${BASH_SOURCE[0]}")/.."

# Permissive plus weak (file-level) copyleft licenses accepted for
# redistribution: MPL-2.0 is weak copyleft and is allowed. Strong copyleft
# (GPL/LGPL/AGPL) and unknown licenses are rejected; add a new SPDX id here only
# with maintainer sign-off recorded in docs/dependencies.md.
ALLOWED="Apache-2.0,MIT,BSD-2-Clause,BSD-3-Clause,BSD-3-Clause-Clear,ISC,MPL-2.0,Unlicense,Zlib,Python-2.0"
IGNORE="github.com/kbukum/gokit"

modules=$(find . -name go.mod -not -path '*/vendor/*' -not -path '*/.git/*' \
  | sed 's|/go.mod||' | sed 's|^\./||' | sed 's|^$|.|' | sort)

failed=0
for mod in $modules; do
  echo "::group::Licenses $mod"
  if ! (cd "$mod" && go-licenses check ./... --allowed_licenses="$ALLOWED" --ignore "$IGNORE"); then
    echo "::error::License check failed for module: $mod"
    failed=1
  fi
  echo "::endgroup::"
done

if [ "$failed" -eq 0 ]; then
  echo "License allow-list check passed for all modules."
fi
exit $failed
