# Releasing

The mechanical steps to cut a release of `gokit`.
For *what* counts as a breaking change vs a feature vs a fix, see `policy/SEMVER.md`
and `policy/DEPRECATION.md`.
For how much of the release and CI lifecycle Toven drives (and what stays native),
see [`TOVEN-MIGRATION.md`](TOVEN-MIGRATION.md).

## Prerequisites

- You are listed in `MAINTAINERS.md` and have push access to `kbukum/gokit`.
- Your local clone is on `main` with no uncommitted changes.
- `git`, `gh`, and `go` are on your `$PATH`.
- Your commits are GPG-signed (`git config commit.gpgsign true`) — release tags must be signed.

## 1. Decide the version

```sh
# What's the latest tag?
git tag --sort=-v:refname | head -1

# What changed since then?
git log --oneline $(git describe --tags --abbrev=0)..HEAD
```

Use the [SEMVER policy](./policy/SEMVER.md) to pick the next version. While in `0.x`,
every release with a breaking change in the `[Unreleased]` CHANGELOG section bumps MINOR;
otherwise PATCH.

## 2. Update the CHANGELOG

1. Open `CHANGELOG.md`.
2. For a prerelease, replace the populated `## [Unreleased]` section with `## [X.Y.Z-alpha.N] - YYYY-MM-DD` and add a fresh empty `## [Unreleased]` section above it.
3. For a stable release, use `## [X.Y.Z] - YYYY-MM-DD` and the same empty-section rotation.
4. Refuse to release when the populated `[Unreleased]` section is empty.

For stable releases, Toven requires a matching changelog section.
Prereleases use the dated alpha or beta section for release notes.

## 3. Stage the version bump (Phase 1)

A lock-step release rewrites every module's version and the inter-module
dependency floors — the `require github.com/kbukum/gokit/<mod> vX.Y.Z` lines that
let a published consumer resolve one gokit module's dependency on another. Toven
stages that rewrite for review; it never commits, tags, or pushes here.

From a clean `main`:

```sh
make release-bump   # toven release bump --yes
```

Toven rewrites each module's version and dependency floors and stages the
change. Then, still on `main`:

1. Rotate the CHANGELOG as in step 2 (if not already done) and stage it.
2. Cut a `release/vX.Y.Z` branch carrying the staged bump.
3. Open a pull request and merge it into `main` after review.

The bump lands through a reviewed PR so `main` stays the source of truth; the
signed tags are cut only after the PR merges (Phase 2). This is a first-release
caveat: gokit has no reachable tag yet, so `release status` / `release plan`
fail closed ("no reachable release tag") until the first version is cut — supply
that first version explicitly to the mutating action, per
[`VERSIONING.md`](VERSIONING.md); Toven never fabricates a synthetic `0.0.0`.

## 4. Cut the tags and hosted Release (Phase 2)

Run this only after the Phase 1 bump PR has merged into `main`, from a clean
checkout of the merged commit. Toven owns tagging and the hosted GitHub Release:
it discovers every `go.mod`, cuts path-prefixed signed tags in lock-step, and
creates the Release with commit-derived notes.

```sh
make release-plan               # preview the exact version cascade and tag set
make release-publish-dry-run    # mutation-free registry + hosted-Release rehearsal
make release-publish            # cut and push signed tags, then create the hosted Release
```

`make release-tag` is available if you want to create and push the signed tags
before creating the Release; `make release-publish` performs the full tag → push
→ hosted-Release sequence idempotently.

Toven will:
- Refuse to run with a dirty working tree (the clean-tree guardrail has no bypass).
- Refuse a partial or divergent existing tag set, failing closed with forward-fix guidance.
- Create signed annotated tags (`gpg.format` / signing key inherited from git config, or set via `[…release] sign_format` / `signing_key`) for the root and every sub-module in lock-step.
- Create the hosted GitHub Release with notes derived from each module's commit range.

## 5. Attach the supply-chain artifacts

Pushing the root tag starts `.github/workflows/release.yml`. GoReleaser runs in `keep-existing` mode and attaches the source archive, checksums, SBOM, signatures, and provenance to the Release Toven already created — it does not recreate the Release or replace Toven's notes. Do not create a second release manually with `gh`.

