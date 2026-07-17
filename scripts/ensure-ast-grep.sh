#!/usr/bin/env bash
# Ensure ast-grep is available, installing it if missing (local dev + CI).
#
# ast-grep powers the advisory `make structure` guard. Version-pinned managers
# (npm/cargo/pipx) are tried first so the pinned AST_GREP_VERSION is honored;
# Homebrew is a last-resort fallback that installs its current (unpinned) formula.
# Override the version with AST_GREP_VERSION.
set -euo pipefail

version="${AST_GREP_VERSION:-0.44.1}"

# ast-grep exposes its CLI as `ast-grep` (recommended) or, on some installs, `sg`.
have_astgrep() { command -v ast-grep >/dev/null 2>&1 || command -v sg >/dev/null 2>&1; }
astgrep_bin() { command -v ast-grep >/dev/null 2>&1 && echo ast-grep || echo sg; }

if have_astgrep; then
  exit 0
fi

echo "ensure-ast-grep: ast-grep not found — installing ${version}..." >&2

attempt() {
  local label="$1"
  shift
  echo "ensure-ast-grep: trying ${label}..." >&2
  if "$@" && have_astgrep; then
    echo "ensure-ast-grep: installed via ${label} ($("$(astgrep_bin)" --version))" >&2
    return 0
  fi
  return 1
}

if command -v npm >/dev/null 2>&1; then
  # Global installs often fail without write access to the default npm prefix
  # (e.g. /usr/local); target a user-writable prefix and expose it on PATH.
  npm_prefix="${NPM_CONFIG_PREFIX:-$HOME/.local}"
  export NPM_CONFIG_PREFIX="$npm_prefix"
  export PATH="$npm_prefix/bin:$PATH"
  attempt "npm" npm install -g "@ast-grep/cli@${version}" && exit 0
fi
if command -v cargo >/dev/null 2>&1; then
  attempt "cargo" cargo install ast-grep --locked --version "${version}" && exit 0
fi
if command -v pipx >/dev/null 2>&1; then
  attempt "pipx" pipx install "ast-grep-cli==${version}" && exit 0
fi
# Homebrew last: its formula is unpinned, so it may install a version other than
# ${version} when no version-pinned manager is available.
if command -v brew >/dev/null 2>&1; then
  attempt "brew (unpinned)" brew install ast-grep && exit 0
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
