# gokit Package Map

gokit is a multi-module Go workspace.
The **core module** (`github.com/kbukum/gokit`) is light-weight and dependency-thin;
**sub-modules** (`github.com/kbukum/gokit/{name}`) bring in heavier dependencies à la carte.

Every package has its own `README.md` with API examples — start there for details.
This file is the bird's-eye index.

## Compatibility

| Go version | gokit version |
|------------|---------------|
| 1.26+      | v0.1.2+       |

## Core Packages

| Package | Import | Description |
|---|---|---|
| `errors` | `gokit/errors` | Structured errors with codes, HTTP status mapping, RFC 9457 |
| `config` | `gokit/config` | Base configuration with `Environment` type and defaults |
| `logging` | `gokit/logging` | Structured logging via zerolog with context injection |
| `util` | `gokit/util` | Generic slice, map, pointer, functional utilities |
| `codec` | `gokit/codec` | Pluggable structured-text codecs (JSON, TOML, …) over a shared value tree |
| `fs` | `gokit/fs` | Local filesystem primitives — safe paths, atomic writes, temp files, metadata |
| `version` | `gokit/version` | Build version info — git commit, branch, build time |
| `encryption` | `gokit/encryption` | AES-256-GCM encryption for sensitive data |
| `validation` | `gokit/validation` | Struct tag and programmatic validation |
| `di` | `gokit/di` | Type-keyed dependency injection with eager/singleton/transient modes and closeable lifecycle |
| `resilience` | `gokit/resilience` | Circuit breaker, retry, bulkhead, rate limiting |
| `observability` | `gokit/observability` | OpenTelemetry tracing, metrics, health checking |
| `sse` | `gokit/sse` | Server-sent events broadcasting |
| `provider` | `gokit/provider` | Generic provider framework + sink combinators |
| `stream` | `gokit/stream` | Pull-based stream (Throttle, Batch, Debounce, Window) |
| `dag` | `gokit/dag` | DAG execution engine — batch / streaming / cascade |
| `chain` | `gokit/chain` | Sequential chain execution — step piping, progress, cancellation |
| `media` | `gokit/media` | Light standalone kit: type/format detection, metadata, cheap image ops |
| `security` | `gokit/security` | TLS configuration and certificate utilities |
| `process` | `gokit/process` | Subprocess execution with cancellation |
| `worker` | `gokit/worker` | Push-based worker pools with supervision |
| `component` | `gokit/component` | Lifecycle interface (start/stop/health) |
| `bootstrap` | `gokit/bootstrap` | Application startup orchestration & graceful shutdown |

## Sub-Modules

| Module | Import | Description |
|---|---|---|
| `auth` | `gokit/auth` | JWT, OIDC, password hashing, token validation |
| `authz` | `gokit/authz` | Permission checking, wildcard pattern matching |
| `database` | `gokit/database` | Explicit-driver database component — pooling, migrations, health |
| `cache` | `gokit/cache` | Cache abstraction with memory default and opt-in Redis adapter |
| `httpclient` | `gokit/httpclient` | HTTP client with resilience patterns |
| `messaging` | `gokit/messaging` | Transport-agnostic producer/consumer registry with memory default and opt-in Kafka/NATS/RabbitMQ adapters |
| `storage` | `gokit/storage` | Object storage — local + S3-compatible |
| `server` | `gokit/server` | HTTP server (Gin, HTTP/2, middleware stack) |
| `grpc` | `gokit/grpc` | gRPC client config — TLS, keepalive, pooling |
| `discovery` | `gokit/discovery` | Service discovery (Consul + static) |
| `connect` | `gokit/connect` | Connect-Go RPC over HTTP/1.1 |
| `workload` | `gokit/workload` | Docker / Kubernetes workload execution |
| `testutil` | `gokit/testutil` | Component lifecycle & state-management testing |
| `stateful` | `gokit/stateful` | Push-based stateful accumulation |
| `llm` | `gokit/llm` | LLM chat completion abstraction |
| `inference` | `gokit/inference` | Model-serving runtime adapters — Triton, KServe v2, vLLM, TGI |
| `ai` | `gokit/ai` | Universal AI/ML primitives — value types, sentinel errors, semantic keys |
| `bench` | `gokit/bench` | Evaluation framework — datasets, evaluators, reports |
| `bench/viz` | `gokit/bench/viz` | Pure-Go SVG ROC / confusion / calibration / distribution plots |
| `bench/storage` | `gokit/bench/storage` | Bench storage adapter |
| `agent` | `gokit/agent` | Agentic loop — LLM, tools, context management |
| `tool` | `gokit/tool` | Type-safe tool definitions with auto-generated schemas |
| `schema` | `gokit/schema` | JSON Schema generation from Go types |
| `hook` | `gokit/hook` | Generic event hook system |
| `mcp` | `gokit/mcp` | Model Context Protocol server / client |
| `skill` | `gokit/skill` | SDK-free skill manifests, loading, registry, activation |
| `git` | `gokit/git` | Git repository operations — capability interfaces, embedded/CLI backends |
| `embedding` | `gokit/embedding` | Cosine similarity, distance metrics, pooling |
| `vectorstore` | `gokit/vectorstore` | Vector similarity search abstraction |

## Layered Architecture (Foundation → Specialist)

| Group | Packages | Focus |
|---|---|---|
| **Foundation** | errors, config, logging, version, codec, fs | Configuration, logging, errors, encoding, filesystem |
| **Utilities** | util, encryption, validation | Helpers and data validation |
| **Architecture** | di, provider, component, bootstrap | DI, lifecycle, provider pattern |
| **Auth & Authz** | auth, authz | Authentication and authorization |
| **Resilience** | resilience, observability | Fault tolerance, tracing, metrics |
| **Data & Flow** | stream, dag, chain, sse, media, stateful | Streams, DAGs, chains, SSE, media, accumulation |
| **Infrastructure** | database, cache, messaging, storage | Data stores and messaging |
| **Networking** | httpclient | HTTP client with resilience |
| **Transport** | server, grpc, connect, discovery | HTTP, gRPC, service discovery |
| **Execution** | process, workload, worker | Subprocess and container workloads |
| **Testing** | testutil | Component lifecycle testing infrastructure |
| **AI** | ai, llm, inference, agent, tool, hook, mcp, skill, schema | LLM orchestration & tooling |
| **Evaluation** | bench, bench/viz, bench/storage | Provider benchmarking |
| **Vectors** | embedding, vectorstore | Embedding & similarity search |
| **Devtools** | git | Git repository operations |

See [`docs/adr/0001-three-tier-layering.md`](adr/0001-three-tier-layering.md) for the layering rationale.

## Multi-Module Versioning

Core and sub-modules version **independently**. Each sub-module has its own `go.mod`
and release tags:

```
v0.5.0              ← core module
server/v0.3.2       ← server sub-module
database/v0.4.1     ← database sub-module
```

- Upgrading `gokit/server` does not force an upgrade of `gokit/database`.
- Core can ship breaking changes without touching sub-modules (and vice versa).
- Each module follows [semver](https://semver.org/) on its own timeline.

See [`docs/VERSIONING.md`](VERSIONING.md) for the full guide.
