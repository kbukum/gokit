# Multi-Module Versioning Guide

This document explains how versioning works in the gokit multi-module repository.

## Problem: Pseudo-Versions

Without proper Git tags, Go assigns pseudo-versions like `v0.0.0-00010101000000-000000000000` to modules. This happens because Go cannot determine the version from the repository history.

## Solution: Semantic Versioning with Git Tags

gokit uses **semantic versioning** (semver) with Git tags to provide proper module versions.

### Version Format

```
vMAJOR.MINOR.PATCH[-PRERELEASE][+BUILDMETADATA]
```

Examples:
- `v0.1.0` - First minor release
- `v1.0.0` - First major release
- `v1.2.3` - Standard release
- `v1.0.0-alpha.1` - Pre-release
- `v1.0.0+20240522` - With build metadata

### Multi-Module Tag Convention

In a multi-module repository, each module needs its own tag:

```
# Main module
v0.1.0

# Submodules (auto-discovered from go.mod files)
auth/v0.1.0
authz/v0.1.0
connect/v0.1.0
database/v0.1.0
discovery/v0.1.0
grpc/v0.1.0
httpclient/v0.1.0
kafka/v0.1.0
redis/v0.1.0
server/v0.1.0
storage/v0.1.0
testutil/v0.1.0
workload/v0.1.0
```

> **Note:** The `tag-modules.sh` script automatically discovers all modules by finding `go.mod` files. You never need to maintain a hardcoded list.

## Tagging Modules

### Quick Start

```bash
# Tag all modules with v0.1.0
make tag VERSION=v0.1.0

# Tag and push to remote
make tag-push VERSION=v0.1.0

# Force overwrite existing tags
make tag-force VERSION=v0.1.0

# List all tags
make list-tags
```

### Manual Tagging

If you prefer manual control:

```bash
# Tag main module
git tag v0.1.0

# Tag submodules
git tag auth/v0.1.0
git tag connect/v0.1.0
git tag database/v0.1.0
# ... etc

# Push all tags
git push origin --tags
```

### Using the Script Directly

```bash
# Basic usage
./tag-modules.sh v0.1.0

# With options
./tag-modules.sh v0.1.0 --push        # Tag and push
./tag-modules.sh v0.1.0 --force       # Force overwrite
./tag-modules.sh v0.1.0 --push --force # Both
```

## Version Compatibility

### Breaking Changes (Major Version Bump)

When making breaking changes:

```bash
# v0.x.x to v1.0.0 (first stable)
make tag VERSION=v1.0.0

# v1.x.x to v2.0.0 (breaking change)
make tag VERSION=v2.0.0
```

For major versions ≥2, Go requires a version suffix in the module path:

```go
// go.mod for v2+
module github.com/kbukum/gokit/v2

// imports
import "github.com/kbukum/gokit/v2/logger"
```

### Non-Breaking Changes

```bash
# New features (minor bump)
make tag VERSION=v0.2.0

# Bug fixes (patch bump)
make tag VERSION=v0.1.1
```

## Using Versioned Modules

### In Consumer Projects

```bash
# Use specific version
go get github.com/kbukum/gokit@v0.1.0
go get github.com/kbukum/gokit/auth@v0.1.0
go get github.com/kbukum/gokit/connect@v0.1.0

# Use latest
go get -u github.com/kbukum/gokit
go get -u github.com/kbukum/gokit/auth
go get -u github.com/kbukum/gokit/connect
```

### Local Development

For local development, use `replace` directives in `go.mod`:

```go
replace (
    github.com/kbukum/gokit => ../gokit
    github.com/kbukum/gokit/auth => ../gokit/auth
    github.com/kbukum/gokit/connect => ../gokit/connect
)
```

## Release Workflow

### 1. Prepare Release

```bash
# Update CHANGELOG.md
vim CHANGELOG.md

# Run all checks
make check

# Tidy dependencies
make tidy

# Commit changes
git add .
git commit -m "chore: prepare v0.1.0 release"
```

### 2. Tag Release

```bash
# Tag all modules
make tag VERSION=v0.1.0

# Review tags
make list-tags

# Push tags
git push origin --tags
```

### 3. Verify

```bash
# Check that modules can be fetched
go get github.com/kbukum/gokit@v0.1.0
go get github.com/kbukum/gokit/connect@v0.1.0
```

## Troubleshooting

### Problem: Pseudo-Version Still Appears

**Cause:** Tags not pushed to remote or `go.sum` cache issue

**Solution:**
```bash
# Ensure tags are pushed
git push origin --tags

# Clear Go module cache
go clean -modcache

# Re-fetch
go get -u github.com/kbukum/gokit@v0.1.0
```

### Problem: "Unknown Revision" Error

**Cause:** Tag doesn't exist on remote

**Solution:**
```bash
# Push tags to remote
git push origin --tags

# Or use make target
make tag-push VERSION=v0.1.0
```

### Problem: Wrong Version Resolved

**Cause:** Newer tag exists or local cache

**Solution:**
```bash
# List all tags
git tag -l

# Delete wrong tag locally
git tag -d v0.1.0
git tag -d auth/v0.1.0

# Delete on remote
git push origin :refs/tags/v0.1.0
git push origin :refs/tags/auth/v0.1.0

# Re-tag correctly
make tag VERSION=v0.1.0 --force
```

## Best Practices

1. **Always tag all modules together** - Use `make tag` to ensure consistency
2. **Follow semver strictly** - Breaking changes = major bump
3. **Update CHANGELOG.md** - Document changes before tagging
4. **Test before tagging** - Run `make check` first
5. **Never force-push tags** - Unless absolutely necessary
6. **Use pre-release tags for testing** - e.g., `v0.2.0-beta.1`

## Version Strategy

### v0.x.x - Pre-1.0 Development

- Breaking changes allowed in minor versions
- Use for active development
- API may change

### v1.0.0 - First Stable Release

- Signals API stability
- Breaking changes require v2.0.0
- Commit to backward compatibility

### v1.x.x - Stable Maintenance

- Only non-breaking changes
- Bug fixes in patch versions
- New features in minor versions

## References

- [Go Modules Reference](https://go.dev/ref/mod)
- [Semantic Versioning](https://semver.org/)
- [Go Module Versioning](https://go.dev/doc/modules/version-numbers)
- [Multi-Module Repositories](https://github.com/go-modules-by-example/index/blob/master/009_submodules/README.md)
