#!/bin/bash
# tag-modules.sh — Create Git tags for all modules in the multi-module repo
# This resolves the v0.0.0-00010101000000-000000000000 pseudo-version issue
#
# Usage:
#   ./tag-modules.sh v0.1.0              # Tag all modules with v0.1.0
#   ./tag-modules.sh v0.1.0 --push       # Tag and push to remote
#   ./tag-modules.sh v0.1.0 --force      # Force overwrite existing tags

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

VERSION="${1:-}"
PUSH_TAGS=false
FORCE_TAG=false

# Parse arguments
for arg in "$@"; do
  case $arg in
    --push)
      PUSH_TAGS=true
      shift
      ;;
    --force)
      FORCE_TAG=true
      shift
      ;;
  esac
done

if [ -z "$VERSION" ]; then
  echo -e "${RED}Error: Version is required${NC}"
  echo ""
  echo "Usage:"
  echo "  ./tag-modules.sh v0.1.0              # Tag all modules"
  echo "  ./tag-modules.sh v0.1.0 --push       # Tag and push to remote"
  echo "  ./tag-modules.sh v0.1.0 --force      # Force overwrite existing tags"
  exit 1
fi

# Validate semantic version format
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?(\+[a-zA-Z0-9.]+)?$ ]]; then
  echo -e "${RED}Error: Invalid version format. Must follow semantic versioning (e.g., v0.1.0)${NC}"
  exit 1
fi

ROOT_DIR=$(pwd)

echo -e "${BLUE}════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  Multi-Module Tagging Script${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════════${NC}"
echo -e "Version:      ${GREEN}${VERSION}${NC}"
echo -e "Push tags:    $([ "$PUSH_TAGS" = true ] && echo "${GREEN}Yes${NC}" || echo "${YELLOW}No${NC}")"
echo -e "Force update: $([ "$FORCE_TAG" = true ] && echo "${GREEN}Yes${NC}" || echo "${YELLOW}No${NC}")"
echo -e "${BLUE}════════════════════════════════════════════════════════════${NC}"
echo ""

# Check if git working directory is clean (refuse, don't prompt — releases
# must be reproducible and signed from a known-good tree).
if [ -n "$(git status --porcelain)" ]; then
  echo -e "${RED}✗ Working directory has uncommitted changes.${NC}"
  echo -e "  Releases must be cut from a clean tree so the tag SHA matches"
  echo -e "  exactly what's on origin/main. Stash or commit first."
  git --no-pager status --short
  exit 1
fi

