# gokit

Multi-module Go infrastructure kit. Core packages share the root `go.mod`; heavy adapters have their own modules. Versions and tools come from `go.mod`, CI, and the Makefile, not copied version lists.

## Invariants

- Pre-stable: redesign root causes, not compatibility shims. Choose Redesign / Align / Enhance / Drop for defects; leave sound code alone. Keep changes and dependent callers within the requested scope.
- Reuse the [canonical concern owner](../docs/concern-owners.md). Imports point downward; `depguard` enforces boundaries. Implement lower-layer contracts and inject behavior at composition. No kit runtime imports another kit. Internal consistency and idiomatic Go outrank symbol parity.
- Public APIs are typed and minimal; use generics where useful, small interfaces, `...Option` or cohesive request structs. Keep `context.Context` first. Document genuinely opaque `any`. Return cause-preserving `AppError`; no runtime panics, ignored errors, or success-shaped fallbacks.
- Config selects explicitly registered adapters. Inject logger, telemetry, policies, and clients; no mutable global registry or `init()` I/O. Provider shapes: RequestResponse, Stream, Sink, Duplex.
- Validate boundaries; no secrets in source/logs, credential URLs, SQL interpolation, or shell-built subprocesses. Bound calls/retries/buffers; every goroutine owns cancellation and shutdown.
- Use focused concern-named files and declare-only `doc.go`. Follow gofmt, golangci-lint, and current stdlib idioms. Split mixed concerns, not files merely exceeding a line count.
- Test-first, deterministic behavior/failures; reuse `testutil`, inject clocks, preserve race/shuffle safety. Coverage: >=80% per package, >=85% overall and for errors/auth/authz/security/resilience/encryption. Never weaken integration gates.
- Markdown and Go comment prose have no arbitrary column wrapping. Describe current behavior, not implementation history.

## Work and validation

Read only the matching [skill](skills/README.md) and needed reference sections. Preserve user edits/index; commit, amend, push, or open draft PRs only when authorized. Keep plans in `tmp/<plan>/`, reusing existing folders. Apply each selected step fully; record progress in the step, not routine handoffs.

From this repository: `make test M=<module> T=<pattern>`, `make lint M=<module>`, `make test-affected`, or `make check-<domain>` during implementation; `make check` for required full acceptance. See [validate](skills/validate/SKILL.md) for Toven selectors and additional gates. Documentation-only edits need link/metadata checks, not application builds.

## Load before the relevant change

| Change | Reference |
|---|---|
| API/error contracts, signatures, package layout | [Code style](engineering.md#code-style) |
| Runtime test environments or auth/session/SSE | [Validation scope](engineering.md#validation-scope); real adapter/session proof is mandatory |
| New module or provider | [Module structure](engineering.md#module-structure), then `new-module` or `new-backend` |
| AI, security, dependencies, release | [Engineering principles](engineering.md#engineering-principles), then the matching review/release checklist |

Read sections, not the whole reference. A unit test does not certify real driver, TLS, process, or browser behavior.
