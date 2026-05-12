#!/bin/bash
# gomod.sh — Run go commands across all modules or a specific module
# Usage:
#   ./gomod.sh tidy                              # go mod tidy all modules
#   ./gomod.sh tidy -m messaging                  # go mod tidy messaging module
#   ./gomod.sh update                            # go get -u ./... all modules
#   ./gomod.sh update-go 1.26.0                  # update go version in all go.mod files
#   ./gomod.sh cmd "go test ./..."               # run command in all modules
#   ./gomod.sh cmd "go test ./..." -m messaging   # run command in messaging module only
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
WORKSPACE_TARGET=""
WORKSPACE_FILE=""

# ─────────────────────────────────────────────────────────────────────────────
# Module resolution
#
# Given a target path (e.g. "httpclient/rest"), resolves:
#   MOD_DIR  — directory containing go.mod (e.g. "httpclient")
#   PKG      — Go package pattern relative to module root (e.g. "./rest/...")
#
# Resolution rules:
#   messaging       → mod_dir=messaging,       pkg=./...           (exact module)
#   messaging/testutil  → mod_dir=messaging/testutil,  pkg=./...           (nested module)
#   grpc/client     → mod_dir=grpc,            pkg=./client/...    (subpackage)
#   security        → mod_dir=.,               pkg=./security/...  (core subpackage)
# ─────────────────────────────────────────────────────────────────────────────
# Validate that a resolved module is part of the selected workspace.
validate_workspace_membership() {
  local mod_dir="$1"
  [[ -z "$WORKSPACE_FILE" ]] && return 0

  local in_ws=false
  while IFS= read -r modfile; do
    local ws_dir
    ws_dir=$(dirname "$modfile")
    ws_dir="${ws_dir#$ROOT_DIR/}"
    [[ "$ws_dir" == "$ROOT_DIR" ]] && ws_dir="."
    if [[ "$mod_dir" == "$ws_dir" ]]; then
      in_ws=true
      break
    fi
  done < <(find_modules)

  if [[ "$in_ws" != true ]]; then
    echo -e "${RED}Error: module '$mod_dir' is not part of workspace '$WORKSPACE_TARGET'${NC}"
    exit 1
  fi
}

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

# Find all modules, excluding vendor directories. When -w is set, limit to modules
# listed in the selected workspace file.
find_modules() {
  if [[ -n "$WORKSPACE_FILE" ]]; then
    awk '
      /^use[[:space:]]*\(/ { in_use = 1; next }
      in_use && /^[[:space:]]*\)/ { in_use = 0; next }
      in_use {
        line = $0
        sub(/^[[:space:]]+/, "", line)
        sub(/[[:space:]]+$/, "", line)
        if (line != "") print line
        next
      }
      /^use[[:space:]]+\./ {
        line = $0
        sub(/^use[[:space:]]+/, "", line)
        sub(/[[:space:]]+$/, "", line)
        print line
      }
    ' "$WORKSPACE_FILE" | while IFS= read -r modpath; do
      modpath="${modpath#./}"
      modpath="${modpath%/}"
      if [[ "$modpath" == "." || -z "$modpath" ]]; then
        echo "$ROOT_DIR/go.mod"
      elif [[ -f "$ROOT_DIR/$modpath/go.mod" ]]; then
        echo "$ROOT_DIR/$modpath/go.mod"
      fi
    done | sort -u
    return
  fi

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
  validate_workspace_membership "$MOD_DIR"

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
    validate_workspace_membership "$MOD_DIR"
    echo "Running: go mod tidy in $MOD_DIR..."
    run_in_module "$MOD_DIR/go.mod" "go mod tidy"
  else
    echo "Running: go mod tidy across ${WORKSPACE_TARGET:+$WORKSPACE_TARGET }modules..."
    while IFS= read -r modfile; do
      run_in_module "$modfile" "go mod tidy"
    done < <(find_modules)
  fi
}

cmd_update() {
  local target="$1"
  if [ -n "$target" ]; then
    resolve_module "$target"
    validate_workspace_membership "$MOD_DIR"
    echo "Running: go get -u in $MOD_DIR..."
    run_in_module "$MOD_DIR/go.mod" "go get -u ./... && go mod tidy"
  else
    echo "Running: go get -u ./... across ${WORKSPACE_TARGET:+$WORKSPACE_TARGET }modules..."
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

  echo "Updating go version to $version across ${WORKSPACE_TARGET:+$WORKSPACE_TARGET }modules..."
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
    echo "Running: '$command' in $target${WORKSPACE_TARGET:+ (workspace: $WORKSPACE_TARGET)}..."
    run_in_target "$target" "$command"
  else
    echo "Running: '$command' across all modules${WORKSPACE_TARGET:+ (workspace: $WORKSPACE_TARGET)}..."
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
# Argument parsing: extract -m <module> and -w <workspace> flags (after action)
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
      -w)
        WORKSPACE_TARGET="$2"
        WORKSPACE_FILE="$ROOT_DIR/$2.go.work"
        if [[ ! -f "$WORKSPACE_FILE" ]]; then
          echo -e "${RED}Error: workspace file not found: $WORKSPACE_FILE${NC}"
          exit 1
        fi
        export GOWORK="$WORKSPACE_FILE"
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
    echo "  ./gomod.sh tidy [-m module] [-w workspace]          # go mod tidy"
    echo "  ./gomod.sh update [-m module] [-w workspace]        # go get -u ./..."
    echo "  ./gomod.sh update-go <version> [-w workspace]       # update go version in all go.mod"
    echo "  ./gomod.sh cmd \"<command>\" [-m mod] [-w workspace] # run command in module(s)"
    echo ""
    echo "Module targeting (-m):"
    echo "  -m messaging         Target messaging module"
    echo "  -m httpclient/rest   Target httpclient module, rest package"
    echo "  -m grpc/client       Target grpc module, client package"
    echo "  -m security          Target root module, security package"
    echo ""
    echo "Workspace targeting (-w):"
    echo "  -w core              Use core.go.work modules only"
    echo "  -w contrib           Use contrib.go.work modules only"
    exit 1
    ;;
esac

print_summary