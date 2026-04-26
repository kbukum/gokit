# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **bench**: per-package `Benchmark*` coverage for the hot paths flagged by the OSS-review perf gap (#50, F-020). Package count grew from 5 → 15; benchmark count from 5 → 42. New benchmarks live in:
  - `registry` — Register/Get/Lookup/Names/Each
  - `di` — Container Register, Resolve (interface + generic + Must variants), Provide, ResolveKey
  - `validation` — fluent validator chains, struct validator, UUID, pattern
  - `chain` — Executor.Execute (1/4/16/64 ops), Builder.Build
  - `dag` — BuildLevels, ExecuteBatch (chain + fan, 4/16/64 nodes)
  - `tool` — Registry Register/Get/Call
  - `workload` / `storage` / `discovery` / `llm` — factory/dialect Register & Get
  - `auth/oidc` — JWKS getKey hit/miss, RSA publicKey decode, RS256 verifyRSA
- **ci**: `.github/workflows/bench.yml` extended to iterate over every gokit module that has benchmarks (was root-only); benchstat still runs head-vs-base and remains advisory until the baseline stabilises.

### Added
- **registry** (NEW package): generic `Registry[T any]` consolidating the previously ad-hoc registries in `auth`, `discovery`, `storage`, `tool`, `workload`, and `llm`. `Register` returns an error on empty name, nil value, or duplicate name; `Names()` returns sorted; `Each` iterates deterministically. (#45)
- **di**: typed-key DI surface layered on top of `UnifiedContainer`:
  - `Key[T any]`, `NameKey[T](name)` — opaque, type-parameterised keys. The full key embeds `reflect.Type` of `T`, so two `Key[T]` of different concrete types with the same `name` cannot collide.
  - `Provide[T](c, key, ctor)` / `ProvideSingleton[T](c, key, value)` — generic registration; constructor signature is validated up front (must return `T` or `(T, error)`).
  - `ResolveKey[T](c, key)` / `MustResolveKey[T](c, key)` — generic resolution, no type assertions in caller code. (#43)

### Changed (Breaking API Changes)
- **auth**: `Registry.Register` now returns `error` on duplicate registration instead of silently overwriting. `Registry.MustGet` removed — use `Get`.
- **tool**: `Registry.MustRegister` removed — use `Register` (which returns `error`). (#46)
- **workload**: `FactoryRegistry.MustRegister` removed — use `Register`. (#46)
- **llm**: `DialectRegistry.MustRegister` removed — use `Register`. (#46)
- **storage**: `FactoryRegistry.Register` now returns `error` (was panic on duplicate). Provider `Register` functions (`local.Register`, `s3.Register`, `supabase.Register`) likewise return `error`.
- **discovery**: `ProviderRegistry.Register` now returns `error` (was panic). `NewComponent(registry, cfg, log, opts...)` now returns `(*Component, error)` — previously panicked. Provider `Register` functions (`static.Register`, `consul.Register`) return `error`.
- **di**: `UnifiedContainer.MustResolve` method removed. The free function `di.MustResolve[T](container, key)` is **kept** (issue #46 explicitly allows `Must*` for `init()` / test / CLI scope, where this helper is idiomatic).
- **auth/authctx**: `MustGet[T]` removed — use `Get[T]`. (#46)
- **server/middleware**: `MustTenantFromContext` removed — use `TenantFromContext`. (#46)
- **agent**: `MustPromptTemplate` and `PromptBuilder.MustBuild` removed — use `NewPromptTemplate` / `Build`. (#46)

### Internal
- All 6 first-party registries are now thin wrappers around `registry.Registry[T]`. Subsequent registries should use the shared package directly.

## [0.2.0] - 2026-04-25

> Tag `v0.2.0` shipped on 2026-04-04 but never received a CHANGELOG entry.
> This entry back-fills the previous `[Unreleased]` section verbatim. From
> this release on, every tag MUST be accompanied by a corresponding CHANGELOG
> entry — enforced by `tag-modules.sh` (see `docs/RELEASING.md`).
>
> The `kafka/v0.2.0` and `kafka/testutil/v0.2.0` tags are orphans from when
> the kafka provider lived at `/kafka`; the package now lives at
> `/messaging/kafka` and is versioned in lock-step with `messaging`.

### Changed (Breaking API Changes)
- **workload**: `RegisterFactory()` global and `New(cfg, providerCfg, log)` removed. `New` now requires an explicit `*FactoryRegistry` as its first argument: `New(registry, cfg, providerCfg, log)`. Provider packages (`docker`, `kubernetes`) no longer register themselves via `init()`; call their `Register(registry)` function from your composition root. `NewComponent` likewise now takes the registry as its first argument.
- **llm**: `RegisterDialect()`, `GetDialect()`, and `Dialects()` package-level functions removed. `New(cfg)` is replaced by `New(registry, cfg)` taking an explicit `*DialectRegistry`. Provider packages (`anthropic`, `gemini`, `openai`) no longer register via `init()`; call their `Register(registry)` function instead.
- **di**: `MustResolve(name string) interface{}` removed from the `Container` interface. Use the generic free function `di.MustResolve[T](container, key)` instead — it provides type safety and works with any `Container` implementation.
- **config**: `WarningFunc` signature changed from `func(msg string, args ...any)` (printf-style) to `func(msg string, attrs ...slog.Attr)` (structured). Update custom warning loggers to emit structured attributes instead of formatted strings; this aligns config warnings with the rest of gokit's structured logging.
- **bootstrap**: `Summary.DisplaySummary` no longer writes directly to `os.Stdout`. Output now goes to the writer configured via `bootstrap.WithWriter(io.Writer)` (default still `os.Stdout`). `NewSummaryWithOptions` and `(*Summary).SetWriter` allow injecting a custom writer for testing or redirection.
- **storage**: `DefaultFactoryRegistry` global and `RegisterFactory()` / `New(cfg, providerCfg, log)` shims removed. `New` now requires an explicit `*FactoryRegistry` as its first argument: `New(registry, cfg, providerCfg, log)`. Provider packages (`local`, `s3`, `supabase`) no longer register themselves via `init()`; call their `Register(registry)` function from your composition root.
- **discovery**: `DefaultProviderRegistry` global, `RegisterProviderFactory()`, and `GetProviderFactory()` shims removed. `NewComponent` now requires an explicit `*ProviderRegistry` as its first argument: `NewComponent(registry, cfg, log, opts...)`. Provider packages (`static`, `consul`) no longer register via `init()`; call their `Register(registry)` function instead. `WithProviderRegistry` option removed.
- **server/middleware**: `Auth()` and `OptionalAuth()` now return `(gin.HandlerFunc, error)` instead of panicking on misconfiguration. All call sites must handle the returned error.
- **server/middleware**: `OptionalAuth` rejects invalid tokens by default (secure-by-default). Use `WithAllowInvalidTokens(true)` to opt in to the previous lax behavior. `WithRejectInvalidTokens` option removed.
- **di**: `ResolveOrError` removed (was an alias of `Resolve`). Use `Resolve` directly.
- **server/middleware**: `TenantFromContextOrError` and `ErrNoTenantID` removed. Use `TenantFromContext` (returns `(string, bool)`) or `MustTenantFromContext`.
- **config**: `Warning` struct and `[]Warning` return value removed from `loadFromResolvedFiles`. Non-fatal warnings are surfaced exclusively through the `WarningFunc` callback.

### Added
- **bootstrap**: `WithWriter(io.Writer)` option and `(*Summary).SetWriter` method for redirecting summary output (testing, in-memory capture, file logging).
- **workload**: `FactoryRegistry` type with `Register`, `MustRegister`, `Get`, and `Names`. Mirrors the `storage` package's explicit-registry pattern.
- **llm**: `DialectRegistry` type with `Register`, `MustRegister`, `Get`, and `Names`.
- **CI/governance**: `.editorconfig`, `.gitattributes`, `.github/dependabot.yml`, committed `go.work`, `GOVERNANCE.md`, `MAINTAINERS.md`. Expanded `SECURITY.md` with a private vulnerability reporting flow and a supply-chain section.
- **CI**: pinned `golangci-lint` to a specific tag, added `govulncheck` per module, multi-OS test matrix on representative modules, `-shuffle=on` and race detection by default, fuzz smoke job, and Go-version-consistency check across all `go.mod` files.
- **lint**: `errorlint`, `nilerr`, `copyloopvar`, `wastedassign`, `sqlclosecheck`, `rowserrcheck`, and govet `shadow` are now enforced.
- **examples**: `Example*` tests added for `config`, `errors`, `logger`, `pipeline`, `provider`, and `di` for godoc discoverability.
- **docs**: Added `doc.go` to packages that previously lacked package-level documentation (`database/repository`, `discovery/{consul,static}`, `grpc/{client,interceptor}`, `messaging/kafka/{consumer,producer}`, `server/{endpoint,middleware}`, `storage/{local,s3,supabase}`, `workload/{docker,kubernetes}`).
- **benchmarks**: All benchmarks now call `b.ReportAllocs()` for allocation visibility.

### Security
- **gosec**: Removed the global `G402` exclude. TLS configuration sites that intentionally allow `InsecureSkipVerify` now carry a per-site `//nolint:gosec` directive with a justifying comment.

### Migration

- **workload**:
  ```go
  // Before
  import _ "github.com/kbukum/gokit/workload/docker" // side-effect init()
  mgr, err := workload.New(cfg, dockerCfg, log)

  // After
  import "github.com/kbukum/gokit/workload/docker"

  reg := workload.NewFactoryRegistry()
  if err := docker.Register(reg); err != nil { return err }
  mgr, err := workload.New(reg, cfg, dockerCfg, log)
  ```

- **llm**:
  ```go
  // Before
  import _ "github.com/kbukum/gokit/llm/providers/openai"
  adapter, err := llm.New(cfg)

  // After
  import "github.com/kbukum/gokit/llm/providers/openai"

  reg := llm.NewDialectRegistry()
  if err := openai.Register(reg); err != nil { return err }
  adapter, err := llm.New(reg, cfg)
  ```

- **di.MustResolve**:
  ```go
  // Before
  svc := container.MustResolve("svc").(*MyService)

  // After
  svc := di.MustResolve[*MyService](container, "svc")
  ```

- **config.WarningFunc**:
  ```go
  // Before
  warn := func(msg string, args ...any) { log.Printf(msg, args...) }

  // After
  warn := func(msg string, attrs ...slog.Attr) {
      slog.LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
  }
  ```

- **bootstrap.Summary**:
  ```go
  // Before
  s := bootstrap.NewSummary("svc", "1.0")
  s.DisplaySummary(reg, c, log) // wrote to stdout

  // After
  s := bootstrap.NewSummaryWithOptions("svc", "1.0",
      bootstrap.WithWriter(myWriter))
  s.DisplaySummary(reg, c, log)
  ```

### Breaking Changes
- **kafka → messaging**: The `gokit/kafka` module has been restructured into `gokit/messaging`
  - Abstract interfaces (`Producer`, `Consumer`, `Message`, `Event`) now live in `github.com/kbukum/gokit/messaging`
  - Kafka-specific code moved to `github.com/kbukum/gokit/messaging/kafka`
  - Middleware moved to `github.com/kbukum/gokit/messaging/middleware` (broker-agnostic)
  - New `InMemoryBroker` in `github.com/kbukum/gokit/messaging/memory` for testing
  - Old `gokit/kafka` module has been removed

### Migration
- `github.com/kbukum/gokit/kafka` → `github.com/kbukum/gokit/messaging/kafka`
- `github.com/kbukum/gokit/kafka/producer` → `github.com/kbukum/gokit/messaging/kafka/producer`
- `github.com/kbukum/gokit/kafka/consumer` → `github.com/kbukum/gokit/messaging/kafka/consumer`
- `github.com/kbukum/gokit/kafka/middleware` → `github.com/kbukum/gokit/messaging/middleware`
- Abstract types (`Message`, `Event`, `MessageHandler`) now in `github.com/kbukum/gokit/messaging`

### Added — Messaging Enhancement

- **messaging**: `ManagedConsumer` — wraps any `Consumer` with lifecycle (Start/Stop/IsRunning) and handler dispatch
- **messaging**: `ConsumerRunner` interface and `AsRunner()` adapter for managed consumption loops
- **messaging**: `MetricsCollector` interface with `RecordPublish()`/`RecordConsume()` for broker-agnostic metrics
- **messaging**: `ErrorTranslator` interface for converting raw errors to `*AppError`
- **messaging**: `ErrorClassifier` interface with `IsConnectionError()`/`IsRetryableError()` helpers
- **messaging**: `BrokerComponent` interface extending `component.Component` with `Producer()`/`Consumer()` accessors
- **messaging**: `MessageHandler` type + `HandlerMiddleware` + `ChainHandlers()` for composable handler pipelines
- **messaging**: `MessageRouter` — topic-based message routing with exact match and wildcard (`*`) pattern support
- **messaging**: `BatchProducer` — buffered producer with size, time (MaxWait), and byte (MaxBytes) flush triggers
- **messaging/bridge**: `ProducerAsSink()` — adapts `Producer` to `provider.Sink[Message]`
- **messaging/bridge**: `EventProducerAsSink()` — adapts `EventProducer` to `provider.Sink[Event]`
- **messaging/bridge**: `ConsumerAsStream()` — adapts `Consumer` to `provider.Stream[struct{}, Message]`
- **messaging/middleware**: `DedupHandler` — deduplication middleware with LRU cache, TTL, and bounded window
- **messaging/middleware**: `CircuitBreakerHandler` — fail-fast middleware wrapping `resilience.CircuitBreaker`
- **messaging/memory**: Enhanced `InMemoryBroker` with message history, topic management, and reset capability
- **messaging/memory**: Test assertions — `AssertPublished()`, `AssertPublishedN()`, `WaitForMessage()`, `AssertNoMessages()`

### Added
- **bench**: New sub-module — pluggable evaluation framework for benchmarking providers against labeled datasets
  - Core types: `Sample[L]`, `Prediction[L]`, `ScoredSample[L]`, `LabelMapper[L]`
  - `DatasetLoader[L]`: manifest-based dataset loading with filtering and pipeline integration
  - `Evaluator[L]`: provider adapter interface with `EvaluatorFunc` and `FromProvider` helpers
  - `BenchRunner[L]`: orchestrates evaluation runs with multi-branch support and concurrency
  - `FileStorage`: JSON file-based run result persistence with listing, filtering, and Latest()
  - `RunComparator`: compares two runs with metric diffs, regression detection, and sample-level tracking
  - Result types: `RunResult`, `MetricResult`, `BranchResult`, `SampleResult`, `RunSummary`
- **bench/metric**: Pluggable metric implementations for evaluation scoring
  - Classification: `BinaryClassification`, `MultiClassClassification`, `ConfusionMatrix`, `ThresholdSweep`
  - Probability: `AUCROC`, `BrierScore`, `LogLoss`, `Calibration`
  - Ranking: `NDCG`, `MAP`, `PrecisionAtK`, `RecallAtK`
  - Regression: `MAE`, `MSE`, `RMSE`, `RSquared`
  - Matching: `ExactMatch`, `FuzzyMatch` (Levenshtein-based)
  - Composite: `Weighted` for combining metrics with weights
  - `Suite[L]` for batch metric evaluation; `AsRunMetric`/`AsRunMetrics` adapters
- **bench/report**: Formatted output generation from benchmark results
  - `Reporter` interface with `JSON`, `Markdown`, `Table`, `CSV`, `JUnit`, `VegaLite`, `HTML` implementations
- **bench/viz**: SVG visualization generation from run results
  - `RenderAll` generates applicable charts; individual renderers for ROC, calibration, confusion matrix, distribution, branch comparison
- **bench/storage**: `ProviderStorage` adapter bridging `bench.RunStorage` with `gokit/storage.Storage` backends
- **tests**: Comprehensive test suite for bench module — types, dataset loading, evaluator adapters, runner, file storage, comparator, classification metrics, probability metrics, regression metrics, matching metrics, JSON reporter
- **docs**: Package-level documentation for bench/metric, bench/report sub-packages

- **provider**: Sink combinator primitives for composable push-based data flow
  - `NewSinkFunc[I]`: wraps a plain `func(ctx, I) error` as a `Sink[I]` (like `http.HandlerFunc`)
  - `FanOutSink[I]`: dispatches input to multiple sinks in parallel, joins errors
  - `AdaptSink[I, BI]`: transforms input types before sending (mirrors `Adapt` for `RequestResponse`)
  - `TapSink[I]`: adds a side-effect observer before forwarding to the inner sink
  - `SinkMiddleware[I]` + `ChainSink[I]`: composable wrapping for sinks (mirrors `Middleware` + `Chain`)
- **tests**: 11 sink combinator tests — SinkFunc, FanOutSink (parallel, errors, passthrough, availability), AdaptSink (mapping, errors), TapSink, ChainSink (ordering)
- **docs**: Updated `provider/doc.go` with Sink Combinators section and composition examples

## [0.1.5] - 2026-03-01

### Added
- **llm**: New sub-module — config-driven LLM adapter with Dialect pattern
  - Universal types: `CompletionRequest`, `CompletionResponse`, `StreamChunk`, `Message`, `Usage`
  - `Dialect` interface for provider-specific HTTP mapping (follows `database/sql` driver pattern)
  - Thread-safe dialect registry: `RegisterDialect()`, `GetDialect()`, `Dialects()`
  - `Adapter` composing REST client + Dialect with `New()` and `NewWithDialect()` constructors
  - Streaming support for both NDJSON (Ollama) and SSE (OpenAI/Anthropic) formats
  - Convenience helpers: `Complete()`, `CompleteStructured()` with JSON extraction
  - Full config: auth, TLS, retry, circuit breaker, rate limiter — all inherited from httpclient
  - Ships with zero built-in dialects — implementations live in separate driver modules
- **provider**: `Streamable[I, O, C]` interface for providers supporting both request-response and streaming modes
- **httpclient**: `MultipartBody` and `FileField` types for multipart/form-data requests
  - `encodeBody()` auto-handles `*MultipartBody` — no more manual `mime/multipart` construction
  - Supports custom content-type per file, streaming upload via `io.Reader`
- **httpclient/rest**: `Client` now implements `provider.Provider` (Name, IsAvailable, Close)
- **httpclient/rest**: Error helper re-exports (`IsNotFound`, `IsAuth`, `IsRateLimit`, `IsServerError`, `IsRetryable`, `IsTimeout`)
- **tests**: 27 LLM adapter tests (81.7% coverage) — adapter, dialect registry, streaming, helpers, types
- **tests**: 5 multipart encoding tests — fields, files, custom content-type, reader, full adapter integration
- **tests**: 3 REST provider interface tests — Name/IsAvailable/Close delegation, error classification
- **docs**: `adapter-derivation-plan.md` — architecture plan for layered adapter composition

## [0.1.4] - 2026-03-01

### Added
- **github**: CODEOWNERS file for automatic code review assignment
- **github**: Issue templates for bug reports and feature requests (YAML forms)
- **github**: Pull request template with comprehensive checklist
- **docs**: CODE_OF_CONDUCT.md based on Contributor Covenant 2.1
- **docs**: SECURITY.md with responsible disclosure policy
- **docs**: adapter-guide.md documenting adapter pattern across all modules
- **docs**: adapter-framework-plan.md for adapter architecture planning
- **docs**: pipeline/README.md with comprehensive operators guide and 7 usage examples
- **kafka/producer**: `adapter.go` implementing `provider.Sink[Message]` with Send method
- **kafka/producer**: Availability checks for producer health monitoring
- **kafka/consumer**: `adapter.go` implementing provider interface
- **kafka**: `FromKafka` error translation utility for consistent error handling
- **kafka**: Message struct with JSON handling and Kafka message conversion
- **kafka**: MockProducer with Publish methods and message tracking for testing
- **process**: Process adapter for subprocess execution with timeout and grace period
- **provider**: Health status reporting interface for all providers
- **redis**: Availability checks for Redis client health monitoring
- **redis**: Name field in Redis config for component identification
- **storage**: Availability checks for storage component health monitoring
- **storage**: Name field in storage config for component identification
- **httpclient**: Component implementation with lifecycle management
- **httpclient**: REST client with simplified interface
- **httpclient**: Options pattern for HTTP client configuration
- **database**: Adapter pattern implementation
- **grpc/client**: Adapter pattern implementation
- **security/tlstest**: Utility for generating TLS certificates in tests
- **tests**: Comprehensive test suite for encryption (ChaCha20 encryption/decryption, error handling)
- **tests**: Logger tests (metadata, context, component registration)
- **tests**: Observability tests (tracing, metrics, health checks)
- **tests**: Process tests (availability checks, command execution failures)
- **tests**: Resilience tests for process execution with retries
- **tests**: SSE hub tests (client registration, lifecycle, event serving)
- **tests**: Versioning tests (version info, dirty builds, branch names)
- **tests**: Kafka component tests (producer/consumer lifecycle, config, errors)
- **tests**: Kafka connection, metrics, translator, and types tests
- **tests**: Security TLS configuration tests (valid/invalid scenarios)
- **tests**: httpclient component and REST client tests

### Changed
- **README.md**: Added contributing section with CODE_OF_CONDUCT link
- **kafka**: Enhanced config with name field for better identification
- **redis**: Enhanced config with name field for better identification
- **storage**: Enhanced config with name field for better identification
- **testutil/fixtures**: Updated documentation for clarity
- **httpclient**: Refactored adapter with improved provider interface implementation
- **httpclient**: Enhanced request handling and REST client functionality

### Fixed
- **discovery**: Standardized Go version to 1.25.8 (was 1.25.5 in discovery and discovery/testutil)

## [0.1.2] - 2026-02-24

### Added
- **provider**: `ContextStore[C]` generic interface for typed state persistence.
- **provider**: `MemoryStore[C]` in-memory implementation with TTL enforcement.
- **provider**: `Stateful[I,O,C]` wrapper for automatic state load/save around Execute.
- **provider**: `Middleware[I,O]` type and `Chain` function for composable middleware.
- **provider**: `WithLogging` middleware using `logger.Logger`.
- **provider**: `WithMetrics` middleware using `observability.Metrics`.
- **provider**: `WithTracing` middleware using OpenTelemetry spans.
- **redis**: `TypedStore[C]` implementing `provider.ContextStore[C]` with JSON serialization.
- **redis**: `GetJSON`/`SetJSON` convenience methods on `Client`.
- **pipeline**: `Throttle` operator for rate-limiting values.
- **pipeline**: `Batch` operator for collecting items by size or timeout.
- **pipeline**: `Debounce` operator for quiet-period emission.
- **pipeline**: `TumblingWindow` operator for non-overlapping fixed-duration windows.
- **pipeline**: `SlidingWindow` operator for overlapping windows with configurable slide.

### Removed
- **redis/testutil**: Removed — exposed raw `*goredis.Client` instead of gokit's `*redis.Client`, making it unusable for testing gokit redis operations.

### Changed
- **ci**: Rewritten CI pipeline with dynamic module discovery — no hardcoded module list, per-module parallel jobs, tidy verification gate.

## [0.1.1] - 2026-02-23

### Changed
- Bump inter-module dependencies with local replace directives.

## [0.1.0] - 2024-05-22

### Changed
- **errors**: Modernized module with comprehensive godoc comments and 100% test coverage.
- **errors**: Internal errors are no longer retryable by default for improved safety.
- **core**: Consolidated core packages (errors, util, validation, etc.) into the root module.
- **component**: Renamed `ComponentHealth` to `Health` to avoid stuttering.
- **config**: Renamed `ConfigResolver` to `Resolver` to avoid stuttering.
- **server**: Renamed `ServerComponent` to `Component` to avoid stuttering.
- **resilience**: Updated `ExecuteWithResult` to accept `context.Context` as the first parameter.
- **various**: Updated multiple functions to accept configuration by pointer to improve performance and satisfy linters.
