#!/bin/bash
# gomod.sh — Run go commands across all modules or a specific module
# Usage:
#   ./gomod.sh tidy                              # go mod tidy all modules
#   ./gomod.sh tidy -m kafka                     # go mod tidy kafka module
#   ./gomod.sh update                            # go get -u ./... all modules
#   ./gomod.sh update-go 1.26.0                  # update go version in all go.mod files
#   ./gomod.sh cmd "go test ./..."               # run command in all modules
#   ./gomod.sh cmd "go test ./..." -m kafka      # run command in kafka module only
#   ./gomod.sh cmd "go test" -m httpclient/rest  # resolves to httpclient module, ./rest/... package
#   ./gomod.sh cmd "go test" -m security         # resolves to root module, ./security/... package

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

ROOT_DIR=$(pwd)
FAILED_MODULES=()

# ─────────────────────────────────────────────────────────────────────────────
# Module resolution
#
# Given a target path (e.g. "httpclient/rest"), resolves:
#   MOD_DIR  — directory containing go.mod (e.g. "httpclient")
#   PKG      — Go package pattern relative to module root (e.g. "./rest/...")
#
# Resolution rules:
#   kafka           → mod_dir=kafka,           pkg=./...           (exact module)
#   kafka/testutil  → mod_dir=kafka/testutil,  pkg=./...           (nested module)
#   grpc/client     → mod_dir=grpc,            pkg=./client/...    (subpackage)
#   security        → mod_dir=.,               pkg=./security/...  (core subpackage)
# ─────────────────────────────────────────────────────────────────────────────
resolve_module() {
  local target="$1"
  MOD_DIR="$target"

  # Walk up from target to find the nearest go.mod
  while [ ! -f "$MOD_DIR/go.mod" ] && [ "$MOD_DIR" != "." ]; do
    MOD_DIR=$(dirname "$MOD_DIR")
  done

  # Determine the package path relative to module root
  if [ "$MOD_DIR" = "$target" ]; then
    PKG="./..."
  elif [ "$MOD_DIR" = "." ]; then
    PKG="./${target}/..."
  else
    PKG="./${target#$MOD_DIR/}/..."
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Helpers
# ─────────────────────────────────────────────────────────────────────────────

# Find all modules, excluding vendor directories
find_modules() {
  find "$ROOT_DIR" -name "go.mod" \
    -not -path "*/vendor/*" \
    -not -path "*/.git/*" \
    | sort
}

run_in_module() {
  local modfile=$1
  local dir
  dir=$(dirname "$modfile")
  local rel_dir="${dir#$ROOT_DIR/}"

  echo -e "\n${YELLOW}▶ Module: ${rel_dir}${NC}"

  if ! (cd "$dir" && eval "$2"); then
    echo -e "${RED}✗ Failed: ${rel_dir}${NC}"
    FAILED_MODULES+=("$rel_dir")
  else
    echo -e "${GREEN}✓ Done: ${rel_dir}${NC}"
  fi
}

run_in_target() {
  local target="$1"
  local command="$2"

  resolve_module "$target"

  local label="$MOD_DIR"
  if [ "$PKG" != "./..." ]; then
    label="$MOD_DIR ($PKG)"
  fi
  echo -e "\n${YELLOW}▶ Module: ${label}${NC}"

  if ! (cd "$MOD_DIR" && eval "$command $PKG"); then
    echo -e "${RED}✗ Failed: ${label}${NC}"
    FAILED_MODULES+=("$label")
  else
    echo -e "${GREEN}✓ Done: ${label}${NC}"
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Commands
# ─────────────────────────────────────────────────────────────────────────────

cmd_tidy() {
  local target="$1"
  if [ -n "$target" ]; then
    resolve_module "$target"
    echo "Running: go mod tidy in $MOD_DIR..."
    run_in_module "$MOD_DIR/go.mod" "go mod tidy"
  else
    echo "Running: go mod tidy across all modules..."
    while IFS= read -r modfile; do
      run_in_module "$modfile" "go mod tidy"
    done < <(find_modules)
  fi
}

cmd_update() {
  local target="$1"
  if [ -n "$target" ]; then
    resolve_module "$target"
    echo "Running: go get -u in $MOD_DIR..."
    run_in_module "$MOD_DIR/go.mod" "go get -u ./... && go mod tidy"
  else
    echo "Running: go get -u ./... across all modules..."
    while IFS= read -r modfile; do
      run_in_module "$modfile" "go get -u ./... && go mod tidy"
    done < <(find_modules)
  fi
}

cmd_update_go() {
  local version=$1
  if [[ -z "$version" ]]; then
    echo -e "${RED}Error: Go version required. e.g. ./gomod.sh update-go 1.26.0${NC}"
    exit 1
  fi

  echo "Updating go version to $version across all modules..."
  while IFS= read -r modfile; do
    run_in_module "$modfile" "go mod edit -go=$version && go mod tidy"
  done < <(find_modules)
}

cmd_custom() {
  local command="$1"
  local target="$2"

  if [[ -z "$command" ]]; then
    echo -e "${RED}Error: Command required. e.g. ./gomod.sh cmd \"go test\" [-m module]${NC}"
    exit 1
  fi

  if [ -n "$target" ]; then
    echo "Running: '$command' in $target..."
    run_in_target "$target" "$command"
  else
    echo "Running: '$command' across all modules..."
    while IFS= read -r modfile; do
      run_in_module "$modfile" "$command ./..."
    done < <(find_modules)
  fi
}

print_summary() {
  echo -e "\n==============================="
  if [[ ${#FAILED_MODULES[@]} -eq 0 ]]; then
    echo -e "${GREEN}✓ All modules completed successfully.${NC}"
  else
    echo -e "${RED}✗ Failed modules:${NC}"
    for m in "${FAILED_MODULES[@]}"; do
      echo -e "  ${RED}- $m${NC}"
    done
    exit 1
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Argument parsing: extract -m <module> from any position
# ─────────────────────────────────────────────────────────────────────────────
parse_module_flag() {
  MODULE_TARGET=""
  REMAINING_ARGS=()
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -m)
        MODULE_TARGET="$2"
        shift 2
        ;;
      *)
        REMAINING_ARGS+=("$1")
        shift
        ;;
    esac
  done
}

# --- Main ---
ACTION="$1"
shift || true
parse_module_flag "$@"

case "$ACTION" in
  tidy)        cmd_tidy "$MODULE_TARGET" ;;
  update)      cmd_update "$MODULE_TARGET" ;;
  update-go)   cmd_update_go "${REMAINING_ARGS[0]}" ;;
  cmd)         cmd_custom "${REMAINING_ARGS[0]}" "$MODULE_TARGET" ;;
  *)
    echo "Usage:"
    echo "  ./gomod.sh tidy [-m module]          # go mod tidy"
    echo "  ./gomod.sh update [-m module]        # go get -u ./..."
    echo "  ./gomod.sh update-go <version>       # update go version in all go.mod"
    echo "  ./gomod.sh cmd \"<command>\" [-m mod]   # run command in module(s)"
    echo ""
    echo "Module targeting (-m):"
    echo "  -m kafka             Target kafka module"
    echo "  -m httpclient/rest   Target httpclient module, rest package"
    echo "  -m grpc/client       Target grpc module, client package"
    echo "  -m security          Target root module, security package"
    exit 1
    ;;
esac

print_summary