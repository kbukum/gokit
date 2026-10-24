# Contributing to gokit

gokit is a pre-1.0 alpha. Contributions are welcome. The rules every change follows are in the [engineering standard](.github/engineering.md); this file covers the workflow.

## Prerequisites

Tool versions are pinned in [`go.mod`](go.mod) and [CI](.github/workflows/ci.yml). Install the same versions locally.

- **Go** — the version in `go.mod`.
- **[Toven](https://github.com/kbukum/toven)** — runs the workspace `make` gates. Install with `brew tap kbukum/tap && brew install toven` or see [installation](https://github.com/kbukum/toven/blob/main/docs/installation.md).
- **Python 3** — CI helper scripts (`scripts/*.py`).
- **golangci-lint v2**, **govulncheck**, **go-licenses** — for `make lint`, `make vuln`, and `make licenses`.
- **shellcheck** — used by `make actionlint` for workflow `run:` scripts.
- **ast-grep** — for the advisory `make structure` guard; it installs itself if missing.
- **Docker** — for `make test-integration` and `make ci` (local CI via [act](https://github.com/nektos/act)).

## Getting started

```bash
git clone https://github.com/kbukum/gokit.git
cd gokit
make check   # build + vet + test across all modules
make lint    # golangci-lint across all modules
```

## Daily workflow

Scope commands to what you changed. `make help` lists every target.

```bash
make check-fast M=<module>         # build + vet + lint
make test M=<module> T=<pattern>   # one module, race + shuffle
make test-affected                 # changed modules and their dependents
make check-<domain>                # one domain; `toven groups` lists them
make fmt M=<path>                  # format
make tidy M=<module>               # go mod tidy (CI runs make tidy-check)
make check                         # full validation before a PR
```

`gomod.sh` runs a command across discovered modules, for example `./gomod.sh cmd "go test" -m messaging`.

## Making a change

1. Branch from `main`.
2. Make the change and its tests together, following the [engineering standard](.github/engineering.md).
3. Run the scoped commands above while you work, then `make check` and `make lint`.
4. Update godoc, READMEs, and the `[Unreleased]` section of [CHANGELOG.md](CHANGELOG.md).
5. Open a pull request and complete the template. The [Definition of Done](.github/engineering.md#definition-of-done) is the bar.

## Adding a package or module

Decide placement with [Architecture](docs/ARCHITECTURE.md) and the [concern owners](docs/concern-owners.md) first.

- **Root-module package** (no heavy dependencies): create `yourpkg/` with a declare-only `doc.go` and tests. Import only the standard library, lower-layer packages, and lightweight dependencies.
- **Sub-module or nested adapter** (heavy dependencies): create the folder with its own `go.mod` using the root module's `go` directive, then:
  - run `make replace-sync` to add local `replace` directives for gokit modules it uses (`make replace-check` verifies them);
  - add it to `go.work` and to `core.go.work` or `contrib.go.work`;
  - add it to its domain group in [`toven.toml`](toven.toml) and run `make module-index`;
  - implement `component.Component` if it wraps infrastructure with a lifecycle.

Agents use the `new-module` and `new-backend` skills for the same steps.

## Versioning and releases

All modules are tagged together at the same version, per module path (`v0.2.0`, `cache/v0.2.0`). Toven creates the tags. See [Versioning](docs/VERSIONING.md) and [Releasing](docs/RELEASING.md).

## CI

CI calls the same `make` targets you run locally, with a pinned Toven release. Modules and fuzz targets are discovered at runtime.

- **Gate** — Go version, changelog, CI script tests, workflow lint, replace graph, tidiness, and the fuzz plan. Docs-only pull requests skip the Go jobs but still check the changelog.
- **Check** — `make build vet test-coverage` on the `go.mod` floor and the latest Go. A module below its coverage floor fails; coverage also goes to Codecov.
- **Lint** (plus the advisory `make structure`), **Security** (`make vuln`), **Licenses**, **Integration** (`make test-integration`), and macOS checks for platform-sensitive modules.
- **Fuzz** — pull requests fuzz affected modules, `main` fuzzes everything, and a nightly run fuzzes longer. Failing inputs are uploaded as artifacts.
- **CI Status** — the single required check; it fails unless every job that should run succeeded.

Which rules these gates check, and which they don't, is in [Enforcement](.github/engineering.md#enforcement).

Run CI locally with `make ci` (Docker and act), `make ci-test`, or `make ci-lint`.

## Sibling parity

Shared abstractions (`AppError`, `Component`, `Provider`, `Stream`, lifecycle hooks) are tracked for parity with [rskit](https://github.com/kbukum/rskit). When you change one here, open a tracking issue in rskit so the change can be evaluated there.

## Related documents

- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) — Contributor Covenant v2.1
- [SECURITY.md](SECURITY.md) — vulnerability reporting and supply chain
- [GOVERNANCE.md](GOVERNANCE.md) — roles, decisions, sibling-parity contract
- [MAINTAINERS.md](MAINTAINERS.md) — maintainers and areas
- [docs/policy/SEMVER.md](docs/policy/SEMVER.md) · [docs/policy/DEPRECATION.md](docs/policy/DEPRECATION.md)
- [docs/adr/](docs/adr/) — architecture decision records
