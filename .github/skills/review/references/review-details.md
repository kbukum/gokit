# Go Review — Plan, Clarify, Apply

An alternative orchestrator to [`review-changes.md`](./review-changes.md) / [`review-project.md`](./review-project.md): instead of sequencing the 00–07 lenses, it fans the review out into **parallel subagent passes by Go concern**, then plans and applies fixes. Use it when you want one driver to take a change from review through to merged fixes.

Run each pass as a **separate subagent with clean context**. The orchestrator (this file) sequences them and collects findings. Do not concatenate passes into one prompt.

Mode is either **changes** (a diff: branch, commit range, `HEAD~1`) or **project** (whole tree, no diff). State the mode up front.

> The focused 00–07 files hold the canonical, gokit-specific checks (placement, canonical-owner reuse, security/privacy, supply chain, comments). This file is the *driver*; when a pass below needs the full rule for a lens, defer to the matching focused file rather than duplicating it.

---

## Phase 1 — Scope

1. `git status`, `git diff --stat`, `git diff` (changes mode) or `ls` the module tree + dependency map (project mode). Preserve uncommitted changes; integrate on top, never discard.
2. List the surface to review: changed modules/packages (changes mode) or chosen modules/workspace (project mode). Note cross-cutting touches: a root-module package's public surface fans out to every sub-module, every nested adapter, and downstream consumers (rskit/pykit parity). Also flag `go.work` / `core.go.work` / `contrib.go.work` edits, shared error types (`AppError`), public re-exports, `.golangci.yml`/`depguard` config.
3. Determine which passes apply via the triggers below. Skip non-applicable passes explicitly in the final report.

The reviewer judges code as written, against the rules below and the baseline in [`.github/copilot-instructions.md`](../../../copilot-instructions.md). PR descriptions, commit messages, or plan/ADR docs are scope hints only — never justifications.

## Phase 2 — Passes

Run **A first** (cheap, gates the rest). Then **B–F in parallel** where independent. Then **G last** (cross-references everything).

Each subagent receives: its scope, the pass spec below, and nothing else. Scope `go`/`make` to the touched module(s) with `./gomod.sh cmd "<cmd>" -m <module>` or `make check-<domain>`; the unscoped workspace gates are slow across every module and belong to sign-off/CI.

### Pass A — Mechanical (always runs)

Tool output only, no judgment. Use gokit's real gates:

```bash
make fmt                                          # gofmt -s, whole tree (fast)
make lint                                         # golangci-lint incl. vet + depguard (layering)
make check-<domain>                               # fmt+vet+lint+test for the touched domain
govulncheck ./...                                 # from the touched module, if deps/public in scope
```

Report pass/fail per command with the first failure block verbatim.

### Pass B — Correctness

**Scope:** all in-scope `.go` files.

Check: `panic()` / `log.Fatal` / ignored errors (`_ = f()`) / unchecked type assertions (`x.(T)` without `, ok`) on fallible runtime paths (tests excepted); no success-shaped fallback that masks failure (zero value + `nil` error on real failure); error context preserved through `%w`/`errors.Is`/`As`/`Join` as gokit `AppError` with its typed code and cause intact; `MustXxx` only for compile-time-safe construction; resource cleanup on every return path including errors (`defer` for `Close`/`Unlock`); `context.Context` is the first parameter and propagated, never stored in a struct. *(Canonical owner: pass [`01`](./01-canonical-reuse.md).)*

Skip if: scope is docs-only or config-only.

### Pass C — Concurrency

**Scope:** files with `go func`/`go <call>`, `sync`, `chan`, `context`, `errgroup`, or `atomic`.

Check: every spawned goroutine has clear ownership, cancellation (context), timeout, and shutdown — a `go func()` with no stop path is a **blocker**; no `sync.Mutex`/`RWMutex` held across a blocking call or channel op; no data races (shared state guarded or goroutine-confined, verified under `-race`); structured concurrency via `errgroup`/`worker` over loose `go`; channels/queues/buffers are **bounded with documented backpressure** and components **drain in-flight work on shutdown** (an unbounded channel or a goroutine with no cancellation path is a **blocker**); context cancellation actually observed (`select` on `ctx.Done()`). Time-dependent paths are testable via an **injected clock**, not wall-clock `time.Sleep`.

Skip if: no goroutine/channel/sync surface in scope.

### Pass D — Composition and lifecycle

**Scope:** registries, `Component` impls, `di`/`bootstrap` wiring, provider/adapter construction, anything wiring dependencies together.

Check: registries and policies are **explicitly injected**, selection is config-driven; **no `init()` side effects, no mutable package-global registry**, no reaching for a global logger/tracer — inject them (a package-level `var registry`, or an `init()` that dials network / reads env / registers into a global, is a **blocker**); `Component` lifecycle (`Start`/`Stop`/`Health`) honored with registry ordering and drain-on-stop; adapters register via explicit `Register(registry)` and sit behind their own sub-module/package, not wired unconditionally into the core default. *(Placement: pass [`00`](./00-structure-placement.md); composition principle: pass [`02`](./02-principles.md).)*

