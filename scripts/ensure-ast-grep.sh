#!/usr/bin/env bash
# Ensure ast-grep is available, installing it if missing (local dev + CI).
#
# ast-grep powers the advisory `make structure` guard. This installs a pinned
# version via the first available package manager that places the binary on PATH.
# Override the version with AST_GREP_VERSION.
set -euo pipefail

version="${AST_GREP_VERSION:-0.44.1}"

if command -v ast-grep >/dev/null 2>&1; then
  exit 0
fi

echo "ensure-ast-grep: ast-grep not found — installing ${version}..." >&2

attempt() {
  local label="$1"
  shift
  echo "ensure-ast-grep: trying ${label}..." >&2
  if "$@" && command -v ast-grep >/dev/null 2>&1; then
    echo "ensure-ast-grep: installed via ${label} ($(ast-grep --version))" >&2
    return 0
  fi
  return 1
}

if command -v brew >/dev/null 2>&1; then
  attempt "brew" brew install ast-grep && exit 0
fi
if command -v npm >/dev/null 2>&1; then
  attempt "npm" npm install -g "@ast-grep/cli@${version}" && exit 0
fi
if command -v cargo >/dev/null 2>&1; then
  attempt "cargo" cargo install ast-grep --locked --version "${version}" && exit 0
fi
if command -v pipx >/dev/null 2>&1; then
  attempt "pipx" pipx install "ast-grep-cli==${version}" && exit 0
fi

cat >&2 <<'EOF'
ensure-ast-grep: could not install ast-grep automatically.
  Install one of the following, then re-run:
    brew install ast-grep
    npm install -g @ast-grep/cli
    cargo install ast-grep --locked
    pipx install ast-grep-cli
EOF
exit 1
