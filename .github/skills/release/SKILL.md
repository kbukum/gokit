---
name: release
description: >-
    Cut a release of the gokit multi-module monorepo — decide the semver bump, update the
    CHANGELOG, run the full pre-release gates (build/vet/test/lint/vuln) via toven, and cut every
    module tag with `toven release`. Use when preparing or publishing a gokit release, tagging
    modules, or checking release readiness.
user-invocable: true
---

# Releasing gokit

gokit is a multi-module repo where **each module needs its own semver git tag** — without proper tags Go assigns broken pseudo-versions. Toven (`toven release`) tags every module consistently and creates the hosted GitHub Release. Full details live in `docs/RELEASING.md`, `docs/VERSIONING.md`, `docs/policy/SEMVER.md`, and `docs/policy/DEPRECATION.md`.

## Prerequisites

- Listed in `MAINTAINERS.md` with push access to `kbukum/gokit`.
- On `main`, clean working tree, `git`/`gh`/`go`/`toven` on `$PATH`.
- Commits GPG-signed (`git config commit.gpgsign true`) — release tags must be signed.

## Step 1 — Full pre-release gate (toven, whole tree)

A release is the one time to run the **complete** gates rather than the affected set.
Everything must be green before tagging:

```bash
toven format-check
toven lint
toven build
toven check                 # go vet
toven test -- -race -count=1 -shuffle=on
toven vuln                  # govulncheck across all modules
toven tidy                  # go mod tidy -diff — must be clean
```

Also run the `review` project audit in a fresh agent before a release.
Treat green gates as necessary but not sufficient.

## Step 2 — Decide the version

```bash
toven release plan          # preview the version cascade and exact tag set
toven release status        # declared versions vs existing tags
```

Use `docs/policy/SEMVER.md`. While in `0.x`:
a breaking change in the `[Unreleased]` CHANGELOG section bumps **MINOR**; otherwise **PATCH**.

## Step 3 — Update the CHANGELOG

1. Open `CHANGELOG.md`.
2. Replace `## [Unreleased]` with `## [vX.Y.Z] - YYYY-MM-DD`.
3. Add a fresh empty `## [Unreleased]` section above it.
4. If `[Unreleased]` was empty, **refuse to release** — nothing to ship.
5. Update the link references at the bottom if present.

## Step 4 — Readiness, then cut the tags and hosted Release

Toven owns tagging and the hosted Release; the pushed root tag then triggers GoReleaser to attach the supply-chain artifacts.

```bash
toven release readiness          # fail-closed go/no-go checks
toven release publish --dry-run  # mutation-free registry + hosted-Release rehearsal
toven release publish --yes      # cut and push signed tags, then create the hosted Release
```

Equivalent Makefile wrappers: `make release-readiness`, `make release-publish-dry-run`,
`make release-publish`, `make list-tags`.

## Step 5 — Publish artifacts

Pushing the root tag triggers `.github/workflows/release.yml`, where GoReleaser (in `keep-existing` mode) attaches the source archive, checksums, SBOM, signatures, and provenance to the Toven-created Release. See `docs/RELEASING.md`.
CI actions must be SHA-pinned; artifacts signed.

## Guardrails

- **Never** run destructive git commands (`reset --hard`, `checkout -- .`, `clean`) on uncommitted work without explicit permission.
- Per repo workflow, the agent prepares the branch/CHANGELOG/edits; **the maintainer commits, pushes, and runs the actual `toven release publish --yes`** unless explicitly asked otherwise. Only create a PR when explicitly requested, following the PR template.
- Reference other-repo items with full URLs, never bare `#123`.
