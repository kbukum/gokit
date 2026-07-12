# gokit

[![CI](https://github.com/kbukum/gokit/actions/workflows/ci.yml/badge.svg)](https://github.com/kbukum/gokit/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/kbukum/gokit.svg)](https://pkg.go.dev/github.com/kbukum/gokit)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go 1.26+](https://img.shields.io/badge/Go-1.26+-00ADD8.svg)](go.mod)

**A modular Go toolkit for building production services.** Config, logging, resilience, observability, dependency injection, and infrastructure adapters — so teams can focus on business logic instead of reinventing plumbing.

> **Status — pre-1.0.** Public surface is semver-stable per module; breaking changes are documented in [`CHANGELOG.md`](CHANGELOG.md). See [`docs/policy/SEMVER.md`](docs/policy/SEMVER.md).

> **Sibling projects.** gokit (Go, this repo) · [**rskit**](https://github.com/kbukum/rskit) (Rust) · [**pykit**](https://github.com/kbukum/pykit) (Python). Public abstractions (`AppError`, `Component`, `Provider`, `Pipeline`, lifecycle hooks) are evaluated for parity across all three.

## Browse by Domain

Modules are organized into domains for scoped development. See [Module Index](docs/MODULE-INDEX.md) for the full breakdown.

| Domain | Focus | Quick check |
|--------|-------|-------------|
| core | Foundation types, config, logging | `make check-core` |
| patterns | Component, provider, DI, hooks | `make check-patterns` |
| crosscutting | Observability, resilience, security | `make check-crosscutting` |
| composition | Bootstrap, pipeline, DAG, workers | `make check-composition` |
| transport | Server, HTTP, gRPC, SSE | `make check-transport` |
| auth | Authentication, authorization | `make check-auth` |
| data | Database, cache, storage, messaging | `make check-data` |
| ai | LLM, inference, agents, tools | `make check-ai` |
| media | Media processing, transcription | `make check-media` |
| infra | Workload, CLI, benchmarks, testing | `make check-infra` |

CI still runs full-workspace validation; on pull requests the `changes` job also publishes an `affected` domain list from `./scripts/affected-domains.sh` so later workflow steps can consume the same domain mapping developers use locally with `make check-<domain>`.

## Highlights

- **Multi-module layout** — light core (`github.com/kbukum/gokit`) + sub-modules (`gokit/server`, `gokit/database`, …) you opt into individually. No transitive heavy deps unless you ask.
- **Lifecycle-managed components** — uniform `Component` interface (start / stop / health) and `bootstrap.App` orchestrator with graceful shutdown.
- **Production resilience** — circuit breakers, retries with backoff + jitter, bulkheads, rate limiting, OpenTelemetry tracing & metrics.
- **Provider pattern** — typed `RequestResponse[I,O]`, `Stream`, `Sink`, and `Duplex` traits with composable middleware and sink combinators.
- **Per-module versioning** — `gokit/server` and `gokit/database` upgrade independently of core. See [`docs/VERSIONING.md`](docs/VERSIONING.md).
- **Sibling parity** — APIs mirror [rskit](https://github.com/kbukum/rskit) (Rust) and [pykit](https://github.com/kbukum/pykit) (Python).

## Install

```bash
# Core (lightweight, zero heavy deps)
go get github.com/kbukum/gokit@latest

# Add sub-modules à la carte
go get github.com/kbukum/gokit/server@latest
go get github.com/kbukum/gokit/database@latest
```

Requires **Go 1.26+**.

## Quickstart

```go
package main

import (
    "context"
    "github.com/kbukum/gokit/bootstrap"
    "github.com/kbukum/gokit/logging"
)

func main() {
    log := logging.New(&logging.Config{Level: "info", Format: "console"}, "my-service")

    app := bootstrap.NewApp("my-service", "1.0.0", bootstrap.WithLogger(log))
    app.OnConfigure(func(ctx context.Context, app *bootstrap.App) error {
        // wire routes, handlers, business logic — components are already started
        return nil
    })

    // Init → Start → Configure → Ready → wait for signal → Stop
    if err := app.Run(context.Background()); err != nil {
        log.Fatal("app failed", map[string]interface{}{"error": err})
    }
}
```

More examples → [`docs/EXAMPLES.md`](docs/EXAMPLES.md). Full package list → [`docs/PACKAGES.md`](docs/PACKAGES.md).

## Documentation

| Topic | Link |
|---|---|
| All packages & sub-modules | [`docs/PACKAGES.md`](docs/PACKAGES.md) |
| Usage examples | [`docs/EXAMPLES.md`](docs/EXAMPLES.md) |
| Architecture decisions | [`docs/adr/`](docs/adr/) |
| Versioning & releases | [`docs/VERSIONING.md`](docs/VERSIONING.md) · [`docs/RELEASING.md`](docs/RELEASING.md) |
| Semver & deprecation policy | [`docs/policy/SEMVER.md`](docs/policy/SEMVER.md) · [`docs/policy/DEPRECATION.md`](docs/policy/DEPRECATION.md) |
| Cross-module integration | [`INTEGRATION.md`](INTEGRATION.md) |
| Per-package API docs | [pkg.go.dev](https://pkg.go.dev/github.com/kbukum/gokit) |

## Development

```bash
make check    # build + vet + test (all modules)
make test     # tests with -race across all modules
make lint     # golangci-lint
make tidy     # go mod tidy for core + sub-modules
```

## Contributing

We welcome contributions. See [`CONTRIBUTING.md`](CONTRIBUTING.md) for setup, coding standards, and the PR process. By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

Other community docs: [`SECURITY.md`](SECURITY.md) · [`GOVERNANCE.md`](GOVERNANCE.md) · [`MAINTAINERS.md`](MAINTAINERS.md)

## License

[MIT](LICENSE) — Copyright (c) 2024 kbukum
