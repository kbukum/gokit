#!/bin/bash
# gomod.sh — Run go commands across all modules in a multi-module project
# Usage:
#   ./gomod.sh tidy                        # go mod tidy all modules
#   ./gomod.sh update                      # go get -u ./... all modules
#   ./gomod.sh update-go 1.26.0            # update go version in all go.mod files
#   ./gomod.sh cmd "go test ./..."         # run any custom command in each module

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

ROOT_DIR=$(pwd)
FAILED_MODULES=()

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

cmd_tidy() {
  echo "Running: go mod tidy across all modules..."
  while IFS= read -r modfile; do
    run_in_module "$modfile" "go mod tidy"
  done < <(find_modules)
}

cmd_update() {
  echo "Running: go get -u ./... across all modules..."
  while IFS= read -r modfile; do
    run_in_module "$modfile" "go get -u ./... && go mod tidy"
  done < <(find_modules)
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
  local command=$1
  if [[ -z "$command" ]]; then
    echo -e "${RED}Error: Command required. e.g. ./gomod.sh cmd \"go test ./...\"${NC}"
    exit 1
  fi

  echo "Running: '$command' across all modules..."
  while IFS= read -r modfile; do
    run_in_module "$modfile" "$command"
  done < <(find_modules)
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

# --- Main ---
case "$1" in
  tidy)        cmd_tidy ;;
  update)      cmd_update ;;
  update-go)   cmd_update_go "$2" ;;
  cmd)         cmd_custom "$2" ;;
  *)
    echo "Usage:"
    echo "  ./gomod.sh tidy                    # go mod tidy all modules"
    echo "  ./gomod.sh update                  # go get -u ./... all modules"
    echo "  ./gomod.sh update-go <version>     # update go version in all go.mod"
    echo "  ./gomod.sh cmd \"<command>\"          # run any command in each module"
    exit 1
    ;;
esac

print_summary