GitHub Releases are mandatory; downstream tooling (`go install`, Dependabot,
and pkg.go.dev) only surface release signal when both the tag and release exist.

## 6. Verify on `pkg.go.dev`

```sh
gh browse --no-browser
# then visit
#   https://pkg.go.dev/github.com/kbukum/gokit@vX.Y.Z
```

Trigger a re-fetch if needed:

```sh
GOPROXY=https://proxy.golang.org go list -m github.com/kbukum/gokit@vX.Y.Z
```

## 7. Announce

- Post in the project's discussion / README "Latest" section.
- Open a "post-release smoke test" issue against the next sprint milestone.

## Hotfix releases

Hotfixes follow the same flow
but skip the `[Unreleased]` rotation if the fix is targeted at an older line:

```sh
git checkout <existing-release-tag>
git checkout -b hotfix/vX.Y.Z
# … apply fix …
# add a `## [0.2.1] - YYYY-MM-DD` section to CHANGELOG.md
toven release publish --yes
```

## Pre-releases

```sh
toven release publish --dry-run
toven release publish --yes
```

Toven marks the hosted Release as a prerelease from the tag's prerelease suffix,
and GoReleaser (in `keep-existing` mode) attaches artifacts to that Release.

## Recovery

Published version tags are immutable — never delete, move, or force-push a `vX.Y.Z` tag (or a path-prefixed submodule tag) once it has been pushed, because the Go module proxy and pkg.go.dev cache them permanently. Recover by fixing forward, not by rewriting history.

Inspect the current state before doing anything:

```sh
toven release status   # reachable tags, declared versions, and the hosted Release state
make list-tags         # every version tag currently on the remote
```

- **Partial tag push (some module tags pushed, others not).** `toven release publish` is idempotent and lock-step: re-run `make release-publish` to reconcile the remaining tags and the hosted Release. It skips tags that already exist rather than failing, so a rerun is safe.
- **Tags pushed but the hosted Release or its artifacts are missing.** The pushed root tag re-triggers `.github/workflows/release.yml`; re-run that workflow (`gh workflow run release.yml` is not needed — dispatch the failed run from the Actions tab) so GoReleaser re-attaches the source archive, SBOM, and signatures in `keep-existing` mode. Do not create a second Release by hand.
- **A bad version was published.** Do not retag. Cut a new forward-fix version: rotate `CHANGELOG.md`, then `make release-tag` followed by `make release-publish` for the next `vX.Y.Z`. Downstream consumers move forward to the corrected version.

## Supply-chain artifacts (automated)

Pushing a root `vX.Y.Z` tag (including pre-releases like `v0.2.0-alpha.1`) triggers `.github/workflows/release.yml`,
which runs GoReleaser in library mode and produces, for every release:

- a reproducible source archive (`gokit-<version>-source.tar.gz`) and a `checksums.txt`;
- a CycloneDX SBOM (`gokit-<version>.cdx.json`) generated by `cyclonedx-gomod` over the root module
  and its dependencies;
- cosign **keyless** (OIDC) signatures
  and certificates for **every** artifact (source archive, checksums, and SBOM);
  the same job then **verifies** the `checksums.txt` signature with `cosign verify-blob` against the workflow's OIDC identity
  — since `checksums.txt` pins the SHA-256 of every other artifact,
  this transitively establishes their integrity, and the release fails if verification fails;
- a **SLSA build provenance** attestation over the source archive, checksums,
  and SBOM via `actions/attest-build-provenance`, verifiable with `gh attestation verify`.

Every dependency's license is additionally gated against a permissive allow-list on each push (`scripts/check-licenses.sh`);
see [`dependencies.md`](dependencies.md).

To dry-run the artifact build locally without signing or publishing:

```sh
make release-dry VERSION=vX.Y.Z   # goreleaser --snapshot, skips sign + publish
```

gokit ships **libraries only** (no `package main` anywhere), so there are no runnable binaries
and the release publishes no container images; distribution is entirely through the Go module proxy
and pkg.go.dev.
