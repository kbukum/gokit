# Review changes

Standing, re-runnable review of a **change set** in this repository — a branch, a commit range,
or `HEAD~1`. Use it after every change set, especially fast/"vibe-coded" work. It sequences the
eight focused passes in [`reviews/`](./) over a diff and adds scope handling; the actual checks
live in the focused files.

## Run this in a separate, clean-context agent

**Always dispatch this review to a fresh reviewer agent with no shared session context.** A
reviewer that "remembers" writing the code rationalizes it; an independent agent re-derives
every judgment from the diff and the principles. Do not run it inline in the same session that
produced the change.

- Hand the reviewer agent: the diff (or base ref), this file, and the [`reviews/`](./) folder.
  Nothing else from the authoring session.
- The reviewer reads the code as-is; it does not trust prior reasoning about why the code
  "should" be correct.
- **Optional plan check.** If a plan/spec exists (e.g. an entry under
  `tmp/release-parity-plan/`, an issue, or a design doc), pass it in *as a scope checklist
  only* — "here is what this change set claimed to do; verify the diff actually did it, with
  tests." The plan defines intended scope; it never excuses a principle violation. If the diff
  diverges from the plan, report the divergence; the baseline in
  [`.github/copilot-instructions.md`](../../copilot-instructions.md) wins over any plan.

## Pass 0 — Scope and context

- Get the actual diff: `git diff <base>...HEAD --stat`, then per file. Review only what changed
  plus its blast radius; do not audit the whole repo (that is
  [`review-project.md`](./review-project.md)).
- gokit is a foundation toolkit: a change to a core package's public surface fans out to every
  sub-module, nested adapter, and downstream repo (rskit/pykit parity, consuming services).
  List that blast radius before reviewing.
- Note whether the change belongs in the **root module**, a **sub-module** (own `go.mod`), or a
  **nested adapter** (e.g. `storage/s3`), and whether it belongs in *this* package at all.

## Passes — run in order, stop early on a structural failure

Work the focused files top to bottom. **Stop and reject as soon as a change fails pass `00` or
`01`** — misplaced or duplicated code makes every later pass moot.

1. [`00-structure-placement.md`](./00-structure-placement.md) — module/package placement,
   acyclic layering, `doc.go` and go.work wiring.
2. [`01-canonical-reuse.md`](./01-canonical-reuse.md) — reuse vs. reimplementation of a
   package/stdlib-owned concern. *(blocker class)*
3. [`02-principles.md`](./02-principles.md) — typed/minimal APIs, errors & resilience,
   concurrency, composition, currency, AI features.
4. [`03-security-privacy.md`](./03-security-privacy.md) — trust-boundary validation, injection
   safety, token hygiene, crypto, data minimization.
5. [`04-quality.md`](./04-quality.md) — root-cause over patches, dead code, file/package
   organization, style gates.
6. [`05-tests-tdd.md`](./05-tests-tdd.md) — TDD, `-race -shuffle` determinism, clock/env/cwd
   discipline, fixtures.
7. [`06-docs-supply-chain.md`](./06-docs-supply-chain.md) — `doc.go`/godoc, Conventional
   Commits, `go.sum`, `govulncheck`, SHA-pinned actions, SBOM.
8. [`07-comments-godoc.md`](./07-comments-godoc.md) — comments and godoc explain the code as it
   is; rewrite or delete plan/history/process prose.

Each focused file carries a "Changes mode" scope note — follow that mode here. When you only
need one lens (e.g. just security, just TDD), run that focused file directly instead of this
orchestrator.

## Findings

Record every finding as:

```
severity (blocker / should-fix / nit) — file:line — what's wrong — which principle — suggested fix
```

See [`README.md`](./README.md) for severity definitions.

## Validation

**Scope every command to the changed module(s) — do not run the full-tree gates here.** gokit
has many modules; `make check` / `make test` / `make build` across the whole tree are slow and
are reserved for [`review-project.md`](./review-project.md) or final pre-merge sign-off
(typically in CI). For a change set, run only:

```bash
make fmt                             # gofmt -s clean
make lint M=<module>                 # golangci-lint, scoped
make test M=<module> T=<pattern>     # narrow further with a test pattern, -race -count=1
make test-affected                   # only modules the diff touches
make check-<domain>                  # per-domain gate if the change spans a domain
```

Use `./gomod.sh cmd "<command>" -m <module>` for a command in one module, or `govulncheck ./...`
in the touched module if a dependency changed. Prefer `make test-affected` over the unscoped
targets — it runs only the modules impacted by the current changes. Step up to a per-domain
`make check-<domain>` when the change spans a domain. Run the full `make check` only when the
change is genuinely tree-wide, or leave it to CI for sign-off. A green scoped run is necessary
but **not sufficient** — it will not catch goroutine leaks, missing timeouts/cancellation,
unbounded channels, global-registry composition smells, duplicated owners, or boundary-
validation gaps. Those are on the reviewer.
