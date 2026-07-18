---
name: review
description: >-
    Run gokit's standing engineering-baseline review over a change set (a branch, commit range,
    or HEAD~1) or over a whole module/domain/tree. Sequences eight focused passes — structure &
    placement, canonical reuse, principles, security & privacy, quality, tests/TDD, docs & supply
    chain, comments & godoc. Use before merging a change, when auditing a module, or before a
    release. Always run it in a fresh, clean-context reviewer.
user-invocable: true
---

# Reviewing gokit against its engineering baseline

gokit is shared foundation infrastructure:
a defect in a core package propagates to every sub-module, every adapter,
and every downstream consumer (rskit/pykit parity repos and services that import gokit).
The bar is correspondingly high.
This skill encodes gokit's permanent review baseline as eight focused passes plus three orchestrators.

The authoritative baseline lives in [`.github/copilot-instructions.md`](../../copilot-instructions.md).
A plan, issue, or roadmap may be passed in **as a scope checklist only** —
it defines intended scope, never excuses a baseline violation. If the code diverges from the plan,
report the divergence; the baseline wins.

## Run in a separate, clean-context agent

**Always dispatch a review to a fresh reviewer with no shared session context** —
never inline in the session that wrote the code.
A reviewer that "remembers" writing the change rationalizes it;
an independent agent re-derives every judgment from the code and the principles.
Hand it only the scope (diff or module/domain) and this skill.

## Pick a driver

- **Change set** → [`references/review-changes.md`](references/review-changes.md).
  A diff (branch, commit range, or `HEAD~1`). Use after every change set,
  especially fast/"vibe-coded" work.
- **Whole tree / module** → [`references/review-project.md`](references/review-project.md).
  A standing audit independent of any diff. Use periodically, before a release, or when onboarding.
- **Review → fix in one pass** → [`references/review-details.md`](references/review-details.md).
  Fans the review into parallel subagent passes by Go concern, then plans and applies fixes.

## The eight focused passes (run in order)

Stop and reject as soon as a change fails pass `00` or `01` — misplaced
or duplicated code makes every later pass moot.
Each file also carries a "Project mode" note for tree-wide sweeps
and can be run standalone when you need only one lens.

1. [`references/00-structure-placement.md`](references/00-structure-placement.md) —
   module placement (root vs sub-module vs nested adapter), acyclic layering (`depguard`), `doc.go`
   and go.work wiring.
2. [`references/01-canonical-reuse.md`](references/01-canonical-reuse.md) —
   did the code reimplement a concern an existing package (or the stdlib) already owns?
   *(blocker class)*
3. [`references/02-principles.md`](references/02-principles.md) — typed/minimal APIs,
   errors & resilience, concurrency, composition, up-to-date idioms, AI/model features.
4. [`references/03-security-privacy.md`](references/03-security-privacy.md) —
   trust-boundary validation, injection safety, token handling, crypto, data minimization.
5. [`references/04-quality.md`](references/04-quality.md) — root-cause over patches, dead code,
   file/package organization, maintainability, style gates.
6. [`references/05-tests-tdd.md`](references/05-tests-tdd.md) — TDD,
   determinism under `-race -shuffle`, injected clocks, env/cwd discipline, fixtures.
7. [`references/06-docs-supply-chain.md`](references/06-docs-supply-chain.md) — `doc.go`/godoc,
   Conventional Commits, `go.sum`, `govulncheck`, SHA-pinned actions, SBOM/provenance.
8. [`references/07-comments-godoc.md`](references/07-comments-godoc.md) —
   comments/godoc describe the code as it is, not plans/history/process.

## Severity and finding format

```
severity (blocker / should-fix / nit) — file:line — what's wrong — which principle — suggested fix
```

- **blocker** —
  hard-principle violation (upward/cyclic import, concern reimplemented, `panic()`/`log.Fatal` on a runtime path, goroutine with no cancellation / unbounded channel, package-global mutable registry / `init()` side effect, trust boundary not validated, `interface{}`/`any` on a public surface, behavioral change with no test).
  Fix before merge.
- **should-fix** — real defect
  or debt that isn't a baseline violation (compat shim, `time.Sleep` in a test, unguarded env/cwd test, inline config instead of a fixture, reinvented stdlib facility, one large file that should be split by concern).
- **nit** — minor/style, take-it-or-leave-it.

## Validation is via toven (see the `validate` skill)

The reference files were written against `make`/`gomod.sh`;
read those commands as their toven equivalents (`make lint M=x` → `toven lint --module go:x`, `make test-affected` → `toven affected test --base origin/main --merge-base`, etc.).
Scope every command to the changed module(s); the full-tree gates belong to a project audit
or CI sign-off.

Treat a green run as **necessary but not sufficient**: it does not catch goroutine leaks,
missing timeouts/cancellation, unbounded channels, global-registry composition smells,
duplicated owners, or boundary-validation gaps. Those are on the reviewer.
