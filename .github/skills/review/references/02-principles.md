# Pass 02 — Principles

Each item here is a hard principle from [`.github/copilot-instructions.md`](../../../copilot-instructions.md), not a preference. This is where vibe coding drifts most — especially around resilience, concurrency, and composition.

> **Run in a separate, clean-context agent** — never inline in the session that wrote the code.
> An independent reviewer re-derives every judgment from the code
> and the principles instead of trusting prior reasoning.
> A plan/spec may be passed in as a scope checklist only; it never excuses a baseline violation.

**Scope note.** *Changes mode:* grep the touched packages and reason about each runtime path. *Project mode:* the panic/concurrency/composition invariants below hold across the whole library surface — sweep the tree.

## Typed, minimal APIs

Generics-first:
no `interface{}`/`any` on public surfaces except genuinely opaque values (JSON body, `ctx.Value`, DB column scan, third-party interface contract) — and those are documented. Actionable typed errors (`AppError`) that preserve cause via `%w`. Minimal public surface — no incidental exported identifiers; unexport what callers don't need.

## Errors & resilience

- No `panic()` / `log.Fatal` / ignored errors (`_ = f()`) / unchecked type assertions (`x.(T)` without the `, ok` form) on runtime paths (tests excepted). `MustXxx` allowed only for compile-time-safe construction (`regexp.MustCompile`, `template.Must`) and explicit Must* twins of error-returning funcs.
- No success-shaped fallbacks that mask failure (returning a zero value + `nil` error on a real failure).
- Every remote call has a **timeout** via `context.Context`. Retries are **bounded, jittered, and applied to idempotent ops only**. Failures circuit-break and degrade gracefully rather than hang or cascade. (Reuse `resilience` — see pass `01`.)

## Concurrency

- Every goroutine has clear **ownership, cancellation (context), timeout, and shutdown** handling; no goroutine leaks (a `go func()` with no way to stop it is a **blocker**).
- Channels / buffers / concurrency are **bounded with documented backpressure**; components **drain in-flight work on shutdown**. An unbounded queue on an ingest path is a **blocker**.
- No data races: shared state guarded by `sync.Mutex`/`sync.RWMutex` or confined to one goroutine;
  verified under `-race`. Context is the first parameter, never stored in a struct.

## Composition

- Registries and policies are **explicitly injected**; selection is config-driven.
- **No `init()` side effects, no mutable package-global registries**, no reaching for a global logger/tracer — inject them. A package-level `var registry = …` mutated at runtime, or an `init()` that dials network / reads env / registers into a global, is a **blocker**.
- Constructors take `...Option`; dependencies passed in, not resolved from a global.

## Up-to-date

Current Go idioms, not folklore (also enforced in pass `01`). `log/slog` not `logrus`/`zap`; `errors.Is/As/Join`; `slices`/`maps`/`cmp`; `any` over `interface{}`; loop-var capture is fixed in 1.22+ (no `v := v` workarounds). Flag outdated patterns.

## AI / model features (only if the change touches them)

Model output and retrieved context are **untrusted**; outputs are structured/validated;
tool calls are least-privilege with a **human gate on destructive actions**;
prompts/models are versioned and changes gated on evals.

## Detection starters

Exclude `_test.go` when judging runtime-path hits.

```bash
grep -rn --include=*.go 'panic(\|log.Fatal' . | grep -v _test.go
grep -rn --include=*.go 'interface{}\|[^.]\bany\b' . | grep -v _test.go   # public-surface any/interface{}
grep -rn --include=*.go '_ = \|_, _ =' . | grep -v _test.go               # ignored errors
grep -rn --include=*.go 'go func\|go [a-zA-Z]' . | grep -v _test.go       # each needs ownership/cancel/shutdown
grep -rn --include=*.go 'make(chan ' . | grep -v _test.go                 # bounded? backpressure?
grep -rn --include=*.go '^var .*[Rr]egistry\|^func init()' . | grep -v _test.go  # global-registry / init smell
```
