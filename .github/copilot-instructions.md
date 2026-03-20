# Copilot Instructions — gokit

## Overview

gokit (`github.com/kbukum/gokit`) is a multi-module Go library providing foundational
infrastructure for all services in the SkillSense ecosystem. It is the **golden framework** —
all infrastructure lives here, never in consuming projects.

## Architecture

- **Multi-module monorepo**: Root `go.mod` for core packages, sub-modules with own `go.mod` for heavy deps
- **Core** (root module): config, logger, errors, validation, encryption, component, di, resilience, observability, provider, pipeline, dag, media, security, bootstrap, sse, util, version, bench
- **Sub-modules**: auth, authz, database, redis, httpclient, kafka, storage, server, grpc, connect, discovery, process, workload, llm, stateful, testutil, bench/viz, bench/storage

## Key Principles

1. **Generics-first**: All public APIs use Go generics (`[T any]`, `[L comparable]`). No `interface{}` in public APIs.
2. **Interface-heavy, minimal methods**: Interfaces have 1-3 methods. Components opt-in to capabilities via separate interfaces.
3. **Functional options**: All constructors accept `...Option` for extensibility.
4. **Module boundary = dependency boundary**: If a package introduces a new external dependency, it's a separate module with its own `go.mod`.
5. **Middleware composition**: Cross-cutting concerns (logging, metrics, tracing) via `Middleware[I, O]` chains.
6. **Provider pattern**: Four interaction types — `RequestResponse[I,O]`, `Stream[I,O]`, `Sink[I]`, `Duplex[I,O]` — with Registry/Manager/Selector for runtime switching.
7. **Pipeline pattern**: Lazy pull-based `Iterator[T]` with composable operators (Map, Filter, Parallel, Batch, etc.).
8. **Component lifecycle**: `Start/Stop/Health` with deterministic ordering via Registry.

## Cross-Language Portability

gokit is the **reference implementation** for a polyglot kit ecosystem:
- **gokit** (Go) — reference implementation, design happens here first
- **pykit** (Python) — mirrors gokit structure with Python idioms (protocols, async/await, Pydantic)
- **ruskit** (Rust) — mirrors gokit structure with Rust idioms (traits, serde, tokio)

When designing new modules:
- Use the **same names** across kits (adapted for language casing: `BenchRunner` / `BenchRunner` / `BenchRunner`)
- Use the **same module structure** (`{kit}/bench/`, `{kit}/bench/metric/`, `{kit}/bench/report/`)
- Shared artifacts (JSON schemas, dataset formats, Vega-Lite specs) must be **identical** across languages
- All three kits must produce **identical output** for the same input data

## Technology

- Go 1.25+ (generics, iterators)
- zerolog (logging), viper (config), validator/v10 (validation)
- OpenTelemetry (tracing + metrics)
- No CGO in core — pure Go

## Code Style

- `gofmt` + `golangci-lint`
- Package names: lowercase, single-word, no plurals
- Every package has a `doc.go` with `# Section` headings, usage examples, and key type overview
- Exported interfaces + factory functions; concrete implementations often unexported
- Errors: RFC 7807 `AppError` with typed error codes
- Config: embed `config.ServiceConfig`, use `ApplyDefaults()` + `Validate()`
- Tests: parallel, table-driven, use `testutil` helpers

## Module Creation Checklist

When adding a new module:
1. Core package (no heavy deps) → add under root, no new `go.mod`
2. Heavy deps → create sub-module with own `go.mod`, `replace` directive to `../` for local dev
3. Always create `doc.go` with package documentation
4. Add to root `README.md` module map
5. Add `CHANGELOG.md` entry
6. Consider pykit/ruskit portability — design for cross-language consistency
7. Follow existing patterns: look at `provider/`, `pipeline/`, `bench/` for reference

## Directory Structure

```
gokit/
├── Core packages (root go.mod): config, logger, errors, validation, encryption,
│   component, di, resilience, observability, provider, pipeline, dag, media,
│   security, bootstrap, sse, util, version, bench
├── Sub-modules (own go.mod): auth, authz, database, redis, httpclient, kafka,
│   storage, server, grpc, connect, discovery, process, workload, llm, stateful,
│   testutil, bench/viz, bench/storage
└── docs/ — Design documents and implementation plans
```
