# gokit

[![CI](https://github.com/skillsenselab/gokit/actions/workflows/ci.yml/badge.svg)](https://github.com/skillsenselab/gokit/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.25-blue.svg)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**A modular Go toolkit for building production services.**

gokit provides a shared foundation across Go services — config, logging, resilience, observability, dependency injection, and infrastructure adapters — so teams can focus on business logic instead of reinventing plumbing.

## Architecture

gokit uses a **multi-module** layout:

- **Core module** (`github.com/skillsenselab/gokit`) — lightweight, zero heavy dependencies. Covers config, logging, errors, DI, resilience, and abstractions.
- **Sub-modules** (`github.com/skillsenselab/gokit/{name}`) — each has its own `go.mod` and brings in heavier dependencies (Gin, GORM, Kafka, gRPC, etc.) only when you need them.

Import the core for foundational utilities. Add sub-modules à la carte for infrastructure.

## Module Map

### Core Packages

| Package | Import | Description |
|---|---|---|
| `errors` | `gokit/errors` | Structured errors with codes, HTTP status mapping, and RFC 7807 support |
| `config` | `gokit/config` | Base configuration with environment-specific settings and defaults |
| `logger` | `gokit/logger` | Structured logging via zerolog with context injection and component tagging |
| `util` | `gokit/util` | Generic slice, map, pointer, and functional utilities |
| `version` | `gokit/version` | Build version info — git commit, branch, build time, dirty state |
| `encryption` | `gokit/encryption` | AES-256-GCM encryption and decryption for sensitive data |
| `validation` | `gokit/validation` | Struct tag and programmatic validation with field-level error collection |
| `di` | `gokit/di` | Dependency injection container with lazy/eager init, retry, and circuit breaker |
| `resilience` | `gokit/resilience` | Circuit breaker, retry with backoff, bulkhead isolation, rate limiting |
| `observability` | `gokit/observability` | OpenTelemetry tracing, metrics, and health checking |
| `sse` | `gokit/sse` | Server-sent events broadcasting with per-client channels |
| `provider` | `gokit/provider` | Generic provider framework for swappable backends with runtime checks |
| `component` | `gokit/component` | Lifecycle interface for infrastructure components (start/stop/health) |
| `bootstrap` | `gokit/bootstrap` | Application startup orchestration and graceful shutdown |

### Sub-Modules

| Module | Import | Description |
|---|---|---|
| `database` | `gokit/database` | PostgreSQL via GORM — pooling, migrations, health checks, slow query logging |
| `redis` | `gokit/redis` | go-redis client wrapper with pooling, health checks, and integrated logging |
| `kafka` | `gokit/kafka` | Kafka producer/consumer with TLS/SASL, configurable transport |
| `storage` | `gokit/storage` | Object storage abstraction — local filesystem and S3-compatible backends |
| `server` | `gokit/server` | HTTP server with Gin, HTTP/2, middleware stack, handler mounting |
| `grpc` | `gokit/grpc` | gRPC client config — TLS, keepalive, message size, connection pooling |
| `discovery` | `gokit/discovery` | Service discovery with Consul and static provider support |
| `connect` | `gokit/connect` | Connect-Go RPC registration over HTTP/1.1 with standardized errors |
| `llm` | `gokit/llm` | LLM provider interface — OpenAI, Ollama; completions, structured output, streaming |
| `transcription` | `gokit/transcription` | Speech-to-text provider interface (Whisper) |
| `diarization` | `gokit/diarization` | Speaker identification provider interface (Pyannote) |

## Quick Start

### Install the core

```bash
go get github.com/skillsenselab/gokit@latest
```

### Add a sub-module

```bash
go get github.com/skillsenselab/gokit/server@latest
go get github.com/skillsenselab/gokit/database@latest
```

## Usage Examples

### Config + Logger

```go
package main

import (
    "github.com/skillsenselab/gokit/config"
    "github.com/skillsenselab/gokit/logger"
)

type ServiceConfig struct {
    config.BaseConfig
    Port int `yaml:"port"`
}

func main() {
    cfg := &ServiceConfig{}
    if err := config.LoadConfig("my-service", cfg,
        config.WithConfigFile("./config.yml"),
        config.WithEnvFile(".env"),
    ); err != nil {
        panic(err)
    }
    cfg.ApplyDefaults()

    log := logger.New(&logger.Config{
        Level:  "info",
        Format: "console",
    }, cfg.Name)

    log.Info("service configured", map[string]interface{}{
        "env": cfg.Environment,
    })
}
```

### HTTP Server with Middleware

```go
import "github.com/skillsenselab/gokit/server"

srvCfg := &server.Config{Host: "0.0.0.0", Port: 8080}
srvCfg.ApplyDefaults()

srv := server.New(srvCfg, log)
srv.ApplyDefaults("my-service", healthChecker)

srv.GinEngine().GET("/api/items", itemsHandler)

srv.Start(ctx)
defer srv.Stop(ctx)
```

### LLM Provider

```go
import "github.com/skillsenselab/gokit/llm"

resp, err := llmProvider.Complete(ctx, llm.CompletionRequest{
    Model:        "gpt-4",
    SystemPrompt: "You are a helpful assistant.",
    Messages:     []llm.Message{{Role: "user", Content: "Explain Go interfaces."}},
    Temperature:  0.7,
})
fmt.Println(resp.Content)

// Streaming
chunks, _ := llmProvider.Stream(ctx, req)
for chunk := range chunks {
    fmt.Print(chunk.Content)
}
```

### Bootstrap Lifecycle

```go
import "github.com/skillsenselab/gokit/bootstrap"

app := bootstrap.NewApp("my-service", "1.0.0",
    bootstrap.WithLogger(log),
    bootstrap.WithGracefulTimeout(15 * time.Second),
)

app.RegisterComponent(db)    // component.Component
app.RegisterComponent(cache) // component.Component

app.OnConfigure(func(ctx context.Context, app *bootstrap.App) error {
    // All components started — set up routes, handlers, business logic
    return nil
})

// Run: Init → Start → Configure → Ready → wait for signal → Stop
if err := app.Run(ctx); err != nil {
    log.Fatal("app failed", map[string]interface{}{"error": err})
}
```

## Module Details

Each module has its own documentation. Refer to the package-level Go docs or source:

| Group | Packages | Focus |
|---|---|---|
| **Foundation** | errors, config, logger, version | Configuration, logging, error handling |
| **Utilities** | util, encryption, validation | Common helpers and data validation |
| **Architecture** | di, provider, component, bootstrap | DI, lifecycle management, provider pattern |
| **Resilience** | resilience, observability | Fault tolerance, tracing, metrics |
| **Networking** | sse | Server-sent events |
| **Infrastructure** | database, redis, kafka, storage | Data stores and messaging |
| **Transport** | server, grpc, connect, discovery | HTTP, gRPC, service discovery |
| **AI/ML** | llm, transcription, diarization | Language models, speech processing |

## Multi-Module Versioning

Core and sub-modules version **independently**. Each sub-module has its own `go.mod` and release tags:

```
v0.5.0              ← core module
server/v0.3.2       ← server sub-module
database/v0.4.1     ← database sub-module
llm/v0.2.0          ← llm sub-module
```

This means:
- Upgrading `gokit/server` does **not** force an upgrade of `gokit/database`.
- Core can ship breaking changes without touching sub-modules (and vice versa).
- Each module follows [semver](https://semver.org/) on its own timeline.

## Development

```bash
make check    # build + vet + test (all modules)
make test     # run tests with -race across all modules
make lint     # golangci-lint across all modules
make fmt      # gofmt -s
make tidy     # go mod tidy for core + all sub-modules
```

## License

[MIT](LICENSE) — Copyright (c) 2024 SkillSense Lab
