---
name: release
description: >-
    Cut a release of the gokit multi-module monorepo — decide the semver bump, update the
    CHANGELOG, run the full pre-release gates (build/vet/test/lint/vuln) via toven, and tag every
    module with tag-modules.sh. Use when preparing or publishing a gokit release, tagging modules,
    or checking release readiness.
user-invocable: true
---

# Releasing gokit

gokit is a multi-module repo where **each module needs its own semver git tag** — without proper
tags Go assigns broken pseudo-versions. `tag-modules.sh` tags every module consistently. Full
details live in `docs/RELEASING.md`, `docs/VERSIONING.md`, `policy/SEMVER.md`, and
`policy/DEPRECATION.md`.

## Prerequisites

- Listed in `MAINTAINERS.md` with push access to `kbukum/gokit`.
- On `main`, clean working tree, `git`/`gh`/`go` on `$PATH`.
- Commits GPG-signed (`git config commit.gpgsign true`) — release tags must be signed.

## Step 1 — Full pre-release gate (toven, whole tree)

A release is the one time to run the **complete** gates rather than the affected set. Everything
must be green before tagging:

```bash
toven format-check
toven lint
toven build
toven check                 # go vet
toven test -- -race -count=1 -shuffle=on
toven vuln                  # govulncheck across all modules
toven tidy                  # go mod tidy -diff — must be clean
```

Also run the `review` project audit in a fresh agent before a release. Treat green gates as
necessary but not sufficient.

## Step 2 — Decide the version

```bash
git tag --sort=-v:refname | head -1                 # latest tag
git log --oneline $(git describe --tags --abbrev=0)..HEAD
```

Use `policy/SEMVER.md`. While in `0.x`: a breaking change in the `[Unreleased]` CHANGELOG section
bumps **MINOR**; otherwise **PATCH**.

## Step 3 — Update the CHANGELOG

1. Open `CHANGELOG.md`.
2. Replace `## [Unreleased]` with `## [vX.Y.Z] - YYYY-MM-DD`.
3. Add a fresh empty `## [Unreleased]` section above it.
4. If `[Unreleased]` was empty, **refuse to release** — nothing to ship.
5. Update the link references at the bottom if present.

`tag-modules.sh` refuses to tag if `[Unreleased]` is the only populated section, or if the
`[vX.Y.Z]` section for the version you're cutting is missing.

## Step 4 — Tag every module

```bash
./tag-modules.sh vX.Y.Z            # local-only dry run — inspect the tags it would create
./tag-modules.sh vX.Y.Z --push     # create and push tags to origin
```

Equivalent Makefile wrappers: `make tag VERSION=vX.Y.Z`, `make tag-push VERSION=vX.Y.Z`,
`make list-tags`.

## Step 5 — Publish

Follow the remaining steps in `docs/RELEASING.md` (GitHub release, notes from the CHANGELOG
section, SBOM/provenance if configured). CI actions must be SHA-pinned; artifacts signed.

## Guardrails

- **Never** run destructive git commands (`reset --hard`, `checkout -- .`, `clean`) on uncommitted
  work without explicit permission.
- Per repo workflow, the agent prepares the branch/CHANGELOG/edits; **the maintainer commits,
  pushes, and runs the actual `--push` tagging** unless explicitly asked otherwise. Only create a
  PR when explicitly requested, following the PR template.
- Reference other-repo items with full URLs, never bare `#123`.
