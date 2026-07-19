#!/usr/bin/env bash
# Structure guard (development principles §4): the package aggregator is declare-only.
#
#   1. `doc.go` is docs-only (ADVISORY): it carries the package clause and comments only —
#      no `func`/`type`/`var`/`const` declarations or imports. Package documentation
#      belongs here; code belongs in concern-named sibling files. Reported by the
#      ast-grep rule scripts/sg-rules/declare-only-aggregator.yml.
#   2. God-file (ADVISORY): a package whose non-test code is piled into a single oversized
#      file is a refactor signal — split it by concern into named sibling files. Reported
#      as a warning, never a hard failure (small single-file packages are legitimate).
#   3. Crowded package (ADVISORY): a package directory that has accumulated many non-test
#      files is a prompt to consider whether separable groups belong in concern-named
#      sub-packages (sub-folder + declare-only doc.go). File count alone is not a verdict —
#      a flat set of files that all serve one concern is legitimate; this only surfaces the
#      candidate. The CROWDED_PKG_FILES default (15) is a deliberately conservative backstop
#      above the "roughly >5-10" judgment range in the docs, to avoid flagging legitimate
#      packages; author/reviewer judgment stays the primary signal. Reported as a warning,
#      never a hard failure.
#
# Both checks are advisory (never gating). Vendored, testdata, and node_modules trees are skipped.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

god_file_lines="${GOD_FILE_LINES:-600}"
crowded_pkg_files="${CROWDED_PKG_FILES:-15}"

# ast-grep may be installed into a user-writable prefix by ensure-ast-grep.sh;
# expose those bin dirs so a freshly installed binary resolves in this process too.
export PATH="${NPM_CONFIG_PREFIX:-$HOME/.local}/bin:$HOME/.local/bin:$HOME/.cargo/bin:$PATH"

# 1. doc.go docs-only (advisory) — AST match via ast-grep (sgconfig.yml).
# Skip the same trees as the god-file check below so behavior matches this header.
if "$repo_root/scripts/ensure-ast-grep.sh"; then
  sg_bin="$(command -v ast-grep >/dev/null 2>&1 && echo ast-grep || echo sg)"
  if command -v "$sg_bin" >/dev/null 2>&1; then
    "$sg_bin" scan \
      --globs '!**/vendor/**' --globs '!**/testdata/**' --globs '!**/node_modules/**' \
      || echo "structure: doc.go docs-only scan reported findings or errored above (advisory, not gating)" >&2
  else
    echo "structure: skipping doc.go docs-only check (ast-grep unresolved after install)" >&2
  fi
else
  echo "structure: skipping doc.go docs-only check (ast-grep unavailable)" >&2
fi

# 2. God-file (advisory): a package with a single non-test .go file (excluding doc.go)
# larger than the threshold.
while IFS= read -r dir; do
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
done < <(find . \
  \( -path '*/vendor' -o -path '*/testdata' -o -path '*/node_modules' -o -path '*/.*' \) -prune \
  -o -type d -print | sort)

# 3. Crowded package (advisory): a package directory with many non-test .go files (excluding
# doc.go) is a candidate for grouping separable concerns into sub-packages. File count alone is
# not a verdict; this only surfaces the candidate to judge.
while IFS= read -r dir; do
  count=0
  while IFS= read -r _; do
    count=$((count + 1))
  done < <(find "$dir" -maxdepth 1 -name '*.go' -not -name '*_test.go' \
    -not -name 'doc.go' -type f)
  if [ "$count" -gt "$crowded_pkg_files" ]; then
    printf 'warning: crowded package (%s non-test files) — consider grouping separable concerns into sub-packages: %s\n' \
      "$count" "$dir" >&2
  fi
done < <(find . \
  \( -path '*/vendor' -o -path '*/testdata' -o -path '*/node_modules' -o -path '*/.*' \) -prune \
  -o -type d -print | sort)

exit 0
