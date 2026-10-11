# Multi-module versioning

Every Go module has its own path-scoped tag. Toven discovers the module set and publishes normal releases in lock-step; consumers pin only the modules they use.

## Tags and compatibility

Tags follow `vMAJOR.MINOR.PATCH[-PRERELEASE]`:

| Module | Example tag |
|---|---|
| Root | `v0.3.0-alpha.1` |
| Auth | `auth/v0.3.0-alpha.1` |
| SQLite adapter | `database/sqlite/v0.3.0-alpha.1` |

Before 1.0, breaking changes require a minor bump; non-breaking additions and fixes use a patch bump. Prereleases progress through suffixes such as `alpha.1` and `beta.1` before the stable release. See [SemVer](policy/SEMVER.md) and [Deprecation](policy/DEPRECATION.md).

## Release flow

Review the version/dependency bump through a PR, then publish the merged commit through Toven. The hosted Release carries notes; its root tag triggers signed artifacts and provenance. [Releasing](RELEASING.md) owns the complete gates, proxy checks, approval, and recovery procedure.

Toven derives versions from reachable tags and commit history. A module without a prior version needs an explicit `--set-version`, with no `v` prefix. It does not invent a zero-version baseline.

Git history is not proof a version is unused: the Go proxy permanently caches published versions. Check every module before selecting a release, and never move or reuse a published tag.

## Inspect and consume

```bash
toven modules
toven release status
toven release plan
```

Consumers request a published version:

```bash
go get github.com/kbukum/gokit@v0.2.0
go get github.com/kbukum/gokit/auth@v0.2.0
```

For unreleased local development, use relative `replace` directives. After publication, verify the selected root and nested versions with `GOPROXY=https://proxy.golang.org go list -m <module>@<version>`.
