# Changelog

Notable changes follow [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **connect/client**: `Unary` makes one unary call and returns its validated response, and `CallFailure` turns a failed call into the outage mapping or the peer's typed error, so module clients stop copying the same call pipeline.
- **cli/command**: `Program` runs a command tree on the standard `flag` package: global and command flags in any order, help with examples, typo suggestions for commands and flags, bash/zsh/fish completion scripts and usage errors that exit with `ExitUsage`. `ExitCode` keeps an error's own exit code.
- **cli/cienv**: `Detect` reads the CI provider, repository, commit, branch and pipeline from GitHub Actions, GitLab CI or a generic `CI=true`, as labels safe to record.
- **auth/apikey**: `Manager.RecordUse` records a validated key's last use at minute precision (`UseGranularity`), writing at most once a minute per key.
- **bench/stats**: `BootstrapMean` (mean with a seeded percentile bootstrap interval) and `BradleyTerry` (ranking from pairwise outcomes with seeded intervals).
- **llm**: `CompletionRequest.Seed`, `PresencePenalty` and `FrequencyPenalty`. OpenAI and Gemini send them; Anthropic refuses a request that sets them instead of dropping them.
- **fs**: `ReadZip` reads an untrusted zip held in memory, bounded by `ZipReadLimits` (members, bytes per member and in total, compression ratio). Unsafe, repeated or non-regular members are refused, and each refusal matches an `ErrArchive*` sentinel.
- **media**: `VerifyImage` fully decodes one still PNG, JPEG or WebP within `ImageLimits` (bytes, sides, pixels). The type comes from the bytes, structure and trailing bytes are checked, and SVG, HTML and polyglot files fail with a typed `ImageError` reason.
- **workload**: `ModelPuller` downloads a model and reports `PullProgress` (status, bytes done, total). `workload/dmr` and `workload/ollama` implement it; an error from the progress callback or a canceled context stops the pull, and a truncated or failed stream is never reported as success. `ollama.Lister` is now `ollama.Client`, with `Config.PullTimeout`.
- **workload/ollama, workload/lmstudio, workload/openaicompat**: model listers that describe each model a runtime holds as an `ai.Model`, through the shared `httpclient`, with an optional endpoint policy and header credential, bounded bodies and status-only errors. Ollama describes models a few at a time, and a model whose description fails, or is not reached before the caller's deadline, keeps its listed facts instead of failing the whole listing.
- **llm/providers/openai**: user messages send image parts as `image_url` content (a base64 data URL or an http(s) source); any other non-text part fails instead of being dropped.
- **llm/testutil**: `OpenAIServer` lists its models on `GET /v1/models`, accepts multi-part message content, and `vision-*` models name the color of a PNG image. `WithKeyless` answers without a bearer token, like a local runtime.
- **server/spa**: `Config.ImmutableDirs` caches every file beneath a fingerprinted build directory, such as Vite's `assets`, for one year, so a runtime-loaded build needs no manifest list.
- **database/postgres**: `Migrate` opens a short-lived connection with the migration login, applies a `SchemaPlan` and closes it, bounded per migration set.
- **server/spa**: `NewDir` serves a build directory as a lifecycle component that opens the confined directory on Start, closes it on Stop, reports health, and answers 503 while closed.
- **llm/testutil**: `OpenAIServer`, an OpenAI-compatible chat-completions HTTP fake that echoes the last user message with usage, streamed or not, counts calls per model, and fails `fail-*` models with 502.
- **server / server/middleware**: `NewAuthStages` and `Server.Authenticate` authenticate a request once, before tracing, so the same result decides access and whether to continue the caller's trace.
- **httpclient**: `Config.MaxConnsPerHost` caps connections per host, separate from the endpoint policy. The README lists every call budget.
- **tooling**: `make consumer-check CONSUMER=<dir>` builds and vets a consumer module against this checkout.
- **tooling**: `make fuzz`, `make fuzz-plan`, `make vuln`, `make licenses`, `make tidy-check` and `make actionlint` run the same checks as CI.
- **connect**: `HeaderInterceptor` sets a fixed header, such as a service key, on every unary and streaming client call and replaces a caller's value.
- **resilience**: `ConcurrencyLimit` bounds concurrent work overall and per key without waiting; a slot is taken first and bound to its key once known, such as after authentication.
- **observability**: `ObserveGauges` registers observable gauges built with `Int64Gauge` and `Float64Gauge` under one callback, so values that belong together come from one snapshot.
- **util**: `AfterFunc` runs a function after a delay on a `TimerClock`; its stop prevents the call or waits for it to return, so the function never runs after stop.
- **messaging/nats/jetstream**: a durable, at-least-once adapter registered as `jetstream`. Publishes wait for the server's acknowledgement and deduplicate by message ID. Durable pull consumers acknowledge after the handler succeeds, redeliver with backoff, and never drop a message by default: a `DeliveryPolicy` with `MaxAttempts` must say whether to hand off (`OnDeadLetter`) or discard. A failed hand-off is retried without running the handler again, and `Bind` refuses a consumer that could lose a failed message. It also declares streams, replays retained messages in bounded pages, and reports receive status. `NewComponent` owns the client in a component registry; registry producers and consumers share one connection per request timeout, and a slow connect never blocks other timeouts or releases.
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
- **connect/client**: `Interceptors` builds availability and timeout interceptors in the right order. Each part is optional, so a client can use timeouts alone. `Peer.Interceptors` adds the peer's availability.
- **messaging**: `ErrTerminal` marks a handler failure that redelivery cannot fix; adapters stop redelivering it and retry middleware stops retrying it.
- **security**: `WithEndpointDialBudget` sets how long an endpoint policy may spend dialing; the dial tries every resolved address.
- **storage/s3/testutil**: `WithImage` runs an immutable alternate artifact, such as a source-built server, without publisher-image fallback; shell-free HTTP readiness and a namespace-sharing listener probe support distroless images.
- **database/postgres/testutil / storage/s3/testutil**: owned container identities support private-network consumers without host-gateway routing.
- **storage**: atomic create-if-absent through `Create`/`CreateProvider` with a portable `ErrExists` identity; S3 (`If-None-Match`), local and in-memory backends implement it, and the component exposes `Create` and `Head`.
- **process**: bounded, explicitly race/coverage-instrumented child builds and live output observers; retained-child signals and exit-status observation preserve ownership and capture failures.
- **observability**: instance-owned Prometheus application/runtime export with bounded collection, private registries and lifecycle-owned reader teardown.
- **encryption / security / httpclient**: injected mounted-key resolution and immutable outbound endpoint grants enforce redaction, DNS-at-dial checks, hostname-verified TLS and redirect/retry refusal for restricted clients.
- **storage/s3**: mounted credential files and authenticated bucket-readiness checks; a separate real S3 fixture verifies scoped access, hardening and process-crash persistence.
- **encryption**: caller-supplied authenticated bindings prevent ciphertext from being moved between tenant, record or version contexts.
- **dataset/record**: cancellable JSONL parsing with explicit byte/record limits and refusal of ambiguous top-level fields or trailing values.
- **bootstrap**: typed module composition, dependency validation, named listeners, module-aware operator tasks, and application-owned health, metrics and diagnostics endpoints.
- **bootstrap/testutil**: module lifecycle, port capture, remote-contract validation, and shared implementation tests.
- **observability**: instance-owned tracing with bounded export, explicit provider injection, and lifecycle-managed shutdown.
- **connect / httpclient / server**: HTTP and RPC tracing, correlated errors, and typed unary forwarding.
- **auth/session**: server-side browser sessions with rotation, cross-instance revocation, CSRF protection, logout-driven stream cancellation, recent-authentication timestamps and batch lookup.
- **auth/lease**: bounded, renewable authorization leases for long-lived work.
- **auth/password**: bounded password workers and verification-cost limits.
- **auth/testhost / auth/apikey/apikeytest**: HTTPS integration fixtures and shared API-key store contracts.
- **cache / resilience**: bounded cache eviction, keyed rate limiting with lifecycle ownership, and bulkhead queues with injected clocks.
- **database/postgres**: PostgreSQL adapter, least-privilege schema provisioning, readiness audits, and isolated database fixtures.
- **database/migration**: schema-qualified migration sets, exact-version application, and coordinated migration sessions.
- **database/repository**: partition-scoped repositories with shared SQLite and PostgreSQL contracts.
- **database**: named components with initialization and readiness hooks.
- **connect/client**: peer availability tracking, transport-failure classification, unary and first-message timeouts, and redirect refusal for injected HTTP clients.
- **fs**: atomic exclusive file creation, bounded inter-process locks, and bounded secret-file reads.
- **util**: shared typed-nil dependency validation.

### Changed
- **bench/metric**: the LLM judge is redesigned. `NewJudge` takes a versioned `Rubric` and judges one output (`Score`) or two in both orders (`Compare`, a tie when the orders disagree), reference-free unless the rubric uses the expected answer, with text and media input. A reply that breaks the strict schema is a `no_verdict`, not an error or a score. `LLMJudge` now wraps a `Judge`, averages judged samples and counts `no_verdict`. `JudgePrompt`, `ParseJudgePrompt`, `DefaultJudgePrompt` and `JudgeVerdict` are removed.
- **ci**: every job runs the same `make` targets as local development through a pinned Toven. Coverage is measured per module in parallel under `-race` and blocks below each module's floor; test-support modules are measured only, and modules still below the floor are held at today's level. Fuzz targets run as balanced parallel shards, only for affected modules on pull requests, with a nightly longer run; integration runs only the modules that have integration tests; Go caches and each fuzz shard's corpus are saved from `main` only. Every job has a time limit, a fuzz time must be positive and fit a shard's budget, and a failed change-scope check fails the gate instead of skipping the Go jobs.
- **tooling**: `make test-integration` runs through Toven and builds the auth test host first; it fails when the integration module list cannot be found.
- **tooling**: domains are Toven `[groups.*]` in `toven.toml`, and Toven enforces their layering. `make check-<domain>` and `G=<domain>` on `build`, `vet`, `test`, `lint` and `test-coverage` run one domain; `toven groups` lists them. The root-module domains `core`, `patterns` and `crosscutting` are now `foundation`. `docs/MODULE-INDEX.md` reads its domains from Toven.
- **storage/local**: canceled conditional creates no longer publish files; copy and temporary-file ownership remain synchronous.
- **encryption**: unavailable or invalid mounted key material reports a cause-preserving dependency failure instead of misleading product not-found.
- **storage**: nonblocking fail-closed lifecycle injection, canceled initialization cleanup, and serialized owned backend teardown prevent unavailable or closing dependencies from appearing ready.
- **storage/s3**: instance-owned bounded HTTP connections, TLS 1.3, redirect refusal, no implicit SDK retries after ambiguous writes, and existence checks that keep authorization, cancellation and availability failures.
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
- **ai**: `Model` is a descriptor with family, variant, digest, task, input and output modalities, and parameters. `Capabilities` fields are tri-state `Fact`s and `Known` values that record their `Source`; the zero value is unknown and never supported. `MaxInputTokens` and the `ReasoningTokens` flag are replaced by `ContextLength`, `Reasoning` and `Capabilities.InputBudget`, which `agent` uses for compaction.
- **workload**: `ModelInfo.Model` describes each listed model. **workload/dmr** fills it, reads human-readable sizes, accepts an `EndpointPolicy`, and `Error` no longer carries a response body.
- **messaging**: `NewManagedConsumer` validates its config and returns an error. It restarts only when `Restart` allows, bounds each message with `HandleTimeout`, reports outcomes to a `ConsumerObserver`, and keeps consuming after the Start context ends until Stop. Health is degraded while restarting or not receiving.
- **messaging/nats/jetstream**: `ErrTerminal` moved to `messaging.ErrTerminal`. `OnError` receives adapter failures only; handler errors stay with the handler and its observers. `NewComponent` validates and returns an error.
- **httpclient**: query strings must come from `Request.Query`, and errors redact them.
- **bootstrap**: `WithAdminMetrics` replaces `AdminConfig.Metrics`. `ReadyCheck` follows the same rule as `/readyz`. `Need` reports a type mismatch as an error, and port tasks report `ErrInvalidPortTask` or `ErrPortNotProvided`.
- **database**: `NewComponent(cfg, log, opts...)` takes construction-time options (`WithComponentName`, `WithDialect`, `WithConnectOptions`, `WithStartupCheck`, `WithInitialize`, `WithReadiness`, `WithHealthTimeout`, …); `WithStartupCheck` runs before `WithInitialize` and returns an error; the post-construction setters are gone. `Failure` is renamed `WrapFailure`.
- **database/postgres**: integration tests moved to the `database/postgres/testutil` module, so the adapter no longer depends on testcontainers.
- **resilience**: `NewKeyedComponent` returns `(*KeyedComponent, error)` and validates its config.
- **server/middleware / connect/client**: `StreamWriteDeadline` and `FirstMessageTimeoutInterceptor` use the system clock by default; inject a test clock with `WithStreamDeadlineClock` or `WithFirstMessageClock`.
- **fs**: `ReadSecret(path, SecretBounds{Min, Max})` replaces the positional bounds.
- **auth/lease**: `Set.Revoke` returns an error and refuses a nil match.
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

### Behavior changes

Defaults and rules that change what an existing app sees without a compile error:

- **security**: an endpoint policy's dial budget (DNS plus TCP) is 10s, up from 2s. Set it with `WithEndpointDialBudget`.
- **security**: shared address space (100.64.0.0/10), benchmarking and IETF protocol ranges need a private grant. NAT64 and 6to4 addresses are judged by the IPv4 address inside them.
- **httpclient**: endpoint-policy clients no longer cap headers and TLS at 2s or connections at 8 per host. `Timeout` bounds each attempt, and `MaxConnsPerHost` sets the cap for any client.
- **httpclient**: negative `Timeout` and `MaxResponseBodyBytes` fail at `New` instead of falling back to defaults.
- **database**: the default health timeout is 1s, up from 500ms.
- **messaging/nats/jetstream**: a consumer retries a failing message until it succeeds, unless `DeliveryPolicy.MaxAttempts` is set with a hand-off or discard.
- **connect/client**: `Interceptors` takes options only. A zero timeout is an error; leave the option out for no timeout.

### Removed
- **tooling**: `scripts/affected-domains.sh`; Toven selects affected modules.
- **tooling**: `domains.toml` and `scripts/check-domain.sh`; Toven groups replace them.
- **server/middleware**: `GinRequestLogger`; `RequestLogger` already logs every route at the server level.
- **auth/apikey**: standalone authentication middleware; API keys use the shared authentication chain.
- **database**: legacy tenant-scoping and session-setting helpers; use scoped repositories and transaction-local settings.
- **database**: untimed `Ping`; use context-aware `PingContext`.

### Security
- **dependencies**: `golang.org/x/net` v0.62.0 resolves GO-2026-6860 and GO-2026-6866 (`html` parsing and rendering).

### Fixed
- **storage/s3**: response headers share the existing ten-second request budget instead of timing out at two seconds while storage commits a write. Earlier caller deadlines, connection bounds and no-retry behavior remain unchanged.
- **ci**: pull requests that only change docs now report `CI Status` instead of waiting for it forever.
- **tooling**: `make replace-sync` adds local `replace` lines for intra-repo modules reached through other modules, so `go mod tidy` no longer needs unpublished versions.
- **ci**: the Dependabot tidy workflow passes the branch name through an environment variable instead of inline script text.
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
