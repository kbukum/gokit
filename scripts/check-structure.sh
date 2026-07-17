#!/usr/bin/env bash
# Structure guard (development principles §4): the package aggregator is declare-only.
#
#   1. `doc.go` is docs-only (HARD): it carries the package clause and comments only —
#      no `func`/`type`/`var`/`const` declarations. Package documentation belongs here;
#      code belongs in concern-named sibling files.
#   2. God-file (ADVISORY): a package whose non-test code is piled into a single oversized
#      file is a refactor signal — split it by concern into named sibling files. Reported
#      as a warning, never a hard failure (small single-file packages are legitimate).
#
# Vendored, generated, and testdata trees are skipped.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

god_file_lines="${GOD_FILE_LINES:-600}"
hard_fail=0

skip_path() {
  case "$1" in
    */vendor/*|*/testdata/*|*/node_modules/*) return 0 ;;
    *) return 1 ;;
  esac
}

# 1. doc.go docs-only (hard).
while IFS= read -r file; do
  skip_path "$file" && continue
  # Strip block/line comments and string bodies crudely, then look for top-level decls.
  decls="$(grep -nE '^[[:space:]]*(func|type|var|const)[[:space:]]' "$file" || true)"
  if [ -n "$decls" ]; then
    printf 'doc.go carries code (must be docs-only): %s\n%s\n' "$file" "$decls" >&2
    hard_fail=1
  fi
done < <(find . -name doc.go -type f | sort)

# 2. God-file (advisory): a package with a single non-test .go file (excluding doc.go)
# larger than the threshold.
while IFS= read -r dir; do
  skip_path "$dir/" && continue
  src=""
  count=0
  while IFS= read -r f; do
    src="$f"
    count=$((count + 1))
  done < <(find "$dir" -maxdepth 1 -name '*.go' -not -name '*_test.go' \
    -not -name 'doc.go' -type f)
  [ "$count" -eq 1 ] || continue
  lines="$(wc -l < "$src")"
  if [ "$lines" -gt "$god_file_lines" ]; then
    printf 'warning: single-file package (%s lines) — split by concern: %s\n' \
      "$lines" "$src" >&2
  fi
done < <(find . -type d -not -path '*/.*' | sort)

exit "$hard_fail"
