# gokit

Multi-module Go library providing foundational infrastructure for service development. This is the reference implementation — pykit (Python) and rskit (Rust) mirror its structure.

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

- **Root module** (`github.com/kbukum/gokit`): config, logger, errors, validation, encryption, component, di, resilience, observability, provider, pipeline, dag, media, security, bootstrap, sse, util, version, bench
- **Sub-modules** (own `go.mod`): auth, authz, database, redis, httpclient, messaging, storage, server, grpc, connect, discovery, workload, llm, stateful, testutil

When adding a new module:
1. No heavy deps → add under root module, no new `go.mod`
2. Heavy deps → create sub-module with own `go.mod`, `replace` directive to `../` for local dev
3. Always create `doc.go` with package documentation

## Code Style

- `gofmt` + `golangci-lint` (`.golangci.yml` at root)
- Generics-first: all public APIs use Go generics. No `interface{}` in public APIs.
- Interfaces have 1–3 methods. Components opt-in to capabilities via separate interfaces.
- Constructors accept `...Option` for extensibility (functional options pattern).
- Package names: lowercase, single-word, no plurals.
- Every package has a `doc.go`.
- Exported interfaces + factory functions; concrete implementations unexported.
- Errors: RFC 7807 `AppError` with typed error codes.
- Tests: parallel, table-driven, use `testutil` helpers.

## Key Patterns

- **Provider pattern**: `RequestResponse[I,O]`, `Stream[I,O]`, `Sink[I]`, `Duplex[I,O]` with Registry/Manager/Selector.
- **Pipeline pattern**: Lazy pull-based `Iterator[T]` with composable operators.
- **Component lifecycle**: `Start/Stop/Health` with deterministic ordering via Registry.
- **Middleware composition**: `Middleware[I, O]` chains for cross-cutting concerns.
