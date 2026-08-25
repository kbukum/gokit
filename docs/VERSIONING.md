# Multi-Module Versioning

gokit is a Go workspace with one root module and nested sub-modules. Every module receives its own tag because Go resolves module versions from the module's path-scoped tag.

## Version format

Tags use Semantic Versioning:

```
vMAJOR.MINOR.PATCH[-PRERELEASE]
```

The last published line is `v0.2.0` (immutable on the Go module proxy). The next development line being prepared is `v0.3.0-alpha.1`; prereleases are ordered as `alpha.1`, `alpha.2`, and so on, and the stable `v0.3.0` follows the final prerelease.

## Module tags

The root module uses the plain tag and every sub-module uses its directory prefix:

```
v0.3.0-alpha.1
auth/v0.3.0-alpha.1
database/sqlite/v0.3.0-alpha.1
messaging/kafka/v0.3.0-alpha.1
```

Toven discovers every `go.mod` file, so the module list is never maintained in documentation. Releases tag all modules in lock-step, while consumers still import and pin only the modules they use.

## Prerelease workflow

1. Add the completed changes to a dated `## [0.3.0-alpha.N]` section in `CHANGELOG.md` and leave an empty `## [Unreleased]` section above it.
2. Run the complete release gates on a clean `main` checkout.
3. Branch off a clean `main`, then stage the lock-step version + dependency-floor bump on the branch (Phase 1) — `release bump` computes versions from main's baseline but writes into the working tree, so there is no need to stage on `main` first: `git switch -c release/vX.Y.Z && make release-bump SET_VERSION=0.3.0-alpha.1` (drop `SET_VERSION` once every module carries a tag). Rotate the CHANGELOG, commit, open a PR, and merge into `main`. See [`RELEASING.md`](RELEASING.md) for the full mechanics.
4. Preview the exact tag set without creating tags:

   ```bash
   toven release plan
   toven release publish --dry-run --set-version 0.3.0-alpha.1
   ```

5. Configure a GPG signing key, then cut and push the signed tags and the hosted Release (Phase 2, after the bump PR merges):

   ```bash
   git config tag.gpgsign true
   git config user.signingkey <KEY-ID>
   toven release publish --set-version 0.3.0-alpha.1 --yes
   ```

6. Cutting the tags creates the hosted GitHub prerelease with commit-derived notes; the pushed root tag then triggers the release workflow, where GoReleaser attaches the source archive, checksums, SBOM, signatures, and provenance to that Release.

Subsequent prereleases repeat the same flow with `alpha.2`, `alpha.3`, or `beta.N`. Once the API and behavior are ready for consumers, cut `v0.3.0` from the same release line.

## Stable and patch releases

While gokit is below `1.0.0`, breaking changes require a minor version bump and non-breaking additions or fixes use a patch bump. A stable release rotates the populated `[Unreleased]` section into `## [X.Y.Z] - YYYY-MM-DD`, then adds a new empty `[Unreleased]` section.

Hotfixes for an older line use a dedicated hotfix branch and the same Toven release flow. Never move an existing published tag.

## Consumer usage

```bash
go get github.com/kbukum/gokit@v0.2.0
go get github.com/kbukum/gokit/auth@v0.2.0
go get github.com/kbukum/gokit/database/sqlite@v0.2.0
```

Use local `replace` directives only for development against an unreleased checkout. Published consumers should use a tag so Go does not select a pseudo-version.

## Verification

After publication, verify the root and representative nested modules through the Go proxy:

```bash
GOPROXY=https://proxy.golang.org go list -m github.com/kbukum/gokit@v0.2.0
GOPROXY=https://proxy.golang.org go list -m github.com/kbukum/gokit/database/sqlite@v0.2.0
```

The complete mechanical procedure is in [`RELEASING.md`](RELEASING.md), and the SemVer compatibility rules are in [`policy/SEMVER.md`](policy/SEMVER.md).

## Toven release

`toven.toml`'s `[ecosystems.go.release]` block encodes the tag-only, registry-less model this document describes. The `toven` binary is the authoritative release path: it previews the version cascade and exact tag set, then cuts, signs, and pushes the tags and creates the hosted Release.

| Versioning behavior | Toven command | Expected output |
|---|---|---|
| Next version selection | `toven release plan` / `toven release status` | anchors on reachable git tags. Tagged modules auto-bump from Conventional-Commit history (e.g. `authz 0.2.0 → 0.2.1`); modules with no prior tag report "has never been released and declares no version" until an explicit `--set-version` is supplied to the mutating `bump`/`tag`/`publish` cut. Toven never fabricates a `0.0.0` baseline |
| Per-module tag names | `toven release plan` / `toven release publish --dry-run` | path-prefixed tag names (`auth/v…`, `database/sqlite/v…`, `messaging/nats/v…`) |
| Lock-step tagging of every `go.mod` | `toven modules` + `toven release plan` | all 50 modules discovered; every module deliberately included and tagged in lock-step. Toven does support a per-module `exclude`, but gokit's contract withholds none — inclusion is explicit, not a missing mechanism |
| Dependency-aware cascade | `toven release plan` (`dependency-cascade` reason column) | dependents shown as `cascade` when a dependency changes |
| Clean-tree requirement | `toven release readiness` | `clean-tree` check, `fail` on a dirty tree |
| Cut and push the signed tags | `toven release tag` / `toven release publish` | signed annotated tags created and pushed; the hosted Release is created with commit-derived notes |
