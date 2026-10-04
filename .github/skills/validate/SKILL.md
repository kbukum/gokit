---
name: validate
description: "gokit: Run the repository's build, test, lint, and documentation gates scoped to the change."
user-invocable: true
---

# Validating gokit changes with toven

gokit is a multi-module Go monorepo. **`toven` is the canonical task runner** (config in `toven.toml`, sibling repo `../toven`). It discovers every `go.mod`, orders work by the dependency graph, caches results, and can scope to just the modules a diff touched. Prefer it over calling `go`/`make`/`gomod.sh` by hand — the Makefile targets are thin wrappers and toven gives you affected-set detection and per-module caching for free.

## Golden rule: scope to what changed

Never run the whole tree for a small change. Let toven compute the affected set,
or name the module explicitly. Full-tree gates are for audits and CI sign-off (see `review`).

```bash
# What would this task run, and where? (no execution — reviewable argv)
toven plan test
toven explain test --module go:media          # exact argv for one module

# Only the modules the diff touches (blast radius incl. reverse-dependents)
toven affected test --base origin/main --merge-base
toven test --base origin/main --merge-base    # run just those
```

## Core tasks (bare task name = run it)

| Intent | Command | Notes |
|---|---|---|
| Build | `toven build` | `go build ./...` per module |
| Vet | `toven check` | `go vet` per module |
| Test | `toven test -- -race -count=1 -shuffle=on` | see race note below |
| Lint | `toven lint` | golangci-lint per module |
| Format (write) | `toven format` | `gofmt -w` — but see gofumpt note |
| Format (check) | `toven format-check` | fails on unformatted files |
| Tidy (check) | `toven tidy` | `go mod tidy -diff` |
| Tidy (write) | `toven tidy-fix` | `go mod tidy` |
| Update dependencies | `make update` | Toven runs `go get -u -t ./...`, then tidy |
| Vuln scan | `toven vuln` | `govulncheck` per module |
| Structure | `make structure` | declare-only aggregator guard (`doc.go` docs-only + god-file advisory) |

## Scoping to modules

Module selectors are `go:<name>` (run `toven modules` for the list; sub-modules use `-`, e.g. `go:media`, `go:auth`, `go:messaging-kafka`, `go:database-sqlite`).
Flags are repeatable.

```bash
toven test --module go:media                       # one module
toven lint --module go:auth --module go:authz      # several
toven test --module go:errors --workspace          # + everything that depends on errors
toven build --module go:server --dependencies      # + everything server needs
```

## Race, determinism, and passthrough

The `test` task's default selector does **not** include `-race`;
pass test flags verbatim after `--`. gokit's baseline requires green under race + shuffle:

```bash
toven test --module go:<name> -- -race -count=1 -shuffle=on
toven test --module go:<name> -- -run TestName -race     # single test
toven test --watch                                        # rerun affected tests on change
```

## gofumpt before lint (important)

golangci-lint enforces **gofumpt** (stricter than gofmt), but `toven format` runs plain `gofmt -w`.
After editing, run gofumpt on the changed dirs, then lint:

```bash
gofumpt -w <changed-dir>...
toven lint --module go:<name>
```

## Machine-readable output for agents

When you need to parse results programmatically rather than read a terminal table:

```bash
toven --output jsonl test --base origin/main --merge-base
```

## Cache

toven caches per-unit results. Use `toven cache stats` to inspect, `--no-cache` to bypass a run,
`--refresh` to re-run but rewrite the cache.

## Before you hand work off

For a self-contained change, the minimum green bar is: `format-check`/gofumpt, `lint`, `check` (vet), and `test -- -race -count=1 -shuffle=on` on the affected modules. Escalate to a full-tree run only when the affected set is genuinely tree-wide or you are preparing a release.

For test-environment/runtime ownership changes, also run the real loopback, file-backed SQLite, owned-process, and required Postgres consumers described in [`testutil/README.md`](../../../testutil/README.md). Declare numeric setup, output, fixture, drain, and cleanup budgets before implementation. Unit doubles and the fake migration driver prove failure orchestration, not production adapters. Missing Docker or a forced/failed teardown cannot certify acceptance. Record final source identity, exact commands, failure cases, measured limits, and resource cleanup; leave incomplete prerequisites explicitly blocked.

Per repo workflow, **create the branch and make edits only** — the maintainer commits and pushes.

For session/authentication changes, run `make auth-integration` and follow [`auth/testhost/README.md`](../../../auth/testhost/README.md). This includes the built HTTPS host, actual SQL migrations and required PostgreSQL proof, not just handler mocks. Verify exact trusted readiness, cookie flags, unsafe-method CSRF, same-principal API keys, duplicate/mixed/invalid credentials, store failure, rotation/logout races and measured stream teardown. Test-only controls and credentials must not appear in production dependencies or retained evidence. Browser consumers separately prove terminal session handling and late-response fencing.
