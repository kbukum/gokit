---
name: validate
description: "gokit: Run the repository's build, test, lint, and documentation gates scoped to the change."
user-invocable: true
---

# Validate a change

`toven` is the task runner (config in [`toven.toml`](../../../toven.toml)). It discovers every `go.mod`, orders work by the dependency graph, caches results, and can select only the modules a diff touches. The Makefile targets wrap it; `make help` lists them. Which rules each gate checks is in [Enforcement](../../engineering.md#enforcement).

## Scope to what changed

Full-tree gates are for acceptance, audits, and CI.

```bash
toven plan test                                   # what would run (no execution)
toven explain test --module go:media              # exact argv for one module
toven affected test --base origin/main --merge-base
toven test --base origin/main --merge-base        # run only the affected set
make test-affected                                # same, via make (BASE=main by default)
```

`make test-affected` covers committed, staged, unstaged, and untracked changes plus reverse dependents, with caching off. A missing base fails. It rejects `M=`/`W=`; use `make test M=<module> T=<pattern>` for one module. After changing this dispatch, run `python3 -m unittest discover -s scripts -p test_validation.py`.

## Tasks

| Intent | Command |
|---|---|
| Build | `toven build` |
| Vet | `toven check` |
| Test | `toven test -- -race -count=1 -shuffle=on` |
| Coverage gate | `make test-coverage` |
| Lint | `toven lint` |
| Format check / write | `toven format-check` / `toven format` |
| Tidy check / write | `toven tidy` / `toven tidy-fix` (or `make tidy-check` / `make tidy`) |
| Update dependencies | `make update` |
| Vulnerabilities | `make vuln` |
| Licenses | `make licenses` |
| Structure (advisory) | `make structure` |
| Integration (Docker) | `make test-integration M=<module>` |
| Auth session proof | `make auth-integration` |
| Module index | `make module-index` |
| Workflows | `make actionlint` |

## Select modules

Selectors are `go:<name>`; `toven modules` lists them (sub-modules use `-`, for example `go:messaging-kafka`). `toven groups` lists domains.

```bash
toven test --module go:media
toven lint --module go:auth --module go:authz
toven test --module go:errors --dependents      # plus everything that depends on errors
toven build --module go:server --dependencies   # plus everything server needs
toven test --group transport                    # one domain (`make check-transport` adds build + vet)
```

## Flags and output

- The `test` task does not add `-race`; pass test flags after `--`: `toven test --module go:<name> -- -run TestName -race -shuffle=on -count=1`.
- `toven test --watch` reruns affected tests on change.
- `toven --output jsonl test ...` gives machine-readable results.
- `toven cache stats` inspects the cache; `--no-cache` bypasses it; `--refresh` reruns and rewrites it.
- golangci-lint enforces gofumpt, but `toven format` runs plain gofmt. Run `gofumpt -w <dirs>` before `toven lint`.

## Acceptance

- Minimum for a self-contained change: format check, lint, vet, and race + shuffle tests on the affected modules. Then `make check` for acceptance.
- Runtime test-environment and adapter changes also need [real adapter proof](../../engineering.md#real-adapter-proof): loopback, file-backed SQLite, owned process, and required Postgres consumers, as described in [`testutil/README.md`](../../../testutil/README.md). Declare setup, output, fixture, drain, and cleanup budgets first. Missing Docker or a failed teardown cannot certify acceptance.
- Session and authentication changes run `make auth-integration` and follow [`auth/testhost/README.md`](../../../auth/testhost/README.md).
- Docs-only changes need link and anchor checks, not Go builds.
- Record the final source identity, exact commands, failures, limits, and cleanup. Leave blocked prerequisites explicitly blocked.

Validation does not authorize Git actions. Commit, push, or open a PR only when asked.
