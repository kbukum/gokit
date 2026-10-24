# gokit

Multi-module Go infrastructure kit, pre-1.0 alpha. Core packages share the root `go.mod`; heavy adapters have their own modules. Rules live in the [engineering standard](engineering.md); this file is the short version and the map.

## Invariants

- Pre-1.0: fix root causes and update callers; no compatibility shims. For each defect choose Redesign, Align, Enhance, or Drop. Leave sound code alone.
- Put code where [Architecture](../docs/ARCHITECTURE.md) says. Imports point down. Reuse or enhance the [concern owner](../docs/concern-owners.md); never fork it. No runtime import of another kit.
- Public APIs are small and typed. `context.Context` first. Typed `AppError` with causes. No runtime panics, ignored errors, or success-shaped fallbacks.
- Inject registries, loggers, telemetry, policies, and clients. No `init()` I/O or mutable globals.
- Validate trust boundaries. No secrets in source, logs, or URLs. Parameterized SQL and argv-only subprocesses.
- Bound remote calls, retries, queues, and concurrency. Every goroutine has an owner, cancellation, and shutdown.
- Test first, deterministic, race- and shuffle-safe. Unit tests do not prove real driver, TLS, process, or browser behavior.
- Docs are task-first and concise. No hard-wrapped prose in Markdown or Go comments. Comments describe current behavior, not history.
- Never claim a rule is enforced unless a gate checks it. See [Enforcement](engineering.md#enforcement) and [Known gaps](engineering.md#known-gaps).

## Work and validation

- Read only the matching [skill](skills/README.md) and the engineering sections it needs.
- Preserve user edits and the Git index. Commit, amend, push, or open a draft PR only when authorized.
- Keep plans in `tmp/<plan>/`, reusing existing folders. Apply a selected step fully and record progress in the plan.
- Scope gates to the change while working: `make test M=<module> T=<pattern>`, `make lint M=<module>`, `make test-affected`, or `make check-<domain>`. Run `make check` for acceptance. The [validate](skills/validate/SKILL.md) skill has the rest.
- Docs-only edits need link and anchor checks, not Go builds.
- Versions come from `go.mod`, the Makefile, and CI. Do not copy them into docs.

## Where to read

| Change | Read |
|---|---|
| Package or module placement, imports, shared helpers | [Placement and reuse](engineering.md#placement-and-reuse), [Architecture](../docs/ARCHITECTURE.md) |
| Exported API, signatures, versioning | [Public API and stability](engineering.md#public-api-and-stability) |
| Errors, retries, remote calls | [Errors and resilience](engineering.md#errors-and-resilience) |
| Goroutines, components, registries | [Concurrency and lifecycle](engineering.md#concurrency-and-lifecycle), [Composition](engineering.md#composition) |
| Input, auth, crypto, paths, SQL | [Security and privacy](engineering.md#security-and-privacy) |
| AI, model, or tool features | [AI and model features](engineering.md#ai-and-model-features) |
| File layout, naming, `doc.go` | [Code style and structure](engineering.md#code-style-and-structure) |
| Tests, test environments, auth sessions | [Testing and proof](engineering.md#testing-and-proof); [real adapter proof](engineering.md#real-adapter-proof) is required |
| Docs, godoc, changelog | [Documentation and consumer experience](engineering.md#documentation-and-consumer-experience) |
| Dependencies, CI, release | [Dependencies and supply chain](engineering.md#dependencies-and-supply-chain), then the `release` skill |
| New module or backend | `new-module` or `new-backend` skill |
| Finishing any change | [Definition of Done](engineering.md#definition-of-done) |
