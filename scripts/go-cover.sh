#!/usr/bin/env bash
# Test one Go module under the race detector and write its coverage profile.
#
# Usage: scripts/go-cover.sh <module-root> <profile> [go test flags...]
#
# A module may list extra packages to credit in `<module-root>/.coverpkg`
# (whitespace separated), passed as `-coverpkg`. Driven per module by Toven's
# `coverage` task, so `make test-coverage` and CI measure the same way.
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: $0 <module-root> <profile> [go test flags...]" >&2
  exit 2
fi
root=$1
profile=$2
shift 2
# `go -C` changes directory, so pin the profile path first.
[[ "$profile" = /* ]] || profile="$PWD/$profile"

flags=(-race -shuffle=on -count=1 -timeout=20m -covermode=atomic)
if [[ -f "$root/.coverpkg" ]]; then
  pkgs=$(tr -s '[:space:]' ',' <"$root/.coverpkg" | sed 's/^,//; s/,$//')
  [[ -z "$pkgs" ]] || flags+=("-coverpkg=$pkgs")
fi

mkdir -p "$(dirname "$profile")"
exec go -C "$root" test "${flags[@]}" -coverprofile="$profile" "$@" ./...
