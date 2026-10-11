# Releasing

Releases have two phases: review the version bump, then publish the merged commit. Toven owns version changes, module tags, and the hosted Release; GoReleaser attaches supply-chain artifacts.

See [Versioning](VERSIONING.md) for tag conventions and [SemVer](policy/SEMVER.md) for compatibility rules.

## Prerequisites and gates

Use authorized maintainer access and a clean, current `main`. Publishing through Actions requires access to the protected `release` environment; local publishing requires the configured signing key. Use the repository's declared Go and pinned tools.

Before release, run the complete gates:

```bash
toven format-check
toven lint
toven build
toven check
toven test -- -race -count=1 -shuffle=on
toven vuln
toven tidy
scripts/check-licenses.sh
```

Review the project as well. Green commands do not replace architecture, lifecycle, or security review.

## Choose an unused version

Inspect `toven release status` and `toven release plan`, then apply the SemVer policy. Modules without a previous tag need an explicit version; `--set-version` takes no `v` prefix.

**Check the Go proxy for every discovered module.** Published versions remain immutable even if Git tags are missing. The chosen version must exceed the highest published version across all modules, and its `.info` endpoint must return 404 for every module. Other HTTP failures do not mean a version is available.

This read-only query uses the repository's SemVer helper rather than version-string sorting:

```bash
mods=$(find . -name go.mod -not -path '*/vendor/*' -exec sed -n 's/^module //p' {} \;)
highest=$(
  for mod in $mods; do
    curl -fsS "https://proxy.golang.org/${mod}/@v/list" \
      || { echo "proxy fetch failed: $mod" >&2; exit 1; }
  done | go run ./scripts/semver-max
) || exit 1
printf 'Highest published version: %s\n' "$highest"
```

Check the selected `vX.Y.Z` at `https://proxy.golang.org/<module>/@v/vX.Y.Z.info`. Prereleases sort below their corresponding stable release.

## Prepare the bump

1. Create a release branch from current `main`.
2. Run `make release-bump SET_VERSION=<version>`; omit `SET_VERSION` when history establishes every module's version.
3. Rotate the populated `[Unreleased]` changelog section into a dated release section and add a new empty `[Unreleased]` section. An empty release section blocks publication.
4. Review, commit, and merge the version/dependency bump and release notes through a PR.

The bump stages changes but does not commit, tag, or push. `make replace-sync` maintains local dependency replacements; `make replace-check` verifies them. Use these if tidying an unpublished bump attempts to download another gokit module.

Release notes summarize capabilities, fixes, and meaningful breaking changes. Link detailed guidance rather than embedding implementation recipes.

## Publish the merged commit

Run the **Release (publish)** Actions workflow against `main`:

1. Keep `dry_run` checked and review the uploaded `release-preview` artifact.
2. Dispatch with `dry_run` unchecked only after review and authorization. The protected `release` environment gates publication.

Set `set_version` when an explicit version is needed. CI creates annotated bot tags; environment approval, artifact signatures, and provenance establish integrity.

For an authorized local fallback, use a clean checkout of the merged commit and configured signing:

```bash
make release-readiness
make release-plan
make release-publish-dry-run
make release-publish
```

Pass the same `SET_VERSION` to commands that require it. `make release-tag` creates and pushes tags without the hosted Release. Never bypass clean-tree checks or replace Toven with manual tag/release commands.

## Verify publication

- Confirm root and path-prefixed module tags identify the intended commit.
- Confirm the hosted Release and resolve the root and nested modules through the Go proxy.
- Confirm source archive, checksums, SBOM, cosign signatures, and SLSA provenance.
- Verify provenance with `gh attestation verify` and the artifact signatures against the release workflow's identity.

The root tag triggers `release-artifacts.yml`. GoReleaser attaches artifacts to the existing Release; it must not create a competing Release or replace its notes. `make release-dry VERSION=vX.Y.Z` previews artifact generation without signing or publication. Distribution is through Go modules, not container images.

## Recovery and hotfixes

Inspect `toven release status`, remote tags, and proxy versions first. Reconcile a partial publication only when Toven accepts the existing state; divergent tags require forward-fix guidance, not forced replacement. Retry failed artifact jobs against the existing Release.

Published tags are immutable: never delete, move, or force-push them. A bad release requires a new version. Older-line hotfixes use a dedicated branch and the same review, gates, preview, and publication requirements. Announce completed releases through the project's normal channels.
