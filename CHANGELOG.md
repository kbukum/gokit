# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
- **discovery**: Standardized Go version to 1.25.0 (was 1.25.5 in discovery and discovery/testutil)

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

## [Unreleased]

### Added
- Core module: errors, config, logger, util, version, encryption, validation, di, resilience, observability, sse, provider, pipeline, component, bootstrap, security
- Sub-modules: database, redis, kafka, storage, server, grpc, discovery, connect, httpclient, workload, auth, authz, process
- Provider pattern with generic Registry/Manager/Selector
- Pull-based pipeline with Throttle, Batch, Debounce, and Window operators
- Unified HTTP server (Gin + h2c) with middleware and endpoint sub-packages
- Connect-Go integration module for RPC alongside REST
- Multi-module architecture: core stays lightweight, heavy deps in sub-modules