# Refuse to tag if HEAD is not on origin/main (or the version's release branch
# for hotfixes — anything matching hotfix/${VERSION}).
current_branch=$(git rev-parse --abbrev-ref HEAD)
expected="main"
case "$current_branch" in
  hotfix/${VERSION}|hotfix/${VERSION#v}) expected="$current_branch" ;;
esac
if [ "$current_branch" != "$expected" ]; then
  echo -e "${RED}✗ Refusing to tag from branch '$current_branch'.${NC}"
  echo -e "  Releases must be cut from '$expected' (or 'hotfix/${VERSION}')."
  exit 1
fi

# CHANGELOG check (skipped for pre-releases like v0.3.0-rc.1, v0.3.0-beta.2).
if [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  if ! grep -qE "^## \[${VERSION#v}\]" CHANGELOG.md; then
    echo -e "${RED}✗ CHANGELOG.md is missing a '## [${VERSION#v}] - YYYY-MM-DD' section.${NC}"
    echo -e "  Add one (move items out of '## [Unreleased]') before tagging."
    exit 1
  fi
  # Refuse if [Unreleased] is the only populated section.
  unreleased_body=$(awk '/^## \[Unreleased\]/{flag=1; next} /^## /{flag=0} flag' CHANGELOG.md)
  if [ -z "$(echo "$unreleased_body" | grep -v '^[[:space:]]*$' | grep -v '^_No unreleased')" ] \
     && ! grep -qE "^## \[${VERSION#v}\]" CHANGELOG.md; then
    echo -e "${RED}✗ Nothing to release — CHANGELOG '[Unreleased]' is empty and no [${VERSION#v}] section exists.${NC}"
    exit 1
  fi
else
  echo -e "${YELLOW}⚠ Pre-release detected (${VERSION}); skipping CHANGELOG check.${NC}"
fi

# Verify GPG signing is configured (required for signed tags).
if ! git config --get user.signingkey >/dev/null && ! git config --get tag.gpgsign | grep -qi true; then
  echo -e "${YELLOW}⚠ GPG tag signing is not configured.${NC}"
  echo -e "  Run: git config --global user.signingkey <KEY-ID>"
  echo -e "       git config --global tag.gpgsign true"
  echo -e "  Falling back to annotated (unsigned) tags. Production releases SHOULD be signed."
  TAG_FLAG="-a"
  TAG_MSG_FLAG="-m"
else
  TAG_FLAG="-s -a"
  TAG_MSG_FLAG="-m"
fi

# Find all module directories
MODULES=$(find . -name "go.mod" -not -path "*/vendor/*" -not -path "*/.git/*" | sort)

# Tag main module first
echo -e "\n${YELLOW}▶ Tagging main module: ${VERSION}${NC}"
FORCE_FLAG=""
if [ "$FORCE_TAG" = true ]; then
  FORCE_FLAG="-f"
fi

if git tag $FORCE_FLAG $TAG_FLAG "$VERSION" $TAG_MSG_FLAG "Release $VERSION" 2>/dev/null; then
  echo -e "${GREEN}✓ Created tag: ${VERSION}${NC}"
else
  if [ "$FORCE_TAG" = false ]; then
    echo -e "${RED}✗ Tag already exists. Use --force to overwrite.${NC}"
  else
    echo -e "${RED}✗ Failed to create tag${NC}"
  fi
fi

# Tag each submodule
for modfile in $MODULES; do
  dir=$(dirname "$modfile")
  
  # Skip root module (already tagged)
  if [ "$dir" = "." ]; then
    continue
  fi
  
  # Remove leading ./
  module_name="${dir#./}"
  
  # Create tag: <module>/<version>
  tag_name="${module_name}/${VERSION}"
  
  echo -e "\n${YELLOW}▶ Tagging submodule: ${module_name} → ${tag_name}${NC}"
  
  if git tag $FORCE_FLAG $TAG_FLAG "$tag_name" $TAG_MSG_FLAG "Release ${module_name} ${VERSION}" 2>/dev/null; then
    echo -e "${GREEN}✓ Created tag: ${tag_name}${NC}"
  else
    if [ "$FORCE_TAG" = false ]; then
      echo -e "${RED}✗ Tag already exists. Use --force to overwrite.${NC}"
    else
      echo -e "${RED}✗ Failed to create tag${NC}"
    fi
  fi
done

# List all version tags
echo -e "\n${BLUE}════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  Created Tags${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════════${NC}"
git tag -l | grep -E "^(${VERSION}|.*/${VERSION})$" || echo "No tags found"
echo ""

# Push tags if requested
if [ "$PUSH_TAGS" = true ]; then
  echo -e "${YELLOW}▶ Pushing tags to remote...${NC}"
  PUSH_FORCE_FLAG=""
  if [ "$FORCE_TAG" = true ]; then
    PUSH_FORCE_FLAG="--force"
  fi
  
  if git push origin --tags $PUSH_FORCE_FLAG; then
    echo -e "${GREEN}✓ Tags pushed successfully${NC}"
  else
    echo -e "${RED}✗ Failed to push tags${NC}"
    exit 1
  fi
fi

# Display next steps
echo -e "${BLUE}════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  Next Steps${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════════${NC}"
if [ "$PUSH_TAGS" = false ]; then
  echo -e "1. Review the tags: ${YELLOW}git tag -l${NC}"
  echo -e "2. Push to remote:  ${YELLOW}git push origin --tags${NC}"
  echo -e "   Or re-run with:  ${YELLOW}./tag-modules.sh ${VERSION} --push${NC}"
fi
echo -e ""
echo -e "To update dependencies in consuming projects:"
echo -e "  ${YELLOW}go get -u github.com/kbukum/gokit@${VERSION}${NC}"
echo -e "  ${YELLOW}go get -u github.com/kbukum/gokit/auth@${VERSION}${NC}"
echo -e "  ${YELLOW}go get -u github.com/kbukum/gokit/connect@${VERSION}${NC}"
echo -e "${BLUE}════════════════════════════════════════════════════════════${NC}"
