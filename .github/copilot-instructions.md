# gokit

Multi-module Go library providing foundational infrastructure for service development. A
sibling kit to rskit (Rust) and pykit (Python): same module structure and naming, same
engineering baseline, idiomatic per language. rskit is the current reference for shape and
quality; gokit is kept at parity with it.

## Engineering principles

Shared engineering baseline — apply to all work here:

- **Phases:** discover → decide (Redesign / Align / Enhance / Drop / Leave) → implement completely → validate. Prefer root-cause redesign over symptom patches; no compatibility shims in pre-stable code.
- **Layering & reuse:** explicit, acyclic dependency direction — lower layers never import higher (enforced by `depguard`). Reuse or enhance the canonical owner before writing new code; never duplicate shared concerns (errors, config, logging, auth, retries, observability, HTTP, registries). Consult [`docs/concern-owners.md`](../docs/concern-owners.md) for the canonical owner of each shared concern (formats → `codec`, helpers → `util`, paths → `fs`, …) before writing new code.
- **APIs:** typed and minimal; generics-first, no `interface{}`/`any` in public surfaces (except genuinely opaque values, documented); actionable typed errors that preserve cause.
- **Errors & resilience:** no `panic()` / `log.Fatal` / ignored errors (`_ =`) / unchecked type assertions on runtime paths; no success-shaped fallbacks; timeout every remote call via `context.Context`; bounded jittered retries for idempotent ops only; circuit-break and degrade gracefully.
- **Concurrency:** every goroutine has ownership, cancellation (context), timeout, and shutdown; bound channels / buffers / concurrency with documented backpressure; drain on shutdown; no goroutine leaks.
- **Security & privacy:** validate at every trust boundary; least-privilege and secure-by-default; parameterized queries and argv-only subprocess (via `process`); tokens in headers, not query strings; current crypto only; minimize, redact, and retention-bound sensitive data.
- **Composition:** explicit injected registries and config-driven selection; no `init()` side effects, no mutable package-global registries; inject logger / tracer / policies rather than reaching for globals.
- **Tests:** behavioral and deterministic; green under `-race -shuffle=on -count=1`; cover failure paths; injected clocks (never `time.Sleep`); fixtures over embedded config; regression-test every fix.
- **AI / model features:** treat model output and retrieved context as untrusted; enforce structured outputs; least-privilege tool calls with a human gate on destructive actions; version prompts / models and gate changes on evals.
- **Supply chain:** pin CI actions by SHA; scan dependencies (`govulncheck` + licenses); sign release artifacts; attach SBOM and provenance.
- **Currency:** use current Go idioms and standards, not folklore — `log/slog`, `errors.Is/As/Join`, `slices`/`maps`/`cmp`, `any` over `interface{}`; verify the dependency is maintained, the stdlib doesn't already cover it, and no open CVE applies.

Standing, re-runnable development skills encoding this baseline live in
[`.github/skills/`](skills/README.md) — the `review` skill runs the review passes in a
fresh, clean-context agent after every change set and before releases; `create-branch`,
`create-plan`, `apply-plan`, `apply-step`, `create-pr`, `validate`, `new-module`, `new-backend`,
`parity`, and `release` cover the rest of the workflow. Validation is driven through `toven` (see
`toven.toml`).

## Build, Test, and Lint

```bash
make check              # Build + vet + test (full validation)
make build              # Build (M=<module> for specific module)
make test               # Test with -race -count=1 (M=<module>, T=<pattern>)
make test-coverage      # Test with coverage report
make lint               # golangci-lint (M=<module>)
make fmt                # gofmt -s -w
make tidy               # go mod tidy across all modules
```

Cross-module operations use `./gomod.sh`:
```bash
./gomod.sh tidy         # Tidy all modules
./gomod.sh cmd "go test -race -count=1"   # Run command in all modules
./gomod.sh cmd "go test" -m messaging      # Run in specific module
```

Requires: Go 1.25+, golangci-lint (`go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`).

## Module Structure

Multi-module monorepo. Core packages share the root `go.mod`. Packages with heavy external dependencies have their own `go.mod` as sub-modules.

- **Root module** (`github.com/kbukum/gokit`): config, logger, errors, validation, encryption, component, di, resilience, observability, provider, pipeline, dag, security, bootstrap, sse, util, version, bench
- **Sub-modules** (own `go.mod`): auth, authz, database, cache, httpclient, messaging, storage, server, grpc, connect, discovery, workload, llm, media, stateful, testutil

When adding a new module:
1. No heavy deps → add under root module, no new `go.mod`
2. Heavy deps → create sub-module with own `go.mod`, `replace` directive to `../` for local dev
3. Always create `doc.go` with package documentation

## Code Style

- `gofmt -s` + `golangci-lint` (`.golangci.yml` at root; `depguard` enforces layer direction).
- Generics-first: all public APIs use Go generics. No `interface{}`/`any` in public APIs
  (except genuinely opaque values — JSON body, `ctx.Value`, DB scan — documented).
- Interfaces have 1–3 methods. Components opt-in to capabilities via separate interfaces.
- Constructors accept `...Option` for extensibility (functional options pattern).
- Package names: lowercase, single-word, no plurals.
- Every package has a `doc.go`.
- Exported interfaces + factory functions; concrete implementations unexported.
- Errors: RFC 9457 `AppError` with typed error codes.
- Tests: parallel, table-driven, use `testutil` helpers; deterministic under `-race -shuffle`.
- **Readability & structure (load-bearing, not cosmetic):** organize by focused, well-named
  files within a package — never pile unrelated logic into one large file. Split by concern
  (types, options, registry, middleware, adapter) into separate files so the next reader can
  navigate by filename. A file that has grown to cover several responsibilities is a refactor
  signal, not a normal state.
- **Declare-only aggregator.** `doc.go` holds package documentation only; a package's parent
  file never accumulates code — split by concern into named sibling files (as in
  `cli/{theme,render,…}` and `dataset/{payload,record,stage,…}`). Enforced by
  `scripts/check-structure.sh` (`make structure`).

## Validation scope

Scope commands to what changed; the full workspace gates are for audits/CI sign-off:

```bash
make lint M=<module>                 # golangci-lint, one module
make test M=<module> T=<pattern>     # scoped tests (-race -count=1)
make test-affected                   # only modules the diff touches
make check-<domain>                  # per-domain gate: core|patterns|crosscutting|composition|
                                     #   transport|auth|data|ai|media|infra
make check                           # full canonical gate (build + vet + test) — audit/CI
```

## Key Patterns

- **Provider pattern**: `RequestResponse[I,O]`, `Stream[I,O]`, `Sink[I]`, `Duplex[I,O]` with Registry/Manager/Selector.
- **Pipeline pattern**: Lazy pull-based `Iterator[T]` with composable operators.
- **Component lifecycle**: `Start/Stop/Health` with deterministic ordering via Registry.
- **Middleware composition**: `Middleware[I, O]` chains for cross-cutting concerns.
