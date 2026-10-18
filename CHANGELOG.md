# Changelog

Notable changes follow [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **database/postgres**: `Migrate` opens a short-lived connection with the migration login, applies a `SchemaPlan` and closes it, bounded per migration set.
- **server/spa**: `NewDir` serves a build directory as a lifecycle component that opens the confined directory on Start, closes it on Stop, reports health, and answers 503 while closed.
- **llm/testutil**: `OpenAIServer`, an OpenAI-compatible chat-completions HTTP fake that echoes the last user message with usage, streamed or not, counts calls per model, and fails `fail-*` models with 502.
- **connect**: `HeaderInterceptor` sets a fixed header, such as a service key, on every unary and streaming client call and replaces a caller's value.
- **resilience**: `ConcurrencyLimit` bounds concurrent work overall and per key without waiting; a slot is taken first and bound to its key once known, such as after authentication.
- **observability**: `ObserveGauges` registers observable gauges built with `Int64Gauge` and `Float64Gauge` under one callback, so values that belong together come from one snapshot.
- **util**: `AfterFunc` runs a function after a delay on a `TimerClock`; its stop prevents the call or waits for it to return, so the function never runs after stop.
- **messaging/nats/jetstream**: a durable, at-least-once adapter that implements the messaging producer and consumer ports and registers as `jetstream`. It publishes with server acknowledgement and deduplication IDs, consumes through durable pull consumers with explicit acknowledgement and delayed redelivery, declares file-backed streams with explicit limits, and reads bounded replay pages from a sequence or time. It reconnects without a limit by default, bounds every API request, retries only idempotent transient failures, and terminates messages whose handler returns `ErrTerminal`. A consumer that owns its stream can set `Declare` to create or update it each time it binds, so its limits follow the configuration and a stream lost in a restart comes back. `Consumer.ConsumeStatus` reports whether it is receiving; a fetch the server does not complete by its deadline, such as one a missing permission drops, is a retryable `ErrFetchIncomplete`, not an empty fetch. `ErrorClassifier` treats JetStream rejections that depend on server state (limits, placement, a consumer created or deleted concurrently) as retryable. `NewComponent` owns the client in a component registry, reports degraded health while the broker is unreachable and unhealthy once the connection closed for good, and drains within the stop context.
- **messaging/nats**: `Connect` opens a validated connection; `Config.PasswordFile` reads the password from a mounted secret at every connect, so a rotated password applies on reconnect. `Config.InboxPrefix` gives a client its own reply inbox, and `Config.UserInbox` derives it as `_INBOX.<username>`, so a broker user can be limited to its own replies. `Config.Validate` rejects a URL with a path, query or fragment, and a CA file with a cleartext URL.
- **messaging/nats/testutil**: `StartJetStream` runs a pinned, authenticated, hardened single-node JetStream server for integration tests; `WithUser` adds a user with limited rights and `Restart` restarts it on the same address.
- **database/testutil**: `InjectFaults` fails a chosen SQL statement on a gorm database, and `Faults.EveryStatement` fails each statement of an operation in turn, to prove that every failure is reported and rolled back.
- **database/migration**: `Session.Transaction` runs a transaction on the session's pinned connection; canceling its context stops the statements while the rollback stays on the caller's goroutine.
- **bench**: `SummarizePerformance` summarizes externally collected measurements into the same `latency_ms`, `ttft_ms`, throughput, `failure_rate` and `usage_tokens` metrics a run reports, through `PerformanceSummary.Metrics`.
- **llm**: `Config.EndpointPolicy` applies the httpclient endpoint policy to model calls: operator origin grants, DNS-at-dial checks, and refused redirects, proxies and implicit retries.
- **server/middleware**: `StreamWriteDeadline` bounds each write and flush of a long-lived streaming route and disarms the deadline while idle, so the server's `WriteTimeout`, an absolute deadline per HTTP/2 stream, no longer resets healthy streams.
- **encryption**: `WithKeyMaterial` derives message keys from random key material (32 bytes or more, such as mounted key files) with HKDF-SHA256 instead of PBKDF2, in envelope version 2, so each operation takes microseconds instead of tens of milliseconds. Passphrase and key-material ciphertexts never open each other; shared vectors keep the format identical to rskit.
- **storage/testutil**: `Conformance` checks write-once create, empty-object fencing, exact reads, not-found identity, delete and single-winner concurrent creates; local, in-memory and real S3 backends pass it.
- **database/postgres/testutil**: `Connect` and `Open` let many test binaries share one PostgreSQL server; connected fixtures drop only their own databases and roles, and a configured server never falls back to a container.
- **connect/client**: `Peer` owns one peer's HTTP client and availability as a lifecycle component, reporting health from real calls and releasing idle connections on stop.
- **connect/client**: enforced redirect refusal for injected HTTP clients without mutating their settings or transferring transport ownership.
- **storage/s3/testutil**: `WithImage` runs an immutable alternate artifact, such as a source-built server, without publisher-image fallback; shell-free HTTP readiness and a namespace-sharing listener probe support distroless images.
- **database/postgres/testutil / storage/s3/testutil**: owned container identities support private-network consumers without host-gateway routing.
- **storage**: atomic create-if-absent through `Create`/`CreateProvider` with a portable `ErrExists` identity; S3 (`If-None-Match`), local and in-memory backends implement it, and the component exposes `Create` and `Head`.
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
- **storage/local**: canceled conditional creates no longer publish files; copy and temporary-file ownership remain synchronous.
- **encryption**: unavailable or invalid mounted key material reports a cause-preserving dependency failure instead of misleading product not-found.
- **storage**: nonblocking fail-closed lifecycle injection, canceled initialization cleanup, and serialized owned backend teardown prevent unavailable or closing dependencies from appearing ready.
- **storage/s3**: instance-owned bounded HTTP connections, TLS 1.3, redirect refusal, and no implicit SDK retries after ambiguous writes.
- **storage/s3**: existence checks preserve authorization, cancellation and availability failures instead of falsely reporting missing objects.
- **storage/gcs**: storage closes its owned client through `storage.Closer`; later operations fail instead of panicking.
- **storage/local**: `Create` stages incomplete data in a reserved `.gokit-staging` directory that is hidden from listings and cannot be used as a key.
- **process**: live output observers are serialized across stdout and stderr, so one writer can safely serve both and a failure stops both streams.
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
- **messaging**: `NewManagedConsumer` validates its config and returns an error. It restarts a failed consumer only when `Restart` says so, with doubling delays, bounds each message with `HandleTimeout`, reports handled messages and failures to a `ConsumerObserver` (`LoggingObserver` logs them), and exposes `MessageAge`. Consumption outlives the Start context until Stop, which always closes the consumer and waits for it only as long as its context allows; a slow close finishes after Stop returns. Health is degraded while restarting or while a consumer that implements the new `ConsumerStatus` is not receiving.
- **storage/s3**: `Endpoint` must be a credential-free origin without path, query or fragment and must use https unless the new `AllowInsecureDev` allows http. The unused `UseSSL` field is removed.
- **database/postgres**: `sslmode` `disable` needs the new `ConnParams.AllowInsecureDev`; the test fixtures set it for their local servers.
- **connect/client**: a client without TLS needs `AllowInsecureDev` for cleartext h2c; otherwise validation fails.
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
- **database/migration**: a canceled migration no longer lets `database/sql` close the pinned connection from its own goroutine while the session still unlocks it, which could panic with a nil connection. PostgreSQL schema grants use `Session.Transaction` for the same reason.
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
