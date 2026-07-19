#!/usr/bin/env bash
# Structure guard (development principles §4): the package aggregator is declare-only.
#
#   1. `doc.go` is docs-only (ADVISORY): it carries the package clause and package
#      documentation comment only — no `func`/`method`/`type`/`var`/`const` declarations
#      and no imports. Code belongs in concern-named sibling files. Reported by the
#      ast-grep rule scripts/sg-rules/declare-only-aggregator.yml.
#   2. Large file (ADVISORY): any non-test file whose code-only line count (excluding
#      comments and blank lines) exceeds GOD_FILE_LINES is a prompt to check whether
#      distinct concerns are piled together and should be split into concern-named sibling
#      files (or a sub-package). Length alone is not the verdict — a cohesive single-concern
#      file is legitimate at any size; concern-mixing is the real signal. Reported as a
#      warning, never a hard failure.
#   3. Crowded package (ADVISORY): a package directory that has accumulated more than
#      CROWDED_PKG_FILES non-test files is a prompt to consider whether 2-3+ separable
#      concern groups belong in their own concern-named sub-packages (sub-folder +
#      declare-only doc.go). File count alone is not a verdict — a flat set of files that
#      all serve one concern is legitimate; this only surfaces the candidate to judge.
#      Reported as a warning, never a hard failure.
#
# All checks are advisory (never gating). Vendored, testdata, and node_modules trees are skipped.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

god_file_lines="${GOD_FILE_LINES:-350}"
crowded_pkg_files="${CROWDED_PKG_FILES:-10}"

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

package_counts_cmd() {
  find . \
    \( -path '*/vendor' -o -path '*/testdata' -o -path '*/node_modules' -o -path '*/.*' \) -prune \
    -o -type f -name '*.go' -not -name '*_test.go' -not -name 'doc.go' -print \
    | sort \
    | awk '
      {
        dir = $0
        sub("/[^/][^/]*$", "", dir)
        if (dir == $0) {
          dir = "."
        }
        count[dir]++
        file[dir] = $0
      }
      END {
        for (dir in count) {
          printf "%d\t%s\t%s\n", count[dir], dir, file[dir]
        }
      }
    ' \
    | sort -k2,2
}

# Stream all non-test, non-doc.go Go files (skips the same trees as above) so the
# checks scale to large workspaces without buffering the whole list in a variable.
go_files_cmd() {
  find . \
    \( -path '*/vendor' -o -path '*/testdata' -o -path '*/node_modules' -o -path '*/.*' \) -prune \
    -o -type f -name '*.go' -not -name '*_test.go' -not -name 'doc.go' -print \
    | sort
}

# 2. Large file (advisory): any non-test file whose code-only line count (excluding
#    comments and blank lines) exceeds the threshold is a concern-mixing candidate to judge.
#    Comment stripping is best-effort: "//" and "/*" inside string literals (e.g. a URL)
#    are treated as comment starts, so the count can slightly undercount such lines.
count_code_lines() {
  awk '
    BEGIN { inblock = 0; n = 0 }
    {
      line = $0
      if (inblock) {
        idx = index(line, "*/")
        if (idx == 0) { next }
        line = substr(line, idx + 2)
        inblock = 0
      }
      while ((s = index(line, "/*")) > 0) {
        rest = substr(line, s + 2)
        e = index(rest, "*/")
        if (e == 0) { line = substr(line, 1, s - 1); inblock = 1; break }
        line = substr(line, 1, s - 1) substr(rest, e + 2)
      }
      c = index(line, "//")
      if (c > 0) { line = substr(line, 1, c - 1) }
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", line)
      if (line != "") { n++ }
    }
    END { print n }
  ' "$1"
}

while IFS= read -r src; do
  [ -n "$src" ] || continue
  code_lines="$(count_code_lines "$src")"
  if [ "$code_lines" -gt "$god_file_lines" ]; then
    printf 'warning: large file (%s code lines) — check for mixed concerns to split into concern-named sibling files or a sub-package: %s\n' \
      "$code_lines" "$src" >&2
  fi
done < <(go_files_cmd)

# 3. Crowded package (advisory): a package directory with more than the threshold of non-test .go files (excluding doc.go) and 2-3+ separable concern groups is a candidate for grouping those groups into concern-named sub-packages. File count alone is not a verdict; this only surfaces the candidate to judge.
while IFS=$'\t' read -r count dir _; do
  if [ "$count" -gt "$crowded_pkg_files" ]; then
    printf 'warning: crowded package (%s non-test files) — if 2-3+ separable concern groups, consider grouping them into concern-named sub-packages: %s\n' \
      "$count" "$dir" >&2
  fi
done < <(package_counts_cmd)

exit 0