Skip if: no composition/lifecycle/registry surface in scope.

### Pass E — Security, config, and boundaries

**Scope:** external-facing surfaces (HTTP, process, storage/database/cache adapters, auth, crypto), config loaders, env-var handling, path handling, and docs describing config or env.

Check: untrusted input validated at every trust boundary before flowing into a query, path, command, or deserialization (an unvalidated path is a **blocker**); parameterized queries only — never `fmt.Sprintf`-built SQL; argv-only subprocess via `process`, no `sh -c` interpolation of untrusted input; tokens/credentials in headers not query strings, never logged, redacted in errors; auth header-only, reject query-string tokens, JWT alg allow-list + reject `alg: none` + require `exp`/`iss`/`aud`; current crypto only (no MD5/SHA-1-for-security/ECB/static-IV/ hard-coded key) routed through `encryption`/`security`; unbounded reads of untrusted input get explicit limits (`io.LimitReader`); path-shaped values use `filepath.Join` (never hardcoded separators) and `t.TempDir`/temp helpers over `/tmp/...`. *(Full rule: pass [`03`](./03-security-privacy.md).)*

Skip if: no security-sensitive, config, env, or path code in scope.

### Pass F — API surface and dependencies

**Scope:** package public surfaces, `doc.go`, `go.mod`, anything changing exported items.

Check: new exported items intentional (unexport what callers don't need — no stutter, `cache.New` not `cache.NewCache`); no `interface{}`/`any` escape hatch on a public surface except documented genuinely-opaque values; generics over `interface{}` for typed containers; `...Option` constructor shape; new deps justified (maintained, no open CVE, not duplicating an owning package or the stdlib — up-to-date check, pass [`01`](./01-canonical-reuse.md)); `go.mod`/`go.sum` tidy per affected module (`go mod tidy`); MSRV/`go` directive correct; a new module wired into `go.work` / `core.go.work` / `contrib.go.work`, `domains.toml`, and the matching `make check-<domain>`, with a `doc.go` package overview.

Skip if: no public items, deps, or `go.mod` in scope.

### Pass G — Tests, docs, semantics (runs last)

**Scope:** the in-scope code plus findings from A–F.

Check: behavioral code in scope has tests covering it (changes mode: in the same diff; project mode: anywhere in the tree); bug fixes have a regression test that fails without the fix; failure paths asserted, not just happy paths; tests **green under `-race -shuffle=on`** and depend on no wall clock, network, or working directory unless intentional (time uses an **injected clock**; env-var tests use `t.Setenv`; filesystem tests use `t.TempDir`); coverage meets the per-package floors (≥80% per package, ≥85% overall, ≥85% for `errors`/`auth`/`authz`/`security`/`resilience`/`encryption`, checked via `make test-coverage`; CI codecov enforces project 80% / patch 85%); parsers/validators/auth/codecs/schema carry `Fuzz` targets; fixtures over large inline config; an operation does what its name implies; every exported identifier has godoc that **matches implemented behavior**, each package has a `doc.go`, examples compile; comments describe the code as it is, not plans/history. *(Full rules: passes [`05`](./05-tests-tdd.md) and [`06`](./06-docs-supply-chain.md); comment quality: pass [`07`](./07-comments-godoc.md).)*

Always runs.

## Phase 3 — Consolidate

Orchestrator collects findings into one table:

```
pass | severity (blocker/should-fix/nit) | file:line | finding | suggested fix
```

Severity rule: **blocker** = principle violation, behavior is wrong, or a contract is broken (see [`SKILL.md`](../SKILL.md) for the full definition). Otherwise should-fix or nit.

Group by file in the final report. State explicitly any pass that was **skipped** (with the trigger that failed) and any pass that was **deferred** (with reason).

## Phase 4 — Plan and clarify

Group findings by pass, order by severity. For each group write a one-line fix plan: what changes, where, how it's verified. Flag ambiguities (behavior change vs strict fix, breaking API vs deprecation, doc-only vs behavior-aligning) with a proposed default and the alternative. **Pause for user confirmation before editing.**

## Phase 5 — Apply

After confirmation:

1. Apply fixes in plan order, one pass per commit where reasonable (Conventional Commits: `feat`/`fix`/`docs`/`refactor`/`test`/`chore`).
2. Re-run the matching pass's validation after each fix, scoped to the touched module(s). Stop and report if anything fails.
3. Final step: re-run Pass A across the in-scope modules.

## Reviewer notes

- Code judges itself. External narrative (PR description, commit message, plan/ADR doc) is scope only, not justification.
- Detection commands (`rg`/`grep`, `go`, `make`) are loaded by the subagent when it searches, not held in the resident prompt.
- If scope is trivial (docs-only, single-line fix), run only A and G; skip the rest with explicit reason.
