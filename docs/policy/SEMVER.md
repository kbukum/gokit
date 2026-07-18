# Semantic Versioning Policy

`gokit` follows [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html) with the multi-module clarifications below.

## Versioning surface

`gokit` is a Go *workspace* with one root module (`github.com/kbukum/gokit`) and 33 sub-modules (e.g. `github.com/kbukum/gokit/storage`, `…/server`, …). **Each module is versioned independently**, even though we currently cut all tags in lock-step. The lock-step practice is convenience, not contract — consumers should pin per module.

## Pre-1.0 (`0.x.y`)

While the project is in `0.x.y`:

- **MINOR** (`0.X.0`) bumps **may** contain breaking API changes. Every break is documented in `CHANGELOG.md` under `### Changed (Breaking API Changes)` for the affected module.
- **PATCH** (`0.x.Y`) bumps are bug fixes, performance improvements, internal refactors, and **non-breaking** additions. PATCH releases never break the public API.
- We will not promote a module to `1.0.0` until its public API is settled and we are willing to commit to the full `1.x` compatibility contract for at least 12 months.

## Post-1.0 (`1.x.y` and beyond)

- **MAJOR** (`X.0.0`) — breaking change to a stable public API. Requires a deprecation cycle (see `DEPRECATION.md`) of at least one MINOR release before the breaking change ships.
- **MINOR** (`x.Y.0`) — backwards-compatible additions and behaviour changes. Marking an API as deprecated is a MINOR change.
- **PATCH** (`x.y.Z`) — backwards-compatible bug and security fixes only.

## What counts as the public API

For a Go module, the public API is every exported identifier reachable via `go doc <module>/...` from the `main` branch. This includes:

- Exported types, functions, methods, constants, variables.
- The signatures and observable behaviour of all of the above.
- Documented invariants in package or symbol doc comments.
- Declared interfaces — adding a method to an exported interface is a break.

The following are explicitly **not** part of the public API and may change in any release:

- Anything in an `internal/` package.
- Test helpers in `*/testutil/` packages — these track the parent module version but make no API-stability promises.
- Generated code (`*.pb.go`, mock files) — when the upstream IDL/source changes.
- Dependency versions, beyond the documented minimum Go toolchain version.

## Module-level version skew

Sub-modules may temporarily be at different versions when a focused fix ships (e.g. `storage/v0.2.1` while the rest of the workspace stays at `v0.2.0`). The next root-level release brings everything back into lock-step.

## Pre-release identifiers

Pre-releases use SemVer suffixes: `v0.3.0-rc.1`, `v0.3.0-beta.2`. Pre-release tags do not require CHANGELOG entries but **must** be reproducible builds (no moving Go-toolchain reference, no floating action SHAs).

## See also

- `DEPRECATION.md` — how we deprecate and eventually remove APIs.
- `../RELEASING.md` — the mechanical release process.
- `../../GOVERNANCE.md` — who can cut a release and how.
