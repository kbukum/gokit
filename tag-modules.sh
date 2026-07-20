#!/usr/bin/env bash

set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'

VERSION=""
PUSH_TAGS=false
DRY_RUN=false

usage() {
  echo "Usage:"
  echo "  ./tag-modules.sh v0.3.0-alpha.1 --dry-run"
  echo "  ./tag-modules.sh v0.3.0-alpha.1 --push"
}

fail() {
  echo -e "${RED}Error: $*${NC}" >&2
  exit 1
}

valid_semver() {
  local version=$1
  local prerelease identifier

  if [[ ! "$version" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$ ]]; then
    return 1
  fi

  prerelease=${BASH_REMATCH[5]:-}
  if [[ -n "$prerelease" ]]; then
    IFS='.' read -r -a identifiers <<<"$prerelease"
    for identifier in "${identifiers[@]}"; do
      if [[ "$identifier" =~ ^[0-9]+$ && ${#identifier} -gt 1 && "$identifier" == 0* ]]; then
        return 1
      fi
    done
  fi
}

validate_changelog() {
  local release_version=${VERSION#v}
  local heading_prefix="## [${release_version}] - "

  if ! awk -v prefix="$heading_prefix" '
    index($0, prefix) == 1 {
      date = substr($0, length(prefix) + 1)
      if (date ~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]$/) {
        found = 1
      }
    }
    END { exit found ? 0 : 1 }
  ' CHANGELOG.md; then
    fail "CHANGELOG.md is missing [${release_version}] with a release date"
  fi

  if ! awk -v prefix="$heading_prefix" '
    index($0, prefix) == 1 { in_release = 1; next }
    in_release && /^## \[/ { exit has_content ? 0 : 1 }
    in_release && $0 !~ /^[[:space:]]*$/ { has_content = 1 }
    END { exit has_content ? 0 : 1 }
  ' CHANGELOG.md; then
    fail "CHANGELOG.md [${release_version}] is empty"
  fi

  if ! awk '
    /^## \[Unreleased\]$/ { in_unreleased = 1; next }
    in_unreleased && /^## \[/ { in_unreleased = 0 }
    in_unreleased && $0 !~ /^[[:space:]]*$/ && $0 != "_No unreleased changes._" { invalid = 1 }
    END { exit invalid ? 1 : 0 }
  ' CHANGELOG.md; then
    fail "CHANGELOG.md [Unreleased] must be empty before tagging"
  fi
}

validate_module_path() {
  local module_file=$1
  local module_dir=${module_file%/go.mod}
  local actual expected

  actual=$(awk '$1 == "module" { print $2; exit }' "$module_file")
  if [[ "$module_dir" == "." ]]; then
    expected="github.com/kbukum/gokit"
  else
    expected="github.com/kbukum/gokit/${module_dir#./}"
  fi
  if [[ "$actual" != "$expected" ]]; then
    fail "module path mismatch in ${module_file}: expected ${expected}, found ${actual:-<missing>}"
  fi
}

while (($# > 0)); do
  case "$1" in
    --dry-run)
      DRY_RUN=true
      ;;
    --push)
      PUSH_TAGS=true
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    --*)
      fail "unknown argument: $1"
      ;;
    *)
      if [[ -n "$VERSION" ]]; then
        echo -e "${RED}Error: multiple versions supplied${NC}" >&2
        usage >&2
        exit 1
      fi
      VERSION="$1"
      ;;
  esac
  shift
done

if [[ -z "$VERSION" ]]; then
  echo -e "${RED}Error: version is required${NC}" >&2
  usage >&2
  exit 1
fi

valid_semver "$VERSION" || fail "invalid semantic version: $VERSION"

if [[ "$PUSH_TAGS" == true && "$DRY_RUN" == true ]]; then
  fail "--dry-run cannot be combined with --push"
fi
if [[ "$PUSH_TAGS" == false && "$DRY_RUN" == false ]]; then
  fail "choose --dry-run to preview or --push to create and atomically publish tags"
fi

validate_changelog

MODULE_FILES=()
while IFS= read -r module_file; do
  validate_module_path "$module_file"
  MODULE_FILES+=("$module_file")
done < <(find . -name go.mod -not -path '*/vendor/*' -not -path '*/.git/*' | sort)

TAGS=("$VERSION")
for module_file in "${MODULE_FILES[@]}"; do
  module_dir="${module_file%/go.mod}"
  if [[ "$module_dir" != "." ]]; then
    TAGS+=("${module_dir#./}/${VERSION}")
  fi
done

for tag in "${TAGS[@]}"; do
  if git show-ref --tags --verify --quiet "refs/tags/$tag"; then
    fail "tag already exists locally: $tag"
  fi

  set +e
  git ls-remote --exit-code --tags origin "refs/tags/$tag" >/dev/null 2>&1
  remote_status=$?
  set -e
  if [[ "$remote_status" -eq 0 ]]; then
    fail "tag already exists on origin: $tag"
  elif [[ "$remote_status" -ne 2 ]]; then
    fail "could not check origin for existing tag: $tag"
  fi
done

echo -e "${BLUE}Version: ${GREEN}${VERSION}${NC}"
echo -e "${BLUE}Modules: ${#TAGS[@]} tags${NC}"
if [[ "$DRY_RUN" == true ]]; then
  printf '%s\n' "${TAGS[@]}"
  exit 0
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo -e "${RED}Error: working directory has uncommitted changes${NC}" >&2
  git --no-pager status --short >&2
  exit 1
fi

current_branch=$(git symbolic-ref --quiet --short HEAD) || fail "release tags cannot be created from detached HEAD"
expected_branch="main"
case "$current_branch" in
  hotfix/${VERSION}|hotfix/${VERSION#v})
    expected_branch="$current_branch"
    ;;
esac
if [[ "$current_branch" != "$expected_branch" ]]; then
  fail "releases must be cut from '${expected_branch}', not '${current_branch}'"
fi

git fetch --no-tags origin "refs/heads/${expected_branch}:refs/remotes/origin/${expected_branch}"
if [[ "$(git rev-parse HEAD)" != "$(git rev-parse "refs/remotes/origin/${expected_branch}")" ]]; then
  fail "HEAD must match origin/${expected_branch} before tagging"
fi

if [[ -z "$(git config --get user.signingkey || true)" ]]; then
  fail "configure user.signingkey before creating release tags"
fi

CREATED_TAGS=()
rollback_tags() {
  status=$?
  if ((${#CREATED_TAGS[@]} > 0)); then
    git tag -d "${CREATED_TAGS[@]}" >/dev/null 2>&1 || true
  fi
  exit "$status"
}
trap rollback_tags ERR INT TERM

for tag in "${TAGS[@]}"; do
  git tag -s -a "$tag" -m "Release ${tag}"
  CREATED_TAGS+=("$tag")
done

PUSH_REFS=()
for tag in "${TAGS[@]}"; do
  PUSH_REFS+=("refs/tags/${tag}:refs/tags/${tag}")
done
git push --atomic origin "${PUSH_REFS[@]}"

trap - ERR INT TERM
echo -e "${GREEN}Created and atomically pushed ${#TAGS[@]} signed module tags for ${VERSION}${NC}"
