# Quick Fix: Versioning Issue

## Problem

You're seeing pseudo-versions in your dependencies:
```
github.com/kbukum/gokit/connect v0.0.0-00010101000000-000000000000
github.com/kbukum/gokit/database v0.0.0-00010101000000-000000000000
...
```

## Root Cause

The modules don't have Git tags, so Go cannot determine their version and falls back to a pseudo-version.

## Quick Fix (3 Steps)

### Step 1: Commit Your Changes

```bash
cd /Users/kbukum/DEV/skillsense/all/gokit

# Add all changes (including the new auth.go file)
git add .

# Commit
git commit -m "feat(connect): add JWT authentication interceptor integration with gokit/auth"
```

### Step 2: Tag All Modules

```bash
# Tag all modules with v0.1.0
make tag VERSION=v0.1.0
```

This will create tags:
- `v0.1.0` (main module)
- `auth/v0.1.0`
- `connect/v0.1.0`
- `database/v0.1.0`
- `discovery/v0.1.0`
- `diarization/v0.1.0`
- `grpc/v0.1.0`
- `kafka/v0.1.0`
- `llm/v0.1.0`
- `redis/v0.1.0`
- `storage/v0.1.0`
- `transcription/v0.1.0`
- `workload/v0.1.0`

### Step 3: Push Tags to Remote (Optional but Recommended)

```bash
# Push commits and tags
git push
git push origin --tags

# Or use the combined command
make tag-push VERSION=v0.1.0
```

## Verify the Fix

### In gokit repository:

```bash
# List all tags
make list-tags

# Should show:
# auth/v0.1.0
# connect/v0.1.0
# v0.1.0
# ...
```

### In consuming projects (like platform):

```bash
cd /Users/kbukum/DEV/skillsense/all/platform

# Update dependencies to use tagged versions
go get github.com/kbukum/gokit@v0.1.0
go get github.com/kbukum/gokit/auth@v0.1.0
go get github.com/kbukum/gokit/connect@v0.1.0

# Or update all at once
go get -u github.com/kbukum/gokit/...@v0.1.0

# Then tidy
go mod tidy
```

## For Local Development

If you want to continue using local development (without tags), keep the `replace` directives in your `go.mod`:

```go
replace (
    github.com/kbukum/gokit => ../gokit
    github.com/kbukum/gokit/auth => ../gokit/auth
    github.com/kbukum/gokit/connect => ../gokit/connect
    // ... other modules
)
```

The pseudo-versions won't affect functionality when using `replace` directives. They only appear in the dependency list but are overridden by the local paths.

## Future Releases

When you make changes and want to release:

```bash
# 1. Update CHANGELOG.md with changes
# 2. Commit changes
git add .
git commit -m "chore: prepare v0.2.0 release"

# 3. Tag new version
make tag VERSION=v0.2.0

# 4. Push
git push origin --tags
```

## More Information

See [docs/VERSIONING.md](docs/VERSIONING.md) for complete versioning guide.
