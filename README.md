# gokit

[![CI](https://github.com/kbukum/gokit/actions/workflows/ci.yml/badge.svg)](https://github.com/kbukum/gokit/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/kbukum/gokit.svg)](https://pkg.go.dev/github.com/kbukum/gokit)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go 1.27.3+](https://img.shields.io/badge/Go-1.27.3+-00ADD8.svg)](go.mod)

**A modular Go toolkit for building production services.** Config, logging, resilience, observability, dependency injection, and infrastructure adapters — so teams can focus on business logic instead of reinventing plumbing.

> **Status — pre-1.0 alpha.** APIs may change in a minor release; patch releases do not break them. Breaking changes are listed in [`CHANGELOG.md`](CHANGELOG.md). See the [SemVer policy](docs/policy/SEMVER.md).
>
> **Latest release — `v0.2.0`.** Latest prerelease — `v0.3.0-alpha.1`, for evaluation and integration testing; APIs may change before the stable `v0.3.0`.

> **Sibling project.** gokit (Go, this repo) · [**rskit**](https://github.com/kbukum/rskit) (Rust).
> Shared abstractions (`AppError`, `Component`, `Provider`, `Stream`, lifecycle hooks) aim for parity in capability and wire contracts; each kit stays idiomatic to its language.

## Browse by Domain

Modules are organized into domains for scoped development. Domains are `[groups.*]` in [`toven.toml`](toven.toml); Toven checks that each domain only depends on the domains it allows. `toven groups` lists them, and the [Module Index](docs/MODULE-INDEX.md) shows the full breakdown.

| Domain | Focus | Quick check |
|--------|-------|-------------|
| foundation | Root module: core types, config, logging, patterns, observability, resilience, security, bootstrap, stream, DAG, workers, SSE, CLI; `schema`, `testutil` | `make check-foundation` |
| composition | Stateful stores, DAG test helpers | `make check-composition` |
| transport | Server, HTTP, gRPC, connect, discovery | `make check-transport` |
| auth | Authentication, authorization | `make check-auth` |
| data | Database, cache, storage, messaging | `make check-data` |
| ai | LLM, inference, agents, tools | `make check-ai` |
| media | Light detection, metadata, image ops | `make check-media` |
| infra | Workload, dataset, benchmarks, auth test host | `make check-infra` |
| devtools | Git repository operations | `make check-devtools` |

Package layering inside the root module is described in [Architecture](docs/ARCHITECTURE.md). Lint bans only selected edges; the rest is checked in review.

CI always validates the whole workspace with the same `make` targets; see [Development](#development) for the local commands.

## Highlights

- **Multi-module layout** — light core (`github.com/kbukum/gokit`) + sub-modules (`gokit/server`, `gokit/database`, …) you opt into individually. No transitive heavy deps unless you ask.
- **Lifecycle-managed components** — uniform `Component` interface (start / stop / health) and `bootstrap.App` orchestrator with graceful shutdown.
- **Production resilience** — circuit breakers, retries with backoff + jitter, bulkheads, rate limiting, OpenTelemetry tracing & metrics.
- **Provider pattern** — typed `RequestResponse[I,O]`, `Stream`, `Sink`, and `Duplex` traits with composable middleware and sink combinators.
- **Typed authentication** — one principal across HTTP, Connect and SSE, with separate application authorization and credential ceilings. [Opaque browser sessions](auth/README.md) keep credentials in secure host-only cookies and validate database state on every request; automation uses header API keys.
- **Per-module versioning** — every module has its own matching tag, while normal releases are cut in lock-step for convenience. See [`docs/VERSIONING.md`](docs/VERSIONING.md).
- **Sibling parity** — capabilities track [rskit](https://github.com/kbukum/rskit) (Rust), with idiomatic Go APIs.

## Install

```bash
# Core (lightweight, zero heavy deps)
go get github.com/kbukum/gokit@latest

# Add sub-modules à la carte
go get github.com/kbukum/gokit/server@latest
go get github.com/kbukum/gokit/database@latest
```

Requires the Go version in [`go.mod`](go.mod).

## Quickstart

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/kbukum/gokit/bootstrap"
    "github.com/kbukum/gokit/config"
)

type Config struct {
    config.ServiceConfig `mapstructure:",squash"`
}

func main() {
    // Run releases the App's own logger, so report its error another way.
    if err := run(context.Background()); err != nil {
        fmt.Fprintf(os.Stderr, "app failed: %v\n", err)
        os.Exit(1)
    }
}

func run(ctx context.Context) error {
    cfg := &Config{ServiceConfig: config.ServiceConfig{Name: "my-service", Version: "1.0.0"}}

    app, err := bootstrap.NewApp(cfg)
    if err != nil {
        return err
    }

    app.OnConfigure(func(ctx context.Context, app *bootstrap.App[*Config]) error {
        // register components and wire business logic — they start right after configure
        return nil
    })

    // Configure → Start → Ready → wait for signal → Shutdown.
    return app.Run(ctx)
}
```

More examples → [`docs/EXAMPLES.md`](docs/EXAMPLES.md). Full package list → [`docs/PACKAGES.md`](docs/PACKAGES.md). Runnable demo programs → [`examples/`](examples/).

## Documentation

| Topic | Link |
|---|---|
| All packages & sub-modules | [`docs/PACKAGES.md`](docs/PACKAGES.md) |
| Usage examples | [`docs/EXAMPLES.md`](docs/EXAMPLES.md) |
| Where code goes | [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) |
| Engineering standard | [`.github/engineering.md`](.github/engineering.md) |
| Architecture decisions | [`docs/adr/`](docs/adr/) |
| Versioning & releases | [`docs/VERSIONING.md`](docs/VERSIONING.md) · [`docs/RELEASING.md`](docs/RELEASING.md) |
| Semver & deprecation policy | [`docs/policy/SEMVER.md`](docs/policy/SEMVER.md) · [`docs/policy/DEPRECATION.md`](docs/policy/DEPRECATION.md) |
| Cross-module integration | [`INTEGRATION.md`](INTEGRATION.md) |
| Runnable demo programs | [`examples/`](examples/) |
| Per-package API docs | [pkg.go.dev](https://pkg.go.dev/github.com/kbukum/gokit) |

## Development

```bash
make check    # build + vet + test (all modules)
make test     # tests with -race -shuffle=on across all modules
make lint     # golangci-lint
make tidy     # go mod tidy for core + sub-modules
make examples # build + vet the runnable demos in examples/
```

CI runs these same targets; see [`CONTRIBUTING.md`](CONTRIBUTING.md#ci) for the full list, including coverage, integration and fuzz. `make help` lists every target.

## Contributing

We welcome contributions. See [`CONTRIBUTING.md`](CONTRIBUTING.md) for setup, coding standards, and the PR process. By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

Other community docs: [`SECURITY.md`](SECURITY.md) · [`GOVERNANCE.md`](GOVERNANCE.md) · [`MAINTAINERS.md`](MAINTAINERS.md)

## License

[MIT](LICENSE) — Copyright (c) 2024 kbukum
