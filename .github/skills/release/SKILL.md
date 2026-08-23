---
name: release
description: >-
    Cut a release of the gokit multi-module monorepo — decide the semver bump, update the
    CHANGELOG, run the full pre-release gates (build/vet/test/lint/vuln) via toven, then stage the
    version bump through a PR (Phase 1) and cut every module tag plus the hosted Release with
    `toven release` (Phase 2). Use when preparing or publishing a gokit release, tagging modules,
    or checking release readiness.
user-invocable: true
---

# Releasing gokit

gokit is a multi-module repo where **each module needs its own semver git tag** — without proper tags Go assigns broken pseudo-versions. **Every release operation goes through Toven** (`toven …`): the pre-release gates are Toven tasks, and `toven release` bumps versions, tags every module in lock-step, and creates the hosted GitHub Release. Never hand-roll `git tag`, `gh release`, or per-module `go` loops. Full details live in `docs/RELEASING.md`, `docs/VERSIONING.md`, `docs/policy/SEMVER.md`, and `docs/policy/DEPRECATION.md`; the Makefile `release-*` targets are thin wrappers over the same `toven release` verbs.

The release is **two-phase**: Phase 1 stages the version + CHANGELOG bump through a reviewed PR so `main` stays the source of truth; Phase 2 cuts the signed tags and hosted Release from the merged commit. Signed tags are never cut off an unmerged bump.

## Prerequisites

- Listed in `MAINTAINERS.md` with push access to `kbukum/gokit`.
- On `main`, clean working tree, `git`/`gh`/`go`/`toven` on `$PATH`.
- Commits GPG-signed (`git config commit.gpgsign true`) — release tags must be signed.

## Step 1 — Full pre-release gate (toven, whole tree)

A release is the one time to run the **complete** gates rather than the affected set.
Run every gate as a bare Toven task (each fans out across all modules); everything must be green before tagging:

```bash
toven format-check
toven lint
toven build
toven check                 # go vet
toven test -- -race -count=1 -shuffle=on
toven vuln                  # govulncheck across all modules
toven tidy                  # go mod tidy -diff — must be clean
```

The dependency **license** gate (`scripts/check-licenses.sh`) stays native (network-bound, not a Toven task) — run it separately. Also run the `review` project audit in a fresh agent before a release. Treat green gates as necessary but not sufficient.

## Step 2 — Decide the version

```bash
toven release status        # declared versions vs published/tagged (read-only)
toven release plan          # preview the version cascade and exact tag set (read-only)
```

Use `docs/policy/SEMVER.md`. While in `0.x`: a breaking change in the `[Unreleased]` CHANGELOG section bumps **MINOR**; otherwise **PATCH**.

**Always cross-check the Go module proxy — it is the immutable source of truth for "what version is already taken," and Toven does not consult it.** gokit declares no registry, so `toven release status`/`plan` anchor on *reachable git tags only*. If tags are ever deleted or diverge from the proxy, Toven can fail closed ("no reachable release tag") and let you pick a version that is **already permanently published** on the proxy — which downstream consumers have already resolved and which can never be reused or moved. Before choosing a version, confirm the highest published version and that your target is unused:

```bash
# Highest published root version (the proxy caches these forever):
curl -s https://proxy.golang.org/github.com/kbukum/gokit/@v/list | sort -V | tail -5
# Your target must return 404 (not yet published):
curl -s -o /dev/null -w '%{http_code}\n' https://proxy.golang.org/github.com/kbukum/gokit/@v/vX.Y.Z.info
```

The next version must be **strictly greater** than the highest proxy version — and note a prerelease sorts *below* its release (`X.Y.Z-alpha.N < X.Y.Z`), so it must still exceed everything already published. The CHANGELOG can lag or list versions that were never actually pushed; trust the proxy over the CHANGELOG for immutability.

**First-release caveat:** if gokit genuinely has no reachable tag *and* nothing on the proxy, `release status`/`plan` fail closed as expected. Supply the version explicitly to the mutating action with `--set-version` (e.g. `toven release bump --set-version go:gokit=X.Y.Z --yes`); Toven never fabricates a synthetic `0.0.0` baseline. See `docs/VERSIONING.md`.

## Step 3 — Update the CHANGELOG

1. Open `CHANGELOG.md`.
2. Replace `## [Unreleased]` with `## [X.Y.Z] - YYYY-MM-DD` (stable) or `## [X.Y.Z-alpha.N] - YYYY-MM-DD` (prerelease).
3. Add a fresh empty `## [Unreleased]` section above it.
4. If `[Unreleased]` was empty, **refuse to release** — nothing to ship.
5. Update the link references at the bottom if present.

## Step 4 — Phase 1: stage the version bump through a PR

`toven release bump` rewrites every module's version and the inter-module dependency floors and **stages** the change — it never commits, tags, or pushes.

```bash
toven release bump --yes                       # or --set-version go:gokit=X.Y.Z --yes on the first release
# Makefile wrapper: make release-bump
```

Then, still on `main`: rotate the CHANGELOG (Step 3) if not already done, cut a `release/vX.Y.Z` branch carrying the staged bump, open a PR, and merge it after review.

## Step 5 — Phase 2: cut the tags and hosted Release (after the bump PR merges)

Run only from a clean checkout of the merged commit. Toven creates signed path-prefixed tags for the root and every sub-module in lock-step and the hosted GitHub Release with commit-derived notes.

```bash
toven release readiness          # fail-closed go/no-go checks (clean-tree)
toven release publish --dry-run  # mutation-free tag + hosted-Release rehearsal
toven release publish --yes      # cut and push signed tags, then create the hosted Release
```

`toven release tag --yes` cuts and pushes the signed tags without creating the Release; `toven release publish --yes` performs the full tag → push → hosted-Release sequence **idempotently** (safe to re-run to reconcile a partial push). Add `--no-push` to keep tags local for inspection. Makefile wrappers: `make release-readiness`, `make release-publish-dry-run`, `make release-tag`, `make release-publish`, `make list-tags`.

## Step 6 — Supply-chain artifacts (automated)

Pushing the root `vX.Y.Z` tag triggers `.github/workflows/release.yml`, where GoReleaser (in `keep-existing` mode) attaches the source archive, checksums, SBOM, cosign signatures, and SLSA provenance to the Toven-created Release. Do not create a second Release by hand. CI actions must be SHA-pinned; artifacts signed. See `docs/RELEASING.md`.

## Guardrails

- **All release operations go through Toven** — never substitute raw `git tag`, `gh release create`, or manual `go mod` version edits for a `toven release` verb.
- Published version tags are **immutable** — never delete, move, or force-push a `vX.Y.Z` (or path-prefixed submodule) tag. Recover by fixing forward (a new version), never by rewriting history.
- **Never** run destructive git commands (`reset --hard`, `checkout -- .`, `clean`) on uncommitted work without explicit permission.
- Per repo workflow, the agent prepares the branch/CHANGELOG/edits; **the maintainer commits, pushes, and runs the actual mutating `toven release … --yes`** unless explicitly asked otherwise. Only create a PR when explicitly requested, following the PR template.
- Reference other-repo items with full URLs, never bare `#123`.
