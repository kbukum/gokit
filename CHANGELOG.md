# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
