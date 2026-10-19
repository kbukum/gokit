#!/usr/bin/env bash
# Builds and vets a consumer Go module against this gokit checkout.
# usage: scripts/consumer-check.sh <consumer-module-dir>
set -euo pipefail

consumer=${1:?usage: consumer-check.sh <consumer-module-dir>}
gokit=$(cd "$(dirname "$0")/.." && pwd)
consumer=$(cd "$consumer" && pwd)
export GOWORK=off GOFLAGS=-mod=readonly

resolved=$(go -C "$consumer" list -m -f '{{if .Replace}}{{.Replace.Dir}}{{end}}' github.com/kbukum/gokit)
if [[ -z $resolved ]]; then
  echo "consumer-check: $consumer does not replace github.com/kbukum/gokit with a local checkout" >&2
  exit 1
fi
resolved=$(cd "$consumer" && cd "$resolved" && pwd)
if [[ $resolved != "$gokit" ]]; then
  echo "consumer-check: $consumer uses gokit at $resolved, not $gokit" >&2
  exit 1
fi

go -C "$consumer" build ./...
go -C "$consumer" vet ./...
echo "consumer-check: $consumer builds against $gokit"
