# gokit review prompts

A set of standing, re-runnable review prompts for this repository. They encode gokit's
permanent engineering baseline (see [`.github/copilot-instructions.md`](../../copilot-instructions.md)
and the sibling rskit baseline) so any change set — or the whole toolkit — can be reviewed the
same way every time.

gokit is shared foundation infrastructure: a defect in a core package propagates to every
sub-module, every nested adapter, and every downstream consumer (rskit/pykit parity repos and
the services that import gokit). The bar here is correspondingly high — security, concurrency,
and composition each get their own lens. Each prompt works as either a human checklist or the
instruction block you hand an AI reviewer.

## What is here

Three orchestrators that run the full review:

- [`review-changes.md`](./review-changes.md) — review a diff (a branch, commit range, or
  `HEAD~1`) by sequencing the focused passes. Use after every change set, especially fast/
  "vibe-coded" work.
- [`review-project.md`](./review-project.md) — audit the whole tree, independent of any diff.
  Use periodically, before a release, or when onboarding to a module.
- [`review-details.md`](./review-details.md) — an alternative driver that fans the review out
  into **parallel subagent passes by Go concern** (mechanical, correctness, concurrency,
  composition, security, API, tests) and then plans and applies fixes. Use when one driver
  should take a change from review through to merged fixes.

Eight focused passes, each runnable on its own when you only need one lens:

- [`00-structure-placement.md`](./00-structure-placement.md) — module placement (root vs
  sub-module vs nested adapter), acyclic layering (`depguard`), `doc.go` and go.work wiring.
- [`01-canonical-reuse.md`](./01-canonical-reuse.md) — did the code reimplement a concern an
  existing package (or the stdlib) already owns?
- [`02-principles.md`](./02-principles.md) — typed/minimal APIs, errors & resilience,
  concurrency, composition, currency, AI/model features.
- [`03-security-privacy.md`](./03-security-privacy.md) — trust-boundary validation, injection
  safety, token hygiene, crypto, data minimization.
- [`04-quality.md`](./04-quality.md) — root-cause over patches, dead code, file/package
  organization, maintainability, style gates.
- [`05-tests-tdd.md`](./05-tests-tdd.md) — TDD, determinism under `-race -shuffle`, injected
  clocks, env/cwd test discipline, fixtures.
- [`06-docs-supply-chain.md`](./06-docs-supply-chain.md) — `doc.go`/godoc, Conventional
  Commits, `go.sum`, `govulncheck`, SHA-pinned actions, SBOM/provenance.
- [`07-comments-godoc.md`](./07-comments-godoc.md) — comments and godoc explain the code as it
  is, not plans/history/process; rewrite or delete the rest. Runnable any time over the tree.

The orchestrators sequence these passes and add scope handling; the focused files hold the
actual checks. Read the focused file you need and run it directly when a full review is
overkill.

## Run reviews in a separate, clean-context agent

Always dispatch a review to a **fresh reviewer agent with no shared session context** — never
inline in the session that produced the code. A reviewer that "remembers" writing the change
rationalizes it; an independent agent re-derives every judgment from the code and the
principles. Hand the agent only the scope (diff or module/area), the relevant prompt, and this
`reviews/` folder.

A plan, spec, issue, or roadmap (e.g. an entry under `tmp/release-parity-plan/`) may be passed
in *as a scope checklist only* — it defines intended scope ("verify the change did what it
claimed, with tests") but never excuses a baseline violation. If the code diverges from the
plan, report the divergence; the baseline in
[`.github/copilot-instructions.md`](../../copilot-instructions.md) wins over any plan.

## How to run any prompt

1. **Pick scope.** Changes review: set a base ref and get the diff (`git diff <base>...HEAD
   --stat`, then per file). Project review: pick the module(s)/domain or the whole tree.
2. **Work passes in order** (00 → 07). Stop and reject as soon as a change fails pass `00` or
   `01` — misplaced or duplicated code makes every later pass moot.
3. **Run the validation commands** (below). Treat green `make check` as necessary but not
   sufficient: it does not catch goroutine leaks, missing timeouts/cancellation, unbounded
   channels, global-registry composition smells, duplicated owners, or boundary-validation
   gaps. Those are on the reviewer.

## Severity and finding format

Record every finding as:

```
severity (blocker / should-fix / nit) — file:line — what's wrong — which principle — suggested fix
```

- **blocker** — violates a hard principle (upward/cyclic import, concern reimplemented,
  `panic()`/`log.Fatal` on a runtime path, goroutine with no cancellation / unbounded channel,
  package-global mutable registry / `init()` side effect, trust boundary not validated,
  `interface{}`/`any` on a public surface, behavioral change with no test). Must be fixed
  before merge.
- **should-fix** — real defect or debt that is not a baseline violation (compat shim,
  `time.Sleep` in a test, unguarded env/cwd test, inline config instead of a fixture,
  reinvented stdlib facility, one large file that should be split by concern).
- **nit** — minor/style, take-it-or-leave-it.

## Validation commands

**For a change set, scope every command to the changed module(s)** — the full-tree gates are
slow across all modules and belong to a project audit or CI sign-off, not a per-change review:

```bash
make fmt                             # gofmt -s (or verify clean)
make lint M=<module>                 # golangci-lint, scoped to the module
make test M=<module> T=<pattern>     # scoped tests, -race -count=1
make test-affected                   # only modules the diff touches
make check-<domain>                  # per-domain gate: core|patterns|crosscutting|composition|
                                     #   transport|auth|data|ai|media|infra
```

Cross-module operations use `./gomod.sh` (e.g. `./gomod.sh cmd "go test ./..." -m messaging`).

**For a project audit or final sign-off**, run the full gates:

```bash
make build && make vet && make test  # full validation
make check                           # canonical gate (build + vet + test)
make test-coverage                   # coverage report / gate
govulncheck ./...                    # vulnerability scan (per module via ./gomod.sh)
```

Treat green `make check` as necessary but **not sufficient** — it does not catch goroutine
leaks, missing timeouts/cancellation, unbounded channels, global-registry composition smells,
duplicated owners, or boundary-validation gaps. Those are on the reviewer.
