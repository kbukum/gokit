# Toven migration status

[Toven](https://github.com/kbukum/toven) is gokit's argv-first task and release orchestrator. This document tracks how much of gokit's development and CI surface is driven by Toven, records the command mappings for what remains native, and enumerates the Toven feature gaps that still block full parity. File the gaps upstream in the Toven repo.

gokit's release model is **single-set lock-step, path-prefixed, registry-less**: every discovered `go.mod` module shares one version and is tagged together each release — the root module as `vX.Y.Z`, every sub-module as `<path>/vX.Y.Z`. There is no registry upload; in Go the pushed tag *is* the release, and the module proxy resolves each nested module by its own path-prefixed tag. See [`VERSIONING.md`](VERSIONING.md) and [`RELEASING.md`](RELEASING.md) for the full policy and mechanics.

## Adoption at a glance

| Area | Driver | Status |
|---|---|---|
| Release (plan, status, readiness, bump, tag, publish) | `toven release …` | Migrated |
| Lock-step path-prefixed tagging of every `go.mod` | `toven release plan` / `publish` | Migrated |
| Read-only self-hosting canary (modules, graph, release previews) | `toven` via `toven-canary.yml` | Migrated |
| Structural guardrail (declare-only aggregator / god-file / crowded-package) | `toven run structure` | Migrated |
| Module index generation (`docs/MODULE-INDEX.md`) | `toven run module-index` | Migrated |
| Structural guardrail in CI | pinned `kbukum/toven` action → `make TOVEN=… structure` | Migrated (CI) |
| Go tasks (build/check/test/lint/format/coverage/vuln/tidy) | `toven <task>` | **`make` dev gates drive Toven** for the whole workspace; `M=`/`W=` subsets and the domain-sharded CI stay native (see mappings) |
| Dependency license gate | `scripts/check-licenses.sh` (native) | Native by design — see gap 1 |
| Change → domain matrix | `scripts/affected-domains.sh` (native) | Native — stdin projection helper, see gap 2 |
| Go build/vet/test matrix + lint + `govulncheck` in CI | native `go` / `golangci-lint` / `govulncheck` | Native by design (parity with rskit's native cargo gates) |
| Supply-chain artifacts (source archive, checksums, SBOM, cosign, provenance) | GoReleaser via `release.yml` | Native — attaches to the Toven-created Release (`keep-existing`) |

Toven discovers every `go.mod` (`toven modules` lists them; `toven tasks` lists every task) and owns the authoritative release path. The everyday `make` dev gates (`build`, `test`, `lint`, `vet`, `fmt`, `tidy`, `test-coverage`) now run the **whole workspace** through Toven, which orders modules by the dependency graph and fans out across cores; they fall back to the native `gomod.sh` path only for a `M=<module>` / `W=core|contrib` subset, since Toven has no first-class selector for gokit's named domains yet (gap 3). CI's Go-version-matrix + domain-sharded legs and the network-bound supply-chain gates stay native, exactly as rskit keeps its cargo gates and `cargo-deny` native.

## Command mappings for the native CI steps

These CI steps run native today; each has a direct Toven equivalent (declared in `toven.toml`) that can replace it once the corresponding gap below is closed or a live CI matrix leg is verified.

| Current CI step | Toven equivalent |
|---|---|
| per-module `go test -race … ./...` (check matrix) | `toven test` (per-module fan-out) |
| per-module `golangci-lint run ./...` (lint job) | `toven lint` |
| per-module `govulncheck ./...` (security job) | `toven vuln` |
| per-module coverage merge (check job) | `toven coverage` — Migrated: single whole-workspace run over `github.com/kbukum/gokit/...` covers every module (go.work crosses `go.mod` boundaries that `./...` cannot), gated at the `go:gokit` aggregate |
| `scripts/affected-domains.sh` (change → domain shard) | no direct equivalent — see gap 2 |

The Go build/vet/test/lint/security matrix stays native for the same reason rskit keeps its cargo gates native: CI shards work by a Go version matrix and per-module groups that Toven does not yet model as first-class task variants.

## Toven feature gaps blocking full parity

Prioritized. File each upstream in the Toven repo.

### 1. Per-task execution/probe timeout override — medium priority

The dependency license gate (`scripts/check-licenses.sh`, running `go-licenses` across every module) is network-bound and exceeds Toven's default 30s task/probe timeout, so it cannot be wired as a `command` task today (`error[TIMEOUT]: … timed out after 30s`). rskit keeps its analogous `cargo-deny` gate native for the same class of reason.

**Requested:** a per-task timeout override in config (e.g. `timeout_secs`) so a legitimately slow, network-bound gate can run as a Toven `command` task instead of staying native.

### 2. Named module groups / domains (replaces `domains.toml`) — high priority

gokit groups its modules into named domains (`core`, `patterns`, `crosscutting`, `composition`, `transport`, `auth`, `data`, `ai`, `media`, `infra`, `devtools`) in [`../domains.toml`](../domains.toml). Domains drive CI sharding (`scripts/affected-domains.sh` → job matrix), the targeted local gates (`make check-core`, `make check-ai`, …), and the generated module index. Toven models workspaces, individual modules, and globs — not arbitrary named cross-module groups.

**Requested:** first-class module labels/tags in config, group selectors (`toven test --group ai`), and a groups projection for affected sets (`toven affected <task> --output groups`). This is the same gap rskit filed for its domains.

### 3. Stdin-fed projection task shape — low/medium priority

`scripts/affected-domains.sh` consumes changed files on stdin and emits the affected domain set — a projection, not a pass/fail gate. Toven `command` tasks have no first-class shape for "consume changed-file input, emit a projection", so this stays native.

**Requested:** a projection/affected task kind (or a documented pattern) for change-set → projection helpers.

### 4. Per-task toolchain selection (Go version matrix) — low priority

gokit's CI compiles and tests across a Go version matrix and multiple OSes. This works under Toven by activating a toolchain in the job and using the ambient `go`, but there is no Toven-native "run task X on Go 1.N" knob.

**Requested:** an optional per-task/per-run toolchain pin. (Same as rskit's MSRV gap.)

### 5. Go-native dependency & license policy — low priority

The license allow-list check is generic supply-chain policy (allowed SPDX ids, first-party ignore) that Toven could own natively rather than via a shell script, once gap 1 (timeout) is addressed.

**Requested:** a config-driven license/supply-chain policy surface for the Go adapter.

## What is explicitly not a gap

- **Maintainer-local publish.** gokit deliberately cuts and pushes tags locally (`make release-publish`); the pushed root tag triggers `release.yml`, where GoReleaser attaches supply-chain artifacts to the Toven-created Release in `keep-existing` mode. This is an intentional model, not a shortfall — it is the Go analogue of a maintainer-owned release entrypoint.
- **Native dev gates.** The everyday `make` gates run the whole workspace through Toven; only `M=`/`W=` subsets and CI's Go-version-matrix + domain-sharded legs stay native (Toven models workspaces/globs, not gokit's named domains yet — gap 3). Local `go test` fans out across cores in a single wave: a module's `go test ./...` compiles the packages it imports from other modules on demand through Go's shared, concurrency-safe build cache, so Toven runs `test` unordered rather than paying dependency-wave barriers (measured ~2x faster under `-race`); `build`/`vet` keep dependency ordering for cascade fail-fast on a compile break. rskit keeps its cargo gates native too.
- **Guardrail rule logic.** `check-structure.sh`, `generate-module-index.sh`, `check-licenses.sh`, and `affected-domains.sh` encode gokit-specific structure and policy. Toven orchestrates them (or, for the network-bound and projection ones, defers to them); it should not reimplement the rules.
- **Tag grammar.** The Go path-prefixed tag grammar is fixed and correct; Toven rejects a `tag_format` override, and gokit withholds no module from the lock-step set. This is the intended contract.
