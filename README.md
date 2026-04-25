# gokit

[![CI](https://github.com/kbukum/gokit/actions/workflows/ci.yml/badge.svg)](https://github.com/kbukum/gokit/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/kbukum/gokit.svg)](https://pkg.go.dev/github.com/kbukum/gokit)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**A modular Go toolkit for building production services.**

gokit provides a shared foundation across Go services — config, logging, resilience, observability, dependency injection, and infrastructure adapters — so teams can focus on business logic instead of reinventing plumbing.

## Architecture

gokit uses a **multi-module** layout:

- **Core module** (`github.com/kbukum/gokit`) — lightweight, zero heavy dependencies. Covers config, logging, errors, DI, resilience, and abstractions.
- **Sub-modules** (`github.com/kbukum/gokit/{name}`) — each has its own `go.mod` and brings in heavier dependencies (Gin, GORM, Kafka, gRPC, etc.) only when you need them.

Import the core for foundational utilities. Add sub-modules à la carte for infrastructure.

## Compatibility Matrix

| Go Version | Module Version |
|------------|----------------|
| 1.25+      | v0.1.2+        |

## Module Map

### Core Packages

| Package | Import | Description |
|---|---|---|
| `errors` | `gokit/errors` | Structured errors with codes, HTTP status mapping, and RFC 7807 support |
| `config` | `gokit/config` | Base configuration with Environment type (Development/Staging/Production) and defaults |
| `logger` | `gokit/logger` | Structured logging via zerolog with context injection and component tagging |
| `util` | `gokit/util` | Generic slice, map, pointer, and functional utilities |
| `version` | `gokit/version` | Build version info — git commit, branch, build time, dirty state |
| `encryption` | `gokit/encryption` | AES-256-GCM encryption and decryption for sensitive data |
| `validation` | `gokit/validation` | Struct tag and programmatic validation with field-level error collection |
| `di` | `gokit/di` | Dependency injection container with lazy/eager init, retry, and circuit breaker |
| `resilience` | `gokit/resilience` | Circuit breaker, retry with backoff, bulkhead isolation, rate limiting |
| `observability` | `gokit/observability` | OpenTelemetry tracing, metrics, and health checking |
| `sse` | `gokit/sse` | Server-sent events broadcasting with per-client channels |
| `provider` | `gokit/provider` | Generic provider framework with metadata, state management, middleware, sink combinators, and runtime checks |
| `pipeline` | `gokit/pipeline` | Pull-based data pipeline with Throttle, Batch, Debounce, and Window operators |
| `dag` | `gokit/dag` | DAG execution engine — dependency-ordered orchestration with batch, streaming, and cascade modes |
| `media` | `gokit/media` | Media type detection from content bytes — video, audio, image, text format identification |
| `security` | `gokit/security` | TLS configuration, certificate utilities, and test helpers |
| `process` | `gokit/process` | Subprocess execution with context cancellation and signal handling |
| `worker` | `gokit/worker` | Push-based task execution with worker pools, real-time event streaming, supervision, and composition |
| `component` | `gokit/component` | Lifecycle interface for infrastructure components (start/stop/health) |
| `bootstrap` | `gokit/bootstrap` | Application startup orchestration and graceful shutdown |

### Sub-Modules

| Module | Import | Description |
|---|---|---|
| `auth` | `gokit/auth` | Authentication — JWT tokens, OIDC verification, password hashing, token validation interfaces |
| `authz` | `gokit/authz` | Authorization — permission checking, wildcard pattern matching (zero external deps) |
| `database` | `gokit/database` | PostgreSQL via GORM — pooling, migrations, health checks, slow query logging |
| `redis` | `gokit/redis` | go-redis client wrapper with pooling, health checks, TypedStore, and JSON operations |
| `httpclient` | `gokit/httpclient` | HTTP client with resilience patterns, retry, and circuit breaking |
| `messaging` | `gokit/messaging` | Message producer/consumer abstraction with Kafka provider and in-memory broker for testing |
| `storage` | `gokit/storage` | Object storage abstraction — local filesystem and S3-compatible backends |
| `server` | `gokit/server` | HTTP server with Gin, HTTP/2, middleware stack, handler mounting |
| `grpc` | `gokit/grpc` | gRPC client config — TLS, keepalive, message size, connection pooling |
| `discovery` | `gokit/discovery` | Service discovery with Consul and static provider support |
| `connect` | `gokit/connect` | Connect-Go RPC registration over HTTP/1.1 with standardized errors |
| `workload` | `gokit/workload` | Workload execution on Docker and Kubernetes backends |
| `testutil` | `gokit/testutil` | Testing infrastructure with component lifecycle, setup/teardown, and state management |
| `stateful` | `gokit/stateful` | Push-based stateful accumulation with configurable triggers and pluggable storage |
| `llm` | `gokit/llm` | LLM chat completion abstraction — dialect-based provider mapping, streaming, structured output |
| `bench` | `gokit/bench` | Evaluation benchmarking framework — datasets, evaluators, metrics, reports, comparison |
| `bench/viz` | `gokit/bench/viz` | SVG visualization generation — ROC curves, confusion matrices, calibration plots |
| `bench/storage` | `gokit/bench/storage` | Bench storage adapter — bridges bench.RunStorage with gokit/storage backends |
| `agent` | `gokit/agent` | Agentic conversation loop — LLM orchestration, tool execution, context management |
| `tool` | `gokit/tool` | Type-safe tool definitions with auto-generated schemas, registry, and middleware |
| `schema` | `gokit/schema` | JSON Schema generation from Go types with validation |
| `hook` | `gokit/hook` | Generic event hook system for lifecycle handler registration and execution |
| `mcp` | `gokit/mcp` | Model Context Protocol server and client integration |
| `explain` | `gokit/explain` | Structured explanation generation from analysis signals via LLM |
| `embedding` | `gokit/embedding` | Vector utilities — cosine similarity, distance metrics, pooling |
| `vectorstore` | `gokit/vectorstore` | Vector similarity search store abstraction with in-memory backend |

## Quick Start

### Install the core

```bash
go get github.com/kbukum/gokit@latest
```

### Add a sub-module

```bash
go get github.com/kbukum/gokit/server@latest
go get github.com/kbukum/gokit/database@latest
```

## Usage Examples

### Config + Logger

```go
package main

import (
    "github.com/kbukum/gokit/config"
    "github.com/kbukum/gokit/logger"
)

type ServiceConfig struct {
    config.ServiceConfig `yaml:",inline" mapstructure:",squash"`
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
import "github.com/kbukum/gokit/server"

srvCfg := &server.Config{Host: "0.0.0.0", Port: 8080}
srvCfg.ApplyDefaults()

srv := server.New(srvCfg, log)
srv.ApplyDefaults("my-service", healthChecker)

srv.GinEngine().GET("/api/items", itemsHandler)

srv.Start(ctx)
defer srv.Stop(ctx)
```

### Provider Pattern

```go
import "github.com/kbukum/gokit/provider"

// Define a domain provider using the interaction pattern
type DiarizationProvider = provider.RequestResponse[AudioInput, []Segment]

// Use the manager for runtime selection
reg := provider.NewRegistry[DiarizationProvider]()
mgr := provider.NewManager(reg, &provider.HealthCheckSelector[DiarizationProvider]{})
p, _ := mgr.Get(ctx)
result, err := p.Execute(ctx, audioInput)
```

### Sink Composition

```go
import "github.com/kbukum/gokit/provider"

// Wrap a plain function as a Sink
kafkaSink := provider.NewSinkFunc("kafka", func(ctx context.Context, event Event) error {
    return producer.Publish(ctx, topic, event)
})

// Fan out to multiple sinks in parallel
sink := provider.FanOutSink("multi",
    kafkaSink,
    provider.AdaptSink(analyticsSink, "adapt", toAnalyticsEvent),
    provider.TapSink(loggingSink, func(ctx context.Context, e Event) {
        metrics.RecordEvent(e.Type)
    }),
)

// Compose sink middleware
wrapped := provider.ChainSink(withLogging, withMetrics)(sink)
wrapped.Send(ctx, event) // dispatches to all sinks with logging + metrics
```

### Subprocess Execution

```go
import "github.com/kbukum/gokit/process"

result, err := process.Run(ctx, process.Command{
    Binary: "python", Args: []string{"diarize.py", "audio.wav"},
})
fmt.Println(string(result.Stdout))
```

### Bootstrap Lifecycle

```go
import "github.com/kbukum/gokit/bootstrap"

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

### Agent Loop

```go
import (
    "github.com/kbukum/gokit/agent"
    "github.com/kbukum/gokit/tool"
)

registry := tool.NewRegistry()
registry.Register(weatherTool)

a := agent.New(llmProvider, registry,
    agent.WithContextStrategy(&agent.SlidingWindowStrategy{MaxTokens: 4096}),
)
result, err := a.Run(ctx, "What's the weather in Berlin?")
fmt.Println(result.Events)
```

### LLM Chat Completion

```go
import "github.com/kbukum/gokit/llm"

provider := llm.NewProvider(llm.Config{
    Dialect: "openai",
    Model:   "gpt-4",
    APIKey:  os.Getenv("OPENAI_API_KEY"),
})

resp, err := provider.ChatCompletion(ctx, llm.Request{
    Messages: []llm.Message{
        {Role: "user", Content: "Explain circuit breakers"},
    },
})
fmt.Println(resp.Content)
```

### Tool Definition

```go
import "github.com/kbukum/gokit/tool"

t := tool.New("get_weather", "Get current weather for a city",
    tool.HandlerFunc(func(ctx context.Context, input map[string]any) (any, error) {
        city := input["city"].(string)
        return fetchWeather(ctx, city)
    }),
)

registry := tool.NewRegistry()
registry.Register(t)
```

### Messaging

```go
import "github.com/kbukum/gokit/messaging"

producer, _ := messaging.NewProducer(cfg)
producer.Publish(ctx, "events", messaging.Message{
    Key:   []byte("user-123"),
    Value: payload,
})

consumer, _ := messaging.NewConsumer(cfg, "my-group")
consumer.Subscribe("events", func(ctx context.Context, msg messaging.Message) error {
    return processEvent(msg)
})
consumer.Start(ctx)
```

### Object Storage

```go
import "github.com/kbukum/gokit/storage"

store, _ := storage.New(cfg)
_ = store.Put(ctx, "uploads/report.pdf", reader)
rc, _ := store.Get(ctx, "uploads/report.pdf")
defer rc.Close()
```

## Module Details

Each module has its own documentation. Refer to the package-level Go docs or source:

| Group | Packages | Focus |
|---|---|---|
| **Foundation** | errors, config, logger, version | Configuration, logging, error handling |
| **Utilities** | util, encryption, validation | Common helpers and data validation |
| **Architecture** | di, provider, component, bootstrap | DI, lifecycle management, provider pattern |
| **Auth & Authz** | auth, authz | Authentication (JWT, OIDC, password) and authorization (permissions) |
| **Resilience** | resilience, observability | Fault tolerance, tracing, metrics |
| **Data** | pipeline, dag, sse, media | Pull-based pipelines, DAG orchestration, server-sent events, media detection |
| **Infrastructure** | database, redis, messaging, storage | Data stores and messaging |
| **Networking** | httpclient | HTTP client with resilience |
| **Transport** | server, grpc, connect, discovery | HTTP, gRPC, service discovery |
| **Execution** | process, workload | Subprocess and container workload execution |
| **Testing** | testutil | Component lifecycle testing infrastructure |
| **Stateful** | stateful | Push-based accumulation with triggers and storage |
| **AI** | llm | LLM chat completion, structured output, explanation generation |
| **Evaluation** | bench, bench/viz, bench/storage | Provider benchmarking, metrics, visualizations, result storage |
| **AI / Agent** | agent, tool, hook, mcp, schema, explain | Agentic loops, tool registry, MCP integration, explanations |
| **Vectors** | embedding, vectorstore | Embedding utilities, vector similarity search |

## Multi-Module Versioning

Core and sub-modules version **independently**. Each sub-module has its own `go.mod` and release tags:

```
v0.5.0              ← core module
server/v0.3.2       ← server sub-module
database/v0.4.1     ← database sub-module
```

This means:
- Upgrading `gokit/server` does **not** force an upgrade of `gokit/database`.
- Core can ship breaking changes without touching sub-modules (and vice versa).
- Each module follows [semver](https://semver.org/) on its own timeline.

## Cross-Kit Comparison

gokit, [rskit](https://github.com/kbukum/rskit) (Rust), and [pykit](https://github.com/kbukum/pykit) (Python) share the same module structure and design philosophy. The table below shows capability coverage across all three kits.

| Capability | gokit | rskit | pykit |
|---|---|---|---|
| Errors | ✅ `errors` | ✅ `rskit-errors` | ✅ `pykit-errors` |
| Config | ✅ `config` | ✅ `rskit-config` | ✅ `pykit-config` |
| Logging | ✅ `logger` | ✅ `rskit-logging` | ✅ `pykit-logging` |
| Validation | ✅ `validation` | ✅ `rskit-validation` | ✅ `pykit-validation` |
| Encryption | ✅ `encryption` | ✅ `rskit-encryption` | ✅ `pykit-encryption` |
| Utilities | ✅ `util` | ❌ | ✅ `pykit-util` |
| Version | ✅ `version` | ❌ | ✅ `pykit-version` |
| Media | ✅ `media` | ✅ `rskit-media` | ✅ `pykit-media` |
| Security | ✅ `security` | ❌ | ✅ `pykit-security` |
| DI | ✅ `di` | ✅ `rskit-di` | ✅ `pykit-di` |
| Component | ✅ `component` | ❌ | ✅ `pykit-component` |
| Bootstrap | ✅ `bootstrap` | ✅ `rskit-bootstrap` | ✅ `pykit-bootstrap` |
| Provider | ✅ `provider` | ✅ `rskit-provider` | ✅ `pykit-provider` |
| Resilience | ✅ `resilience` | ✅ `rskit-resilience` | ✅ `pykit-resilience` |
| Observability | ✅ `observability` | ✅ `rskit-observability` | ✅ `pykit-observability` |
| Pipeline | ✅ `pipeline` | ✅ `rskit-pipeline` | ✅ `pykit-pipeline` |
| DAG | ✅ `dag` | ✅ `rskit-dag` | ✅ `pykit-dag` |
| Worker | ✅ `worker` | ✅ `rskit-worker` | ✅ `pykit-worker` |
| SSE | ✅ `sse` | ✅ `rskit-sse` | ✅ `pykit-sse` |
| Stateful | ✅ `stateful` | ❌ | ✅ `pykit-stateful` |
| Auth | ✅ `auth` | ✅ `rskit-auth` | ✅ `pykit-auth` |
| Authz | ✅ `authz` | ✅ `rskit-authz` | ✅ `pykit-authz` |
| Database | ✅ `database` | ✅ `rskit-database` | ✅ `pykit-database` |
| Redis / Cache | ✅ `redis` | ✅ `rskit-cache` | ✅ `pykit-redis` |
| Storage / File | ✅ `storage` | ✅ `rskit-file` | ✅ `pykit-storage` |
| Messaging | ✅ `messaging` | ✅ `rskit-messaging` | ✅ `pykit-messaging` |
| HTTP Client | ✅ `httpclient` | ✅ `rskit-httpclient` | ✅ `pykit-httpclient` |
| Server | ✅ `server` | ✅ `rskit-http`, `rskit-server` | ✅ `pykit-server` |
| gRPC Client | ✅ `grpc` | ✅ `rskit-grpc-client` | ✅ `pykit-grpc` |
| Connect | ✅ `connect` | ❌ | ❌ |
| Discovery | ✅ `discovery` | ✅ `rskit-discovery` | ✅ `pykit-discovery` |
| Process | ✅ `process` | ✅ `rskit-process` | ✅ `pykit-process` |
| Workload | ✅ `workload` | ❌ | ✅ `pykit-workload` |
| Test Utilities | ✅ `testutil` | ✅ `rskit-testutil` | ✅ `pykit-testutil` |
| LLM | ✅ `llm` | ✅ `rskit-llm` | ✅ `pykit-llm` |
| LLM Providers | ❌ | ✅ `rskit-llm-providers` | ✅ `pykit-llm-providers` |
| Agent | ✅ `agent` | ✅ `rskit-agent` | ✅ `pykit-agent` |
| Tool | ✅ `tool` | ✅ `rskit-tool` | ✅ `pykit-tool` |
| MCP | ✅ `mcp` | ✅ `rskit-mcp` | ✅ `pykit-mcp` |
| Hook | ✅ `hook` | ✅ `rskit-hook` | ✅ `pykit-hook` |
| Schema | ✅ `schema` | ✅ `rskit-schema` | ✅ `pykit-schema` |
| Explain | ✅ `explain` | ✅ `rskit-explain` | ✅ `pykit-explain` |
| Bench | ✅ `bench` | ✅ `rskit-bench` | ✅ `pykit-bench` |
| Dataset | ❌ | ✅ `rskit-dataset` | ✅ `pykit-dataset` |
| Embedding | ✅ `embedding` | ✅ `rskit-embedding` | ✅ `pykit-embedding` |
| Vector Store | ✅ `vectorstore` | ✅ `rskit-vector-store` | ✅ `pykit-vector-store` |
| Inference | ❌ | ✅ `rskit-inference` | ✅ `pykit-triton` |
| CLI | ❌ | ✅ `rskit-cli` | ❌ |
| Metrics | ❌ | ❌ | ✅ `pykit-metrics` |

## Development

```bash
make check    # build + vet + test (all modules)
make test     # run tests with -race across all modules
make lint     # golangci-lint across all modules
make fmt      # gofmt -s
make tidy     # go mod tidy for core + all sub-modules
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development workflow, coding standards, and how to submit pull requests.

We follow the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md).

## License

[MIT](LICENSE) — Copyright (c) 2024 kbukum
