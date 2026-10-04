# Review project

Standing, re-runnable **whole-toolkit audit**, independent of any diff. Use it periodically, before a release, when onboarding to a module, or whenever you want assurance the tree as a whole still honors the baseline. It sequences the same eight focused passes in [`references/`](./) but over the existing code rather than a change set.

## Execution

Follow [the review skill](../SKILL.md): direct review by default; independent agents only on request. Read current source and relevant contracts. A plan is a scope checklist, not a justification for a baseline violation.

## Scope first to keep the audit tractable

The whole tree is large. Prefer auditing **one domain
or module at a time** rather than everything at once:

- a single package or domain (`errors`, `auth/`, the `data` domain),
- a whole workspace (`core.go.work` vs `contrib.go.work` members), or
- the full tree only when you have time for the slow gates.

State the chosen surface up front so findings are bounded.

## Pass 0 — Scope and context

- Get a structural picture before diving in: list modules and their dependency edges,
  skim each package tree.

```bash
ls -d */                                             # top-level modules/packages
for m in */go.mod; do echo "== $m =="; grep -E '^\s+github.com/kbukum/gokit' "$m"; done
```

## Passes

Follow the trigger table and order in [the review skill](../SKILL.md). Use each checklist's project scope. Load applicable files only; report incomplete checks and stop acceptance on structural/reuse blockers.

## Findings

Record every finding as:

```
severity (blocker / should-fix / nit) — file:line — what's wrong — which principle — suggested fix
```

Group findings by module and by pass so the report is actionable.
See [`SKILL.md`](../SKILL.md) for severity definitions.

## Validation

A full audit is the place for the slow, complete gates:

```bash
make fmt
make lint                 # whole-tree golangci-lint (or M=<module>)
make build
make vet
make test                 # -race -count=1
make test-coverage        # coverage gate
make check                # full canonical gate
govulncheck ./...         # per module via ./gomod.sh cmd "govulncheck ./..."
```

A green `make check` is necessary but **not sufficient** — goroutine leaks, missing timeouts/cancellation, unbounded channels, global-registry composition smells, duplicated owners, and boundary-validation gaps are on the reviewer, not the gate.
