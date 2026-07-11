# Pass 01 — Canonical-owner reuse

gokit *is* the canonical toolkit, so the duplication risk is internal: **did the change
reimplement something an existing package (or the standard library) already owns?** Vibe-coded
code reaches for a fresh local helper instead of the owner — assume duplication until proven
otherwise. Treat findings here as a blocker class.

> **Run in a separate, clean-context agent** — never inline in the session that wrote the code.
> An independent reviewer re-derives every judgment from the code and the principles instead of
> trusting prior reasoning. A plan/spec may be passed in as a scope checklist only; it never
> excuses a baseline violation.

**Scope note.** *Changes mode:* for each new type/helper in the diff, name the concern and find
its owner. *Project mode:* sweep the tree for the patterns below and reconcile each against the
owning package — long-lived internal forks are exactly what this pass exists to surface.

## The rule

Reuse or enhance the canonical owner before writing new code. Never duplicate a shared concern
— **errors, config, logging, auth, retries/resilience, observability, HTTP, registries,
validation, process, di**. If the owner is inadequate, enhance it *generically* rather than
forking a copy in another package. gokit must stay foundational and multi-purpose: a fix
belongs in the owner so every consumer benefits.

## How to check, not just glance

For each candidate, name the concern, locate its owning package, and confirm the change calls
the owner rather than rewriting it:

- **Errors.** gokit `AppError` (RFC 7807) with typed error codes and wrapped cause via `%w`.
  A fresh `errors.New`/`fmt.Errorf` sentinel for a shared concern, or a bespoke error struct
  duplicating `AppError`, is duplication. Use `errors.Is/As/Join`.
- **Resilience.** Retries / timeouts / circuit-breaking come from `resilience`, not hand-rolled
  loops or ad-hoc `context.WithTimeout` scattering with bespoke backoff.
- **Config / logging / di / observability.** Route through the owning package; no parallel
  re-implementation, no second logger/tracer setup. Logging is `log/slog`-based via `logger`,
  not a fresh `log` or `fmt.Print`.
- **HTTP / transport.** Reuse `httpclient` / `server`; a raw `http.Client{}` with bespoke
  retry/timeout in an adapter is duplication.
- **Subprocess.** Execution goes through `process` (argv-only), not a bare `exec.Command`.
- **Currency (part of reuse).** Before adding a dependency or helper, verify the stdlib does
  not already cover it (`slices`, `maps`, `cmp`, `errors.Join`, `sync.OnceValue`), the
  dependency is maintained, and no open CVE applies. Reinventing a std facility is a should-fix.
- **"Almost the same" counts.** A near-copy with one tweaked line is still a fork — enhance the
  owner to cover the new case.

## Detection starters

These flag candidates, not verdicts — read each hit, then name the owner that should have been
used.

```bash
grep -rn --include=*.go 'errors.New(\|fmt.Errorf(' . | grep -v _test.go   # vs AppError for shared concerns
grep -rn --include=*.go 'exec.Command' . | grep -v _test.go               # should route through process
grep -rn --include=*.go 'http.Client{\|&http.Client' . | grep -v _test.go # should reuse httpclient
grep -rn --include=*.go 'time.After\|context.WithTimeout\|backoff\|retry' . | grep -v _test.go  # resilience owner
grep -rn --include=*.go 'log.Print\|fmt.Print\|"log"' . | grep -v _test.go # should use logger (slog)
```

For each hit: is there a package owner for this concern? If yes and the code does not use it →
**blocker** (reuse). If no owner exists and it is a genuinely foundational concern → it should
be **added to the owning package** (or a new one), not solved locally; a local solution is a
**should-fix** with an "upstream to the owner" note.

## Output for this pass

Per finding, name the concrete package/type that should have been used (e.g. "use `resilience`
retry policy instead of a hand-rolled loop", "wrap with `process` rather than `exec.Command`").
