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

**Always cross-check the Go module proxy — it is the immutable source of truth for "what version is already taken," and Toven does not consult it.** gokit declares no registry, so `toven release status`/`plan` anchor on *reachable git tags only*. If tags are ever deleted or diverge from the proxy, Toven can fail closed ("no reachable release tag") and let you pick a version that is **already permanently published** on the proxy — which downstream consumers have already resolved and which can never be reused or moved. A release tags **every discovered module** (root and each nested sub-module) in lock-step, and each module path is an **independent** proxy cache, so a deleted or divergent tag on any one sub-module can already occupy the target version even when the root returns 404. Before choosing a version, confirm the target `.info` returns 404 (unpublished) for **every** Toven-discovered module path, not just the root:

```bash
# Enumerate every published module path (root + sub-modules) straight from the go.mod
# files Toven tags — these are the exact proxy paths, and each caches versions forever:
mods=$(find . -name go.mod -not -path '*/vendor/*' \
  -exec sed -n 's/^module //p' {} \;)

# Highest version published ANYWHERE — a sub-module may sit ahead of the root, so the max
# must span every path. `sort -V` is NOT SemVer-aware (it orders v1.0.0-alpha *after*
# v1.0.0, the reverse of SemVer), so compare with golang.org/x/mod/semver instead, and fail
# closed (curl -fsS) on any proxy error rather than mistaking it for "nothing published":
tmp=$(mktemp -d) && cat > "$tmp/main.go" <<'GO'
package main

import ("bufio"; "fmt"; "os"; "golang.org/x/mod/semver")

func main() {
	max := ""
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		if v := sc.Text(); semver.IsValid(v) && (max == "" || semver.Compare(v, max) > 0) {
			max = v
		}
	}
	if max == "" {
		fmt.Fprintln(os.Stderr, "no valid versions found on proxy")
		os.Exit(1)
	}
	fmt.Println(max)
}
GO
( cd "$tmp" && go mod init semvermax >/dev/null && go get golang.org/x/mod >/dev/null 2>&1 )
highest=$(
  for mod in $mods; do
    curl -fsS "https://proxy.golang.org/${mod}/@v/list" \
      || { echo "proxy fetch failed: $mod" >&2; exit 1; }
  done | ( cd "$tmp" && go run . )
) || exit 1
echo "highest published across all modules: $highest"

# Your target vX.Y.Z must also return 404 (not yet published) for EVERY module path:
for mod in $mods; do
  code=$(curl -s -o /dev/null -w '%{http_code}' "https://proxy.golang.org/${mod}/@v/vX.Y.Z.info")
  echo "${code}  ${mod}"   # every line must read 404
done
```

The next version must be **strictly greater** (per `semver.Compare`) than `$highest` — and note a prerelease sorts *below* its release (`X.Y.Z-alpha.N < X.Y.Z`), so it must still exceed everything already published. The CHANGELOG can lag or list versions that were never actually pushed; trust the proxy over the CHANGELOG for immutability.

**First-release caveat:** if gokit genuinely has no reachable tag *and* nothing on the proxy, `release status`/`plan` fail closed as expected. `--set-version` is a **`release tag`/`release publish` flag only** (not `release bump`), so supply the explicit first version to the Phase 2 mutating cut — `toven release publish --set-version go:gokit=X.Y.Z --yes` (or `toven release tag --set-version go:gokit=X.Y.Z --yes` to tag without the hosted Release). Toven never fabricates a synthetic `0.0.0` baseline. See `docs/VERSIONING.md`.

## Step 3 — Update the CHANGELOG

1. Open `CHANGELOG.md`.
2. Replace `## [Unreleased]` with `## [X.Y.Z] - YYYY-MM-DD` (stable) or `## [X.Y.Z-alpha.N] - YYYY-MM-DD` (prerelease).
3. Add a fresh empty `## [Unreleased]` section above it.
4. If `[Unreleased]` was empty, **refuse to release** — nothing to ship.
5. Update the link references at the bottom if present.

## Step 4 — Phase 1: stage the version bump through a PR

`toven release bump` rewrites every module's version and the inter-module dependency floors and **stages** the change — it never commits, tags, or pushes.

```bash
toven release bump --yes                       # first release: nothing to bump — skip to Phase 2 with --set-version
# Makefile wrapper: make release-bump
```

Then, still on `main`: rotate the CHANGELOG (Step 3) if not already done, cut a `release/vX.Y.Z` branch carrying the staged bump, open a PR, and merge it after review.

## Step 5 — Phase 2: cut the tags and hosted Release (after the bump PR merges)

Run only from a clean checkout of the merged commit. Toven creates signed path-prefixed tags for the root and every sub-module in lock-step and the hosted GitHub Release with commit-derived notes.

```bash
toven release readiness          # fail-closed go/no-go checks (clean-tree)
toven release publish --dry-run  # mutation-free tag + hosted-Release rehearsal
toven release publish --yes      # cut and push signed tags, then create the hosted Release
# First release only (no reachable tag): pin the explicit version here, e.g.
# toven release publish --set-version go:gokit=X.Y.Z --yes
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
