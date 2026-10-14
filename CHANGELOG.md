# Changelog

Notable changes follow [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **process**: bounded, explicitly race/coverage-instrumented child builds and live output observers; retained-child signals and exit-status observation preserve ownership and capture failures.
- **observability**: instance-owned Prometheus application/runtime export with bounded collection, private registries and lifecycle-owned reader teardown.
- **encryption / security / httpclient**: injected mounted-key resolution and immutable outbound endpoint grants enforce redaction, DNS-at-dial checks, hostname-verified TLS and redirect/retry refusal for restricted clients.
- **storage/s3**: mounted credential files and authenticated bucket-readiness checks; a separate real S3 fixture verifies scoped access, hardening and process-crash persistence.
- **encryption**: caller-supplied authenticated bindings prevent ciphertext from being moved between tenant, record or version contexts.
- **dataset/record**: cancellable JSONL parsing with explicit byte/record limits and refusal of ambiguous top-level fields or trailing values.
- **bootstrap**: typed module composition, dependency validation, named listeners, and module-aware operator tasks.
- **bootstrap**: application-owned health, metrics, and diagnostics endpoints, including opt-in container probe access.
- **bootstrap/testutil**: module lifecycle, port capture, remote-contract validation, and shared implementation tests.
- **observability**: instance-owned tracing with bounded export, explicit provider injection, and lifecycle-managed shutdown.
- **connect / httpclient / server**: HTTP and RPC tracing, correlated errors, and typed unary forwarding.
- **auth/session**: server-side browser sessions with rotation, cross-instance revocation, CSRF protection, and logout-driven stream cancellation.
- **auth/session**: recent-authentication timestamps, trusted session references, batch lookup, and transactional subject revocation.
- **auth/lease**: bounded, renewable authorization leases for long-lived work.
- **auth/password**: bounded password workers and verification-cost limits.
- **auth/testhost / auth/apikey/apikeytest**: HTTPS integration fixtures and shared API-key store contracts.
- **cache / resilience**: bounded cache eviction, keyed rate limiting, and bulkhead queues with injected clocks.
- **resilience**: lifecycle ownership for keyed rate limiters.
- **database/postgres**: PostgreSQL adapter, least-privilege schema provisioning, readiness audits, and isolated database fixtures.
- **database/migration**: schema-qualified migration sets, exact-version application, and coordinated migration sessions.
- **database/repository**: partition-scoped repositories with shared SQLite and PostgreSQL contracts.
- **database**: named components with initialization and readiness hooks.
- **connect/client**: peer availability tracking, transport-failure classification, and unary and first-message timeouts.
- **fs**: atomic exclusive file creation, bounded inter-process locks, and bounded secret-file reads.
- **util**: shared typed-nil dependency validation.

### Changed
- **encryption**: unavailable or invalid mounted key material reports a cause-preserving dependency failure instead of misleading product not-found.
- **storage**: nonblocking fail-closed lifecycle injection, canceled initialization cleanup, and serialized owned backend teardown prevent unavailable or closing dependencies from appearing ready.
- **storage/s3**: instance-owned bounded HTTP connections, TLS 1.3, redirect refusal, and no implicit SDK retries after ambiguous writes.
- **storage/s3**: existence checks preserve authorization, cancellation and availability failures instead of falsely reporting missing objects.
- **worker**: scheduled workers reject invalid/canceled startup and honor teardown deadlines while draining before dependencies.
- **bootstrap / database / logging**: clearer startup summaries, consistent component names, and optional banners for one-shot commands.
- **component**: phased, concurrent draining with reserved teardown time.
- **logging**: idempotent close and deadline-bounded exporter shutdown.
- **database/migration**: read-only version and readiness checks that require no migration lock.
- **fs / security**: bounded, regular-file reads for secrets and TLS material, with preserved I/O failures.
- **server**: shared safe HTTP error responses, validated TLS startup, and named server components.
- **connect/client**: strict URL and transport validation; redirects and cleartext downgrades are rejected.
- **errors / resilience / grpc**: consistent cancellation classification, explicit retry verdicts, and preserved remote retry delays.
- **auth / database**: stronger key validation, failure classification, and cancellation handling.
- **toolchain / dependencies**: updated Go toolchain, workspace dependencies, and pinned validation tools.
- **connect / auth / storage**: modern HTTP/2 transport, EC-key parsing, and service-account credential loading.
- **sse/testutil**: reusable test and benchmark helpers with failure-path coverage.

### Breaking Changes
- **storage**: factories and `New` receive the caller's context so provider initialization respects startup cancellation and deadlines.
- **bootstrap / component**: typed lifecycle and rollback errors, single-use application lifecycles, and explicit resource ownership.
- **bootstrap / server**: listeners return registration errors; diagnostics move from server ownership to the application.
- **bootstrap / auth/session / connect/client**: consistent registration, authentication, and client-failure API names.
- **auth**: typed principals and fail-closed authentication chains replace ambiguous credential handling.
- **auth/session**: browser handlers use a validated backend port for local or remote session authorities.
- **auth/jwt**: explicit token profiles, validated key sets, injected clocks, and bounded rotation replace legacy token helpers.
- **auth/apikey**: explicit dependencies, atomic rotation, and one-way revocation replace mutable activation and rotation APIs.
- **auth/password / cache**: constructors validate configuration and return errors.
- **auth/lease / auth/session**: owned renewal workers and coalesced nudges replace synchronous renewal.
- **config**: explicit missing files and invalid profiles return errors; filesystem inspection preserves I/O failures.
- **database**: driver-independent connection preparation, explicit credentials, and error-returning lifecycle access replace legacy connection APIs.
- **database/migration**: drivers use schema-qualified version tables; custom SQL backends must support table-existence checks.
- **errors / validation / transports**: derived statuses, semantic violations, and a shared RPC codec replace the previous failure contract.
- **provider / process / resilience**: invalid construction returns errors rather than producing apparently available resources.
- **sse**: scoped, resumable events with bounded admission and replay replace the monolithic hub.
- **observability**: tracing requires explicit injection rather than global provider registration.
- **fs**: archive extraction rejects absolute member paths instead of normalizing them beneath the destination.

### Removed
- **auth/apikey**: standalone authentication middleware; API keys use the shared authentication chain.
- **database**: legacy tenant-scoping and session-setting helpers; use scoped repositories and transaction-local settings.
- **database**: untimed `Ping`; use context-aware `PingContext`.

### Fixed
- **fs**: harden ZIP and tar extraction path validation.
- **logging**: replace duplicate scoped attributes and preserve valid trace identifiers during masking.
- **bootstrap**: prevent readiness races during shutdown and preserve cancellation during startup and operator tasks.
- **bootstrap**: protect reserved routes from fallback handling and isolate admin tests from ambient proxies.
- **server**: close idle pre-request connections during drain and return safe errors when response encoding fails.
- **connect/client**: preserve timeout, retry, and transport-failure semantics across stream setup and cleanup.
- **sse**: clear per-frame write deadlines so quiet HTTP/2 streams remain healthy.
- **database**: close failed connection attempts and bound retries through the shared resilience policy.

## [0.3.0-alpha.1] - 2026-08-25

### Added
- **discovery**: shared load-balancing strategies, tracked connection load, and lifecycle-owned service registration.
- **grpc**: lossless application-error mapping and an in-memory client/server test harness.
- **sse**: bounded typed events with replay and slow-subscriber isolation.
- **component**: bounded concurrent startup, lifecycle timeouts, lazy construction, and health helpers.
- **resilience**: retry presets, elapsed-time budgets, and validated resilience configuration.
- **observability**: validated HTTP and gRPC OTLP exporter selection.
- **security / util**: canonical authentication schemes and constant-time comparison.

### Changed
- **component**: default startup budgets bound components without caller deadlines.
- **mcp**: SDK 1.7.0 and hardened interactive tools support multi-round-trip protocol requests; standalone requests remain limited to older clients.

### Security
- **toolchain**: Go 1.26.6 resolves standard-library vulnerability findings.
- **dependencies**: approved CC0-1.0 licensing for BLAKE3.

### Fixed
- **workload/docker**: deterministic cancellation while waiting for containers.
- **release**: propagate publish failures, verify a clean worktree, and use pinned Go and Toven versions.

## [0.2.0-alpha.1] - 2026-07-19

### Added
- **dataset**: generic streaming collection with bounded workers, validation, resumable manifests, and record and blob sources.
- **mcp**: prompts, resources, subscriptions, sampling, elicitation, progress, and audit support.
- **mcp**: protected Streamable HTTP with origin validation and optional bearer authentication.
- **ai / llm**: validated raw-JSON extensions and bounded streaming tool arguments.
- **inference**: shared, cancellation-aware completion streaming for TGI and vLLM.
- **cli**: terminal themes, rendering, progress, prompts, signal handling, and bounded live output.
- **codec**: typed JSON/TOML encoding, value merging, and bounded framing.
- **fs**: confined paths, temporary resources, atomic writes, permissions, and metadata.
- **registry / di**: shared typed registries and type-safe dependency keys.
- **bench / ci**: broader benchmark coverage and cross-module performance comparisons.
- **docs**: project documentation index and sibling-project references.

### Breaking Changes
- **ai / llm / tool**: raw JSON and typed structured results replace untyped payload maps.
- **tool**: validate input before side effects and require approval for destructive operations.
- **mcp**: typed, hardened server APIs support stdio and Streamable HTTP; standalone SSE is removed.
- **logging / stream**: canonical package names replace `logger` and `pipeline`.
- **logging / version**: injected registries and immutable defaults replace mutable global state.
- **errors / schema**: generic resource errors and bounded schema compilation and validation.
- **auth / tool / workload / llm / storage / discovery**: registry operations return errors rather than overwriting or panicking.
- **di / auth/authctx / server/middleware / agent**: remove runtime panic-oriented convenience APIs.

### Internal
- **registries**: first-party implementations share the lightweight named-registry owner.
- **security**: document the time-limited, unreachable OpenPGP advisory exception and remove stale Docker suppressions.

### Security
- **coverage**: enforce security-module, patch, and no-regression gates.
- **fuzzing**: discover and exercise fuzz targets across modules.
- **ci**: add Go compatibility, PR-title, and dependency-license checks with pinned actions and least-privilege jobs.
- **release**: publish checksums, SBOMs, signatures, and build provenance.

## [0.2.0] - 2026-04-25

> Historical development baseline. Its tags were removed before the prerelease line began.

### Added
- **workload / llm**: explicit factory and dialect registries.
- **bootstrap**: injectable startup-summary output.
- **messaging**: broker-independent interfaces, managed consumption, routing, batching, metrics, and error classification.
- **messaging**: provider bridges, deduplication, circuit-breaking middleware, and in-memory test support.
- **bench**: dataset-driven evaluation, run storage, comparisons, and sample-level results.
- **bench**: classification, probability, ranking, regression, and matching metrics.
- **bench**: structured reports, visualizations, and storage-provider integration.
- **provider**: composable sinks with fan-out, adaptation, observation, and middleware.
- **ci / governance**: workspace configuration, ownership documents, dependency updates, and security-reporting guidance.
- **tests / docs**: broader examples, package documentation, lifecycle tests, and allocation-aware benchmarks.

### Breaking Changes
- **workload / llm / storage / discovery**: explicit injected registries replace global registration and import-time setup.
- **di**: generic resolution replaces untyped runtime convenience methods.
- **config**: structured warning attributes replace formatted warnings and returned warning lists.
- **bootstrap**: startup summaries write through an injected writer.
- **server/middleware**: authentication construction returns errors; invalid credentials are rejected by default.
- **server/middleware**: simplify tenant-context access and remove redundant helpers.
- **messaging**: broker-independent interfaces replace the Kafka-only module; Kafka adapters move under `messaging/kafka`.

### Security
- **ci**: per-module vulnerability scans, multi-OS tests, race/shuffle checks, fuzz smoke tests, and Go-version consistency.
- **lint**: stronger error, concurrency, SQL cleanup, and shadowing checks.
- **gosec**: replace the global TLS-verification exclusion with documented, site-specific exceptions.

## [0.1.5] - 2026-03-01

### Added
- **llm**: configurable dialect-based completion and streaming adapters with structured-output helpers.
- **provider**: a shared interface for request-response and streaming providers.
- **httpclient**: streaming multipart uploads and REST provider integration.
- **httpclient/rest**: shared error classification helpers.
- **tests / docs**: adapter, streaming, multipart, and provider coverage with a layered composition guide.

## [0.1.4] - 2026-03-01

### Added
- **github / docs**: review ownership, issue and PR templates, conduct and security policies, and adapter guides.
- **kafka**: provider adapters, error translation, message conversion, and test doubles.
- **process**: subprocess lifecycle with timeouts and graceful termination.
- **provider / redis / storage**: shared health reporting and component identification.
- **httpclient / database / grpc**: lifecycle-managed adapters and simplified clients.
- **security/tlstest**: reusable TLS certificate fixtures.
- **tests**: broader security, logging, tracing, process, resilience, SSE, versioning, Kafka, and HTTP coverage.

### Changed
- **httpclient**: align adapter composition and request handling with provider contracts.
- **redis / storage / kafka**: consistent component naming.
- **docs**: clearer contribution, fixture, and pipeline guidance.

### Fixed
- **discovery**: align its Go version with the workspace.

## [0.1.2] - 2026-02-24

### Added
- **provider**: typed state storage, TTL-backed memory state, and stateful execution.
- **provider**: composable logging, metrics, and tracing middleware.
- **redis**: typed provider state and JSON helpers.
- **pipeline**: throttling, batching, debouncing, and time-window operators.

### Removed
- **redis/testutil**: the incompatible raw-client test helper.

### Changed
- **ci**: dynamic module discovery, parallel checks, and tidy verification.

## [0.1.1] - 2026-02-23

### Changed
- **dependencies**: update inter-module versions and local replacement directives.

## [0.1.0] - 2024-05-22

### Changed
- **errors**: improve documentation and coverage; internal errors are non-retryable by default.
- **core**: consolidate foundational packages in the root module.
- **component / config / server**: simplify public type names.
- **resilience**: make execution context-first.
- **apis**: align configuration passing with pointer-based conventions.
