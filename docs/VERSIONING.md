# Multi-Module Versioning

gokit is a Go workspace with one root module and nested sub-modules. Every module receives its own tag because Go resolves module versions from the module's path-scoped tag.

## Version format

Tags use Semantic Versioning:

```
vMAJOR.MINOR.PATCH[-PRERELEASE]
```

The current development line is `v0.2.0-alpha.1`. Prereleases are ordered as `alpha.1`, `alpha.2`, and so on; the stable `v0.2.0` follows the final prerelease.

## Module tags

The root module uses the plain tag and every sub-module uses its directory prefix:

```
v0.2.0-alpha.1
auth/v0.2.0-alpha.1
database/sqlite/v0.2.0-alpha.1
messaging/kafka/v0.2.0-alpha.1
```

`tag-modules.sh` discovers every `go.mod` file, so the module list is never maintained in documentation. Normal releases tag all modules in lock-step for operational simplicity, while each module remains independently consumable and versioned.

## Prerelease workflow

1. Add the completed changes to a dated `## [0.2.0-alpha.N]` section in `CHANGELOG.md` and leave an empty `## [Unreleased]` section above it.
2. Run the complete release gates on a clean `main` checkout.
3. Preview the exact tag set without creating tags:

   ```bash
   ./tag-modules.sh v0.2.0-alpha.1 --dry-run
   ```

4. Configure a GPG signing key, create the signed tags, and push them:

   ```bash
   git config tag.gpgsign true
   git config user.signingkey <KEY-ID>
   ./tag-modules.sh v0.2.0-alpha.1 --push
   ```

5. The root tag starts the release workflow. GoReleaser publishes the GitHub prerelease together with the source archive, checksums, SBOM, signatures, and provenance.

Subsequent prereleases repeat the same flow with `alpha.2`, `alpha.3`, or `beta.N`. Once the API and behavior are ready for consumers, cut `v0.2.0` from the same release line.

## Stable and patch releases

While gokit is below `1.0.0`, breaking changes require a minor version bump and non-breaking additions or fixes use a patch bump. A stable release rotates the populated `[Unreleased]` section into `## [X.Y.Z] - YYYY-MM-DD`, then adds a new empty `[Unreleased]` section.

Hotfixes for an older line use a dedicated hotfix branch and the same module-tagging script. Never move an existing published tag.

## Consumer usage

```bash
go get github.com/kbukum/gokit@v0.2.0-alpha.1
go get github.com/kbukum/gokit/auth@v0.2.0-alpha.1
go get github.com/kbukum/gokit/database/sqlite@v0.2.0-alpha.1
```

Use local `replace` directives only for development against an unreleased checkout. Published consumers should use a tag so Go does not select a pseudo-version.

## Verification

After publication, verify the root and representative nested modules through the Go proxy:

```bash
GOPROXY=https://proxy.golang.org go list -m github.com/kbukum/gokit@v0.2.0-alpha.1
GOPROXY=https://proxy.golang.org go list -m github.com/kbukum/gokit/database/sqlite@v0.2.0-alpha.1
```

The complete mechanical procedure is in [`RELEASING.md`](RELEASING.md), and the SemVer compatibility rules are in [`policy/SEMVER.md`](policy/SEMVER.md).
