# Review project

Standing, re-runnable **whole-toolkit audit**, independent of any diff. Use it periodically, before a release, when onboarding to a module, or whenever you want assurance the tree as a whole still honors the baseline. It sequences the same eight focused passes in [`references/`](./) but over the existing code rather than a change set.

## Run this in a separate, clean-context agent

**Always dispatch this audit to a fresh agent with no shared session context.** The point of a full audit is an independent read of the code as it exists — not filtered through whatever a prior session believed about it. Do not run it inline in a session that has been editing the same code.

- Hand the agent: the module(s)/domain to audit (or "the whole tree"), this file,
  and the [`references/`](./) folder.
- The agent judges the code as written,
  against the principles in [`.github/copilot-instructions.md`](../../../copilot-instructions.md) —
  not against any session's recollection.
- **Optional roadmap check.** If there is a roadmap or versioning plan (e.g. under a `tmp/<plan-name>/` folder, `docs/VERSIONING.md`), pass it in *as context for intended state only* — "here is where the toolkit is meant to be; flag where the tree has not caught up." It frames expectations; it never excuses a baseline violation.

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

## Passes — run in order

Work the focused files top to bottom;
each carries a "Project mode" scope note describing how to sweep the tree for that lens.

1. [`00-structure-placement.md`](./00-structure-placement.md) — module/package placement,
   acyclic layering, `doc.go`/go.work wiring across the tree.
2. [`01-canonical-reuse.md`](./01-canonical-reuse.md) — sweep for local forks of an owned concern.
   *(blocker class)*
3. [`02-principles.md`](./02-principles.md) — typed/minimal, errors & resilience, concurrency,
   composition, up-to-date idioms, AI features across the full surface.
4. [`03-security-privacy.md`](./03-security-privacy.md) — trust-boundary validation,
   injection safety, token handling, crypto, data minimization.
5. [`04-quality.md`](./04-quality.md) — root-cause over patches, dead code,
   file/package organization, outdated patterns, style gates.
6. [`05-tests-tdd.md`](./05-tests-tdd.md) — coverage of behavior and failure paths, determinism,
   clock/env/cwd discipline, fixtures.
7. [`06-docs-supply-chain.md`](./06-docs-supply-chain.md) — `doc.go`/godoc, Conventional Commits,
   `go.sum`, `govulncheck`, SHA-pinned actions, SBOM/provenance.
8. [`07-comments-godoc.md`](./07-comments-godoc.md) — sweep all source prose: comments
   and godoc describe the current code, not plans/history; rewrite or delete the rest.

When you only need one lens across the project (e.g. a standalone security or TDD sweep),
run that focused file directly with its "Project mode" note.

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
