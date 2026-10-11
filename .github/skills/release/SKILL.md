---
name: release
description: "gokit: Prepare or publish a release through the repository's version, validation, and supply-chain gates."
user-invocable: true
---

# Prepare or publish a release

Follow [Releasing](../../../docs/RELEASING.md), [Versioning](../../../docs/VERSIONING.md), and the [SemVer policy](../../../docs/policy/SEMVER.md). Toven owns every release mutation; never substitute manual tags, hosted releases, or version rewrites.

## Acceptance

- Use a clean, current `main` and authorized maintainer access.
- Run full format, lint, build, vet, race/shuffle test, vulnerability, tidy, and dependency-license gates. Review the project directly unless an independent reviewer is requested.
- Verify the highest published version and target availability on the Go proxy for every discovered module, not just the root. Git tags and changelog entries cannot establish proxy availability.
- Choose a version strictly above all published versions; published tags are immutable.
- Keep release notes concise and high-level, with meaningful breaking changes preserved.

## Two phases

1. **Prepare:** create a release branch, run `toven release bump`, rotate the populated changelog section, and submit the staged version/dependency changes for review. An empty changelog blocks release. Commit, push, and open a PR only with authorization.
2. **Publish:** after that PR merges, preview the `Release (publish)` Actions workflow against `main`. Review its artifact, then authorize publishing through the protected `release` environment.

The local fallback uses the same Toven readiness and dry-run gates on the merged commit; configure signing as required. CI uses annotated bot tags, protected approval, and signed supply-chain artifacts.

Use `--set-version` without a `v` prefix when an explicit first version is needed. Discover modules rather than retaining a module count or stale release baseline.

Verify the complete tag set, hosted Release, proxy resolution, checksums, SBOM, signatures, and provenance. Reconcile partial publication through the canonical procedure; fix bad published versions forward, never delete or move them. Do not bypass clean-tree or approval gates, discard edits, or publish implicitly.
