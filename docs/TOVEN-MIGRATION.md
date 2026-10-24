# Toven migration status

[Toven](https://github.com/kbukum/toven) is gokit's argv-first task and release orchestrator. This document tracks how much of gokit's development and CI surface is driven by Toven, what remains native, and the Toven feature gaps that still block full parity. File the gaps upstream in the Toven repo.

gokit's release model is **single-set lock-step, path-prefixed, registry-less**: every discovered `go.mod` module shares one version and is tagged together each release — the root module as `vX.Y.Z`, every sub-module as `<path>/vX.Y.Z`. There is no registry upload; in Go the pushed tag *is* the release, and the module proxy resolves each nested module by its own path-prefixed tag. See [`VERSIONING.md`](VERSIONING.md) and [`RELEASING.md`](RELEASING.md) for the full policy and mechanics.

## Adoption at a glance

| Area | Driver | Status |
|---|---|---|
| Release (plan, status, readiness, bump, tag, publish) | `toven release …` | Migrated |
| Lock-step path-prefixed tagging of every `go.mod` | `toven release plan` / `publish` | Migrated |
| Read-only self-hosting canary (modules, graph, release previews) | `toven` via `toven-canary.yml` | Migrated |
| Structural guardrail (declare-only aggregator / god-file / crowded-package) | `toven run structure` | Migrated |
| Module index generation (`docs/MODULE-INDEX.md`) | `toven run module-index` | Migrated |
| Go tasks (build/check/test/lint/format/coverage/vuln/tidy) | `make` → `toven <task>` | Migrated, locally and in CI; `G=<domain>` narrows through Toven; `M=`/`W=` subsets stay native (packages and alternate `go.work` files) |
| Domain gates (`make check-<domain>`) | `make check G=<domain>` → `toven build/check/test --group <domain>` | Migrated locally; domains are `[groups.*]` in `toven.toml` |
| Race tests with the per-module coverage gate | `make test-coverage` → `toven coverage` | Migrated; CI's Go version matrix is a job matrix (gap 1) |
| Integration suite (`//go:build integration`) | `make test-integration` → `toven test --module …` | Migrated; `scripts/integration_modules.py` picks the modules |
| Fuzz targets | `make fuzz` → `scripts/fuzz.py` | Native discovery; Toven `affected` selects modules on pull requests |
| Dependency license gate | `toven licenses` (per module) or `make licenses` (all modules) → `scripts/check-licenses.sh` | Toven task declared; `make licenses` and CI still call the script directly because the CI `licenses` job does not install Toven |
| Cross-OS checks (macOS) | `make check M=<module>` | Native subset path |
| Supply-chain artifacts (source archive, checksums, SBOM, cosign, provenance) | GoReleaser via `release-artifacts.yml` | Native — attaches to the Toven-created Release (`keep-existing`) |

Toven discovers every `go.mod` (`toven modules` lists them; `toven tasks` lists every task) and owns the authoritative release path. The `make` gates run the **whole workspace** through Toven, which orders modules by the dependency graph and fans out across cores within the configured compute budget. CI calls the same `make` targets with a pinned Toven release, so a green local run predicts a green CI run. `G=<domain>` narrows a gate to one domain through Toven. The `M=<module>` / `W=core|contrib` subsets use the native `gomod.sh` path: `M=` also accepts package paths and `W=` switches `GOWORK`, and Toven selects modules, not packages or alternate `go.work` files.

Domains are Toven groups. `toven groups` lists them, `docs/MODULE-INDEX.md` is generated from that list, and Toven fails any run whose real module graph breaks a group's `allow_groups` layering. Package layering inside the root module is described in [Architecture](ARCHITECTURE.md); depguard bans only selected edges. CI runs whole-workspace gates; `--group` is for local, scoped runs.

## How CI uses Toven

- **Coverage.** `toven coverage` runs the per-module `coverage` task (`scripts/go-cover.sh`: `-race -shuffle=on`, honouring each module's `.coverpkg`). Profiles land in `target/toven/coverage/`; Toven blocks any module below its line floor in `[ecosystems.go.coverage]`: test-support modules are measured but not gated, and the few modules still below the floor carry a per-module ratchet. The same files go to Codecov.
- **Fuzz.** `scripts/fuzz.py plan` splits every `FuzzXxx` target into balanced shards that CI runs as a matrix, so each target runs exactly once. On pull requests it fuzzes only modules from `toven affected test --base <base> --merge-base`.
- **Integration.** `make test-integration` runs `toven test --no-cache` over the modules that hold integration tests.
- **Tidy.** `make tidy-check` runs the read-only `tidy` task (`go mod tidy -diff`), after `make replace-check` proves every reachable intra-repo module has a local `replace`.

## Toven feature gaps blocking full parity

Prioritized. File each upstream in the Toven repo.

### 1. Per-task toolchain selection (Go version matrix) — low priority

gokit's CI compiles and tests across a Go version matrix and multiple OSes. This works under Toven by activating a toolchain in the job and using the ambient `go`, but there is no Toven-native "run task X on Go 1.N" knob.

**Requested:** an optional per-task/per-run toolchain pin. (Same as rskit's MSRV gap.)

### 2. Go-native dependency & license policy — low priority

The license allow-list check is generic supply-chain policy (allowed SPDX ids, first-party ignore) that Toven could own natively rather than via a shell script.

**Requested:** a config-driven license/supply-chain policy surface for the Go adapter.

## Resolved gaps

- **Task timeout.** Toven tasks have no default wall-clock limit (`--timeout` is opt-in), so the network-bound license gate runs as the `licenses` task.
- **Named domains.** Domains are `[groups.*]` with `description` and `guardrails.allow_groups`; `--group` selects them and `toven groups` lists them. `domains.toml` and `scripts/check-domain.sh` are gone.

## What is explicitly not a gap

- **CI-dispatched publish with a local fallback.** The canonical publish runs in CI through the `Release (publish)` workflow (`release.yml`, `workflow_dispatch`, protected `release` environment) — `toven release publish` from `main`, mirroring Toven's own release pipeline; `make release-publish` remains a maintainer-local fallback. Either way the pushed root tag triggers `release-artifacts.yml`, where GoReleaser attaches supply-chain artifacts to the Toven-created Release in `keep-existing` mode. This is an intentional two-workflow model (publish vs. artifacts), not a shortfall.
- **Single-wave tests.** Local `go test` fans out across cores in a single wave: a module's `go test ./...` compiles the packages it imports from other modules on demand through Go's shared, concurrency-safe build cache, so Toven runs `test` unordered rather than paying dependency-wave barriers (measured ~2x faster under `-race`); `build`/`vet` keep dependency ordering for cascade fail-fast on a compile break.
- **Fuzz target discovery.** Fuzz targets are Go functions, not modules or tasks; `scripts/fuzz.py` owns discovery and sharding while Toven owns module selection.
- **Guardrail rule logic.** `check-structure.sh`, `generate-module-index.sh`, and `check-licenses.sh` encode gokit-specific structure and policy. Toven orchestrates them; it should not reimplement the rules.
- **Tag grammar.** The Go path-prefixed tag grammar is fixed and correct; Toven rejects a `tag_format` override, and gokit withholds no module from the lock-step set. This is the intended contract.
