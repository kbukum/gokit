# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **cache**: `MemoryConfig.MaxEntries` bounds the in-memory store with least-recently-used eviction (zero means unbounded; replacing a key never evicts) and `MemoryConfig.Clock` injects time. `MemoryConfig.Validate` rejects a negative TTL or bound, `MemoryStore.Len` reports the entry count, and an expired read no longer races a concurrent `Set` of the same key. `NewMemoryStore` now returns `(*MemoryStore, error)`. Pre-stable breaking change.
- **resilience**: `KeyedRateLimiterConfig.MaxKeys` bounds how many keys hold a bucket (default `DefaultMaxKeys`, 100 000) and `Clock` injects time. A full limiter rescans for expired buckets at most once per second and otherwise denies new keys with `RateLimitDecision.Saturated` set, so admission keyed by source or account fails closed instead of growing without bound; keys that already hold a bucket keep their own limit. `server/middleware.RateLimit` inherits the default bound.
- **bootstrap**: `RunPortTask(ctx, app, port, task)` runs a finite task, such as an operator command, against the value a module provides for a port. It adds a module that needs the port and runs `RunTask`, so a missing provider fails startup before the task runs.
- **auth/session**: `Record.AuthenticatedAt` and `State.AuthenticatedAt` report when the user last proved a credential for the family. Create and relogin set it and rotation keeps it; the database adapter stores it in a nullable `authenticated_at` family column (schema version 3) and rejects a create or relogin without it, and a missing value reads as zero, which callers treat as not recent.
- **auth/session**: count-only Store/Manager.RevokeSubject atomically revokes matching families with the partial subject index (schema version 2); one local subject/kind watch scan runs after commit even for zero count. Store.LookupBatch performs authoritative bounded queries of up to 256 references; outages are not absence.
- **bootstrap**: modules. A `Module` declares typed `Port`s it provides and needs and registers through a `ModuleContext`. A port is identified by its `*Port` value, not its name, and ports of different interfaces cannot be converted into each other. Commands compose modules with `App.Use` and named HTTP listeners with `App.RegisterListener`, which takes a `Listener`: a component that quiesces, drains as ingress and mounts routes with `Handle`. `ValueModule` provides a port with a value the command holds, such as a test double, and a split deployment replaces a module with its client module, which provides the same ports remotely. After `OnConfigure` (which may also call `Use` and `RegisterListener`), startup checks the whole set and returns every missing or duplicate port, missing listener, dependency cycle, conflicting listener component name and invalid declaration in one `*ModuleError`, then registers modules in dependency order; `App.CheckModules` runs the same check without starting anything. Listener components start after module components, and on shutdown they drain before module workers. The startup summary lists modules.
- **bootstrap/testutil**: runs modules in tests. `NewApp` returns a quiet App, `Start` starts any App within a startup budget (`WithStartBudget`, 30 seconds by default) and shuts it down when the test ends, `RegisterListener` declares a loopback HTTP `Listener`, and `Capture` reads a port the modules under test provide. `ValidateRemoteShape` and `AssertRemoteShape` check that a port's methods take a `context.Context` first, return an `error` last, and pass no channels, functions, unsafe pointers, interfaces or complex numbers at any depth; self-encoding types are opaque. The loopback `Listener` drains like `server`: it force-closes and cancels requests at the deadline and waits for handlers. `Contract` runs one behavior suite against every implementation of a port, such as the in-process module and its client module's client.
- **server**: `Component.Handle` mounts a handler on the server's ServeMux and `Component.Fallback` serves unmatched requests (`Server.Fallback`, which `MountSPA` now uses), so `*server.Component` is a `bootstrap.Listener`. `Server.Handle` and `Server.Fallback` return `ErrInvalidRoute` for a nil or typed-nil handler and an invalid or conflicting pattern instead of panicking, and `connect.Mount`, `connect.MountServices` and `connect.HandlerMounter.Handle` return that error. Pre-stable breaking change.
- **bootstrap**: `ModuleContext.Fallback(listener, handler)` gives a listener one fallback, such as a single-page app. The App installs it after every module registers and answers 404 under the first path segment of each module route on that listener; it compares unescaped path segments, as `http.ServeMux` does. `Listener` gains `Fallback`, and its `Handle` and `Fallback` return an error that the App reports as `ErrRouteConflict`, which breaks other `Listener` implementations. Pre-stable breaking change.
- **bootstrap**: `WithAdmin(AdminConfig)` adds an App-owned diagnostics listener (`admin`) serving `/livez`, `/readyz`, `/metrics` and optional pprof. It serves without TLS, so `AdminConfig.Validate` accepts only a loopback or private IP host (empty means 127.0.0.1); a typed-nil `Metrics` handler also fails validation. `/readyz` reports `starting` (503) until startup can no longer roll back and `draining` (503) from the App lifecycle, then `not_ready` (503), `degraded` (200) or `ready` (200) from component health; an unhealthy, unknown or zero component status is `not_ready`. It starts first, stops last and never quiesces, so probes answer throughout the drain. `App.AdminAddr()` reports its address.
- **connect/client**: `Config.NoTimeout` leaves the HTTP client unbounded for long-lived streams; combining it with `Timeout` fails validation, and negative `Timeout` or `DialTimeout` values fail too. `IsUnavailable(err)` reports marked transport/context failures or received `Unavailable`, `DeadlineExceeded` or `Canceled` errors, and `NewAvailability(name)` is an interceptor that records it from real calls and reports it as component health. An `IsUnavailable` failure after the caller's own context ended (cancellation or deadline) is ignored, so callers cannot mark a peer unavailable; success and non-outage wire errors mark it available, while local validation or policy failures leave its state unchanged.
- **util**: `IsNil(any) bool`, the shared reflect-based guard for injected interface seams, so a typed-nil dependency (an interface holding a nil pointer/map/slice/channel/func) is rejected at validation instead of panicking on first use. The SSE transport, its metrics registration, and the worker→SSE forwarder validate their injected dependencies through it.
- **database/postgres**: opt-in PostgreSQL adapter with explicit registry selection, structured connection preparation and `MigrateDriver()`. Required integration tests provision isolated PostgreSQL containers and fail when Docker is unavailable.

- **auth/session**: opaque, server-side BFF sessions for browsers. Cookies carry only an opaque token; rows live behind a `Store` with SQL (`auth/session/database`, own `auth_session_schema_migrations` table) and in-memory backends. Rotation, revocation across instances, CSRF, exact cookie flags, bounded store budgets, and live-stream cancellation on logout. Login is two-phase (`BeginLogin` before password verification, `CompleteLogin` after), so a logout during verification wins and a stale revoked cookie recovers into a fresh session. See `auth/session/README.md`.
- **auth/testhost**: a real HTTPS backend host with isolated state and scenario controls, plus shared contract fixtures for frontend and end-to-end proofs.
- **connect/client**: clients from `NewHTTPClient` mark unreachable peers, gateway 502/503/504 responses that carry no RPC error, and connections lost mid-response with `ErrTransport`; `IsTransportFailure(err)` reports it through Connect's wrapping. A gateway can treat a dropped stream as a retryable peer outage without retrying peer decode or contract errors.
- **server/middleware**: `WriteProblemDetails(w, r, err)` exports the safe problem+json writer `HTTPAuth` already used, so hosts encode non-Connect HTTP failures the same way. An error whose details cannot be encoded falls back to a safe Internal problem.
- **connect/client**: `FirstMessageTimeoutInterceptor(limit, clock)` is an interceptor that ends a server or bidi stream when the peer sends no first message within `limit` of opening it, with a `deadline_exceeded` error wrapping `ErrFirstMessageTimeout`, so a `NoTimeout` stream to a peer that accepts but never answers is bounded and `Availability` records the outage. `MapCallFailure(ctx, peer, err)` maps a failure the peer did not answer to `Timeout` or `Canceled` when the caller's context ended and otherwise to `ServiceUnavailable` with reason `ReasonUnavailable`, preserving validated remote retry hints and causes. Malformed details fail closed as Internal; a caller's own deadline is not reported as a peer outage.
- **connect/client**: `UnaryTimeoutInterceptor(limit)` bounds each unary client call. When the limit passes while the caller's context is live, the call fails with a `deadline_exceeded` error wrapping `ErrUnaryTimeout`, which `IsUnavailable` counts and `MapCallFailure` maps to `ServiceUnavailable`, so a hung peer is recorded as an outage instead of reading as the caller's own deadline.
- **bootstrap**: `AdminConfig.AllInterfaces` (`all_interfaces`) binds the admin listener to every interface for container probes. It must be set explicitly, accepts only an empty or unspecified host and refuses pprof; an unspecified host without it fails validation with a hint.
- **auth/lease**: keeps long-lived work alive only while an injected `Checker` keeps confirming it. A `Set` renews every distinct key in one bounded batch per interval, measures leases from check start on monotonic time, ends denied keys, lets unanswered keys run out and enforces expiry in a separate worker, so a stalled authority cannot extend a lease. Lifetimes report `ErrRevoked`, `ErrExpired`, `ErrEnded`, `ErrReleased` or `ErrClosed` through `context.Cause`.
- **auth/session**: `LoginRequest.Source` carries the login connection's IP address from `RemoteAddr` (forwarding headers are never trusted; zero without one) so a remote backend can admit per source. `POST /auth/logout` now requires exactly one `Origin` equal to `HandlerConfig.Origin`, as login does, and fails `CSRF_INVALID` otherwise. `CheckOrigin(r, origin)` exposes the same exact-origin rule for a gateway's other unsafe cookie routes. `LoginBudget` exports the login request's 5-second bound so a remote backend can fit its call inside it.
- **auth/session**: `Manager.Reference(token)` and `Manager.Resolve(ctx, reference)` validate a session by protected reference without CSRF and return its `State` (principal, family, generation, absolute expiry) for trusted callers such as delegation issuers.
- **auth/apikey/apikeytest**: the `apikey.Store` contract suite. `Run(t, Harness{NewStore})` covers digest and id lookup, duplicate rejection, metadata isolation, one-way revocation, rotation as one compare-and-swap, concurrent rotations with exactly one winner, revocation racing rotation, and canceled contexts. The memory store passes it.
- **resilience**: BulkheadConfig.MaxQueue bounds waiters; zero means no queue. NewBulkhead returns a typed validation error for invalid concurrency/queue/wait/clock settings; Clock injects timer scheduling. Session admission has an explicit finite queue.
- **auth/password**: Pool bounds worker/queue admission and owns Close(ctx), queued cancellation and retained draining after timeout. Caller cancellation during non-preemptible work cannot return verification success. VerifyLimits bounds persisted costs and distinguishes unsupported costs from mismatch/corruption.
- **database/migration**: `Config.Table` (`migration.Table{Schema, Name}`), `IsVersionTable` and `IsIdentifier`, so independent migration sets keep separate, schema-qualified version tables. `Config.Apply(ctx, target)` migrates to an exact version after validating the whole source and recorded version first. `Session` (`Begin`, `Plan`, `Apply`, `Close`) pins one connection under the migration lock so a coordinator can apply several `Set`s with backend schema work in between. Test fixtures protect every `*_schema_migrations` table.
- **database/postgres**: `SchemaPlan`, `ApplySchema`, `ReadyAccess` and `Readiness` provision one application schema with a migration role and verify an exact runtime-role allowlist (schema USAGE without CREATE, no database CREATE/TEMPORARY, declared relation and sequence rights, SELECT-only version tables, leaf runtime role without memberships, no MAINTAIN or other prohibited rights); readiness runs the same full audit. `SetLocal(ctx, tx, name, value)` sets a transaction-local custom setting for optional row-level security policies.
- **database/postgres/testutil**: `Fixture.NewDatabase` and `Database.NewRolePair` provide isolated databases with fresh SCRAM-authenticated owner/runtime role pairs; `Close` is idempotent and drops them.
- **database/repository**: `NewScopedSpec`/`ScopedSpec.Bind` build partition-scoped repositories that stamp scope on create, filter every read and write by it, return identical NotFound for missing and foreign rows, update only declared mutable columns without upserting, and ignore clauses already on the bound handle. `repositorytest.Run` is the shared adapter contract, run against SQLite and PostgreSQL.
- **database**: `Component.WithName`, `WithInitialize` and `WithReadiness` (`database.Check`).

### Changed
- **database**: `Dialect.Prepare(ctx, ConnectionInput)` returns a lazy `Opener` with fresh owned pools per attempt. `New`/`NewWithContext` take a dialect and configured connection input; `StructuredDialect`, credential URL assembly and adapter `Open(dsn)` are removed. DSN/Params and Password/PasswordFile are mutually exclusive. Injected `SecretSource` and canonical bounded file reads resolve credentials; `Failure` preserves causes without formatting private diagnostics. PostgreSQL controls TLS 1.3, explicit application-schema/pg_temp ordering, bounded session options and ambient `PG*` rejection. Descriptions omit DSNs. Pre-stable breaking change.
- **fs / security**: bounded Unix file reads open nonblocking before regular-descriptor inspection and preserve close failures; TLS CA, certificate and key files use the same reader with a 1 MiB per-file limit. Test certificate generation supports explicit DNS/IP names for real hostname-rejection proofs.
- **database/migration**: `Version` and `Ready` are read-only. They take no migration lock and create nothing, so a least-privilege runtime role can check readiness and readiness never waits for a running migration; a missing version table reports no version. `SQLBackend` gains `TableExists`, implemented by `database/postgres` (catalog lookup through `search_path`) and `database/sqlite`. Pre-stable breaking change for custom backends.
- **auth/jwt**: a `Config` is one token profile: method, issuer, audience set, `SingleAudience`, required `Type` (`typ` header), required `MaxLifetime` and explicit `Leeway` (zero means none, at most one minute). Keys move out of configuration into a `KeySet` selected by `kid`, with per-key `RetireAt` for rotation overlap, at most eight keys and one unretiring key, minimum RSA, curve, Ed25519 and HMAC sizes, and `ParseKey` for PKCS #8 and PKIX PEM. `NewService(cfg, keys, clock, newClaims)` requires the clock; claims embed `jwt.Registered`. Issuance fills `iat`/`nbf`/`exp`, issuer and a single configured audience, and rejects claims outside the profile. Validation requires a known `kid`, the profile `typ`, `iat <= nbf < exp`, `exp - iat <= MaxLifetime` and any configured audience; it fixes multiple audiences, where only the last configured audience was accepted. Key paths that were validated but never loaded, `Secret`, `RefreshSecret`, TTL fields, `ClockSkew`, `Generate`, `GenerateAccess`, `GenerateRefresh`, `Parse`, `ParseRefresh` and `ValidatorFunc` are removed; register the service as a `TokenValidator` directly. Pre-stable breaking change.
- **auth/lease / auth/session**: synchronous Renew is removed for coalescing Nudge and one owned renewal worker. Error reporting is injected and required; timely partial answers apply independently. Expired-but-indexed entries cannot be renewed by late results. Lifetimes use lease causes. Pre-stable breaking change.
- **auth/apikey**: `NewManager(Config{Store, Hasher, Clock})` requires every dependency, including the clock, and returns an error. `ValidateKeyID` applies the same revocation, expiry, grace and scope rules as `ValidateKey` for trusted callers, and authenticated principals carry the key id as `Reference`. `Store.Rotate(Rotation)` is one compare-and-swap that creates the replacement and starts the old key's grace only while the old key is unrevoked, unrotated and valid, otherwise `CONFLICT`; it replaces `SetRotation`, so rotation cannot race revocation or another rotation. One-way `Store.Revoke` and `Manager.RevokeKey` replace `SetActive`, and `Key.RevokedAt` replaces `IsActive`. `RotateRequest.Grace` is explicit: zero ends the old key at once, at most `MaxGrace` (30 days), never past the old key's expiry; the replacement inherits owner, name, prefix, kind and restrictions. `Key.ValidAt(now)` replaces the wall-clock `IsExpiredPastGrace` and `Validate`; `WithClock`, `ManagerOption`, `RotationConfig` and `DefaultGracePeriod` are removed. Issuing with a past expiry or an invalid prefix returns `INVALID_INPUT`. Pre-stable breaking change.
- **auth/password**: NewHasher validates Config; minimum/maximum length count characters. Verify distinguishes mismatch, malformed encodings and unsupported costs before KDF work. Default limits match issuance, with ceilings Argon2id 256 MiB/6 passes/8 lanes and bcrypt 14. Full bcrypt encoding validation prevents corrupt hashes from becoming credential mismatches. Pre-stable breaking change.
- **bootstrap / auth/session / connect/client**: align pre-stable API vocabulary around `RegisterListener`, `RegisterComponent`/`RegisterComponentInPhase`, `Implementation`, `ValidateRemoteShape`/`AssertRemoteShape`, `LoginCredentials`/`LoginRequest`/`LogoutRequest`, backend `Login`/`Logout`, `ParseCSRFToken`, `MapCallFailure`, and `FirstMessageTimeoutInterceptor`/`ErrFirstMessageTimeout`. Callers and examples use the new names without compatibility aliases.
- **server**: Gin and HTTP authentication error responses share `httpx.WriteProblemDetails`, including validation, safe encoding fallback, request-path instance, no-store, minimum retry delay, and injected logging.
- **sse/testutil**: helpers accept `testing.TB` for reuse by tests and benchmarks; failure contracts are tested alongside successful streams.
- **connect/client**: `NewHTTPClient` refuses every redirect with `ErrRedirect`, including same-origin redirects, so peer responses cannot forward RPC credentials or request bodies to another destination.
- **component**: shutdown no longer splits the deadline evenly across every operation, which shrank draining as components were added. A quarter (`StopReserveDivisor`) is reserved for stops and the release; drainers in one phase drain concurrently, each phase gets an equal share of the drain window left when it starts, and time ingress does not use passes to workers.
- **server**: `Config.Validate` checks `TLS`: any TLS setting requires both `CertFile` and `KeyFile`, and the minimum is TLS 1.2 or newer, so a listener without a certificate fails validation instead of after `Start` has returned. `Start` loads the certificates and serves TLS with them, and fails if they cannot be loaded; it never falls back to cleartext.
- **server**: the server-owned admin listener (`Config.Admin`, `AdminConfig`, `AdminAddr`) is removed in favor of `bootstrap.WithAdmin`; `Stop` gives the whole budget to the public drain. Pre-stable breaking change.
- **connect/client**: `Config.Validate` checks a set `BaseURL`: it must be an absolute `http` or `https` URL with a host and without credentials, query or fragment, and `https` needs TLS settings while `http` (h2c) must have none. The error never echoes the URL.
- **bootstrap**: a `ModuleContext.Fallback` no longer serves a path whose escaped first segment decodes to a reserved prefix plus more path, such as `/auth%2Fx`; it answers 404, and a segment that fails to decode is reserved too.
- **security**: `TLSConfig.CertFile` and `KeyFile` describe the certificate for either role, and a failed load reports "failed to load certificate and key" instead of naming a client certificate on servers.
- **bootstrap**: the App's component registry logs through the App logger instead of a default stdout logger.
- **bootstrap**: startup fails with a `*StartupError` wrapping the context's cause when the caller's context ends during startup, even if the running phase returned nil or its own error. `Run` on an already canceled context no longer starts components, and `RunTask` no longer runs its task on a canceled context.
- **auth/session**: `NewHandler(backend, HandlerConfig{Origin, Errors, Clock})` serves the browser contract over a `Backend` port (`Login`, `Status`, `Logout`). `NewLocalBackend(manager, verifier)` keeps the in-process behavior; a gateway can supply a remote session authority instead. The handler validates origin, JSON, cookie shape and logout CSRF before calling the backend, and rejects grants that are expired, lack a CSRF token, or carry an invalid or non-session principal. `ParseCSRFToken` exposes the shared single-header rule. Pre-stable breaking change.
- **config**: a missing explicit config file, env file or named profile now returns `ErrFileNotFound` instead of silently using defaults, including a resolved file removed before it is loaded. Profile names must be lowercase slugs (`ErrInvalidProfile`). `WithProfileDir` limits the profile search to one directory and `WithoutDiscovery` skips the working-directory search. `Resolver.ResolveFiles` returns an error, and `FileSystem.Exists` returns `(bool, error)` so permission and I/O failures surface instead of reading as "not found". Pre-stable breaking change.
- **auth**: typed `Principal` and a fail-closed `Chain` of authenticators. Ambiguous, duplicate, or mixed credentials are rejected. A missing credential reports `present=false` with no identity; `HTTPAuth` rejects it unless `WithMissingPolicy` explicitly accepts it. Pre-stable breaking change.
- **database/migration**: `DriverFunc` and `NewSQLDriver` take a `migration.Table`; `DropTables` takes the schema. `Ready` reports an unmigrated schema as not ready rather than returning golang-migrate's nil-version error. Pre-stable breaking change.
- **database**: `Component.DB()` returns `(*DB, error)` with the `SERVICE_UNAVAILABLE` application errors `ErrNotStarted`/`ErrStopped` instead of a possibly nil handle. `Start` publishes only after auto-migration, initialization and readiness succeed, closes the pool of a failed attempt and lets concurrent callers wait for the attempt in flight; `Stop` is terminal, prevents late publication and returns its recorded result, including a failed close of a pool it prevented from publishing, on repeat calls; `Health` runs ping and readiness within 500 ms and reports only safe messages. Pre-stable breaking change.
- **security/tlstest**: options merged into `tlstest.go`; package documentation describes the helpers that exist.
- **provider / process / resilience**: `BuildResilience`, the `With*Resilience` wrappers, `NewConnector`, `process.NewRunner` and `InitializeWithResilience` return construction errors instead of storing them and reporting the provider available. A wrap failure closes the initialized, unpublished provider. `BulkheadConfig.Validate` is the single validator `NewBulkhead` uses; `Policy.Validate` is called by httpclient and gRPC configuration. Pre-stable breaking change.
- **errors**: `FromContext(ctx, operation)` classifies a finished context as TIMEOUT (deadline) or CANCELED with `context.Cause`. Password verification, session admission, JWT and API-key validation use it, so a caller that gives up is never reported as a denied credential or a closed session.
- **auth**: API-key entropy failure is INTERNAL rather than an invalid prefix; JWT reports expiry only when it is the token's sole defect; `jwt.ParseKey` returns typed INVALID_INPUT errors that keep their cause; lease `Revoke` predicates run outside the set lock; session cleanup failures reach the injected `ReportError`. The API-key manager lives in `manager.go`.
- **database**: negative `ConnectTimeout` fails validation; connection retries stop on authentication, missing-database and certificate failures; a spent attempt budget is TIMEOUT rather than cancellation; safe database failures normalize to their classification, never the driver cause. PostgreSQL schema options use the shared `migration.IsIdentifier` grammar; Reset drops only tables of the metadata schema; metadata must be a singleton table, not a view; SQLite migration inspection no longer reconfigures the pool.
- **bootstrap**: lifecycle failures are typed. Startup returns `*StartupError` with the failed phase and the rollback result. `RunTask` returns `*TaskError` with both the task error and any shutdown error, and teardown failures return `*ShutdownError`. Rollback errors were logged before; now they are returned. An `App` runs one lifecycle (`ErrLifecycleUsed` afterward), and `Shutdown` runs once and returns the recorded result. `Shutdown` during startup cancels it with `ErrShutdownRequested` and waits for rollback; it also wakes `Run`, cancels the `RunTask` task, and waits for the task to return before teardown. The App releases the logger it creates from config as the last shutdown step; a `WithLogger` logger is borrowed and never closed. Pre-stable breaking change.
- **component**: `StartAll` and `StartAllConcurrent` return a `*StartError` that carries rollback stop failures instead of only logging them. `Register` rejects empty names. Pre-stable breaking change.
- **logging**: `Close` is idempotent and returns the first result. The new `Shutdown(ctx)` bounds the OTLP flush by the caller's deadline and still stops the exporter when that deadline has already passed.
- **sse**: a connection-limit rejection carries a one-second `Retry-After` hint, so clients back off briefly instead of guessing.
- **server**: `NewComponent` accepts `WithName`, so one registry can host several servers. The default name stays `http-server`.
- **toolchain / dependencies**: Go 1.27.1 across product modules, demos, workspaces, and CI; refreshed compatible dependencies and pinned validation tools. Retains patched gRPC 1.83.2 and its compatible dependencies because 1.84.0 is affected by GO-2026-6443. Kubernetes uses the OpenAPI revision required by its released libraries, not an incompatible development snapshot.
- **connect / auth / storage**: native HTTP/2 client transport, validated SEC 1 EC-key parsing, and service-account-specific GCS credential loading replace deprecated APIs. Connect enforces the configured TLS/h2c URL scheme on every request and redirect, preventing cleartext downgrades and HTTP/1 fallback. GCS explicit credentials must match the documented service-account type; default credential discovery remains available.
- **bootstrap**: `App.DisplaySummary` and `Summary.DisplaySummary` now take a context. Summary health probes preserve startup deadlines; rollback preserves context values while using its own bounded cancellation budget.
- **errors / validation / transports**: redesigned the failure contract around derived statuses, semantic field violations, and one shared standard RPC codec. `errors.New` takes code and message; status is read through `HTTPStatus()`. RPC encoders return encoding errors; `DecodeError` returns a separate remote failure rather than a trusted `AppError`. Unknown failures and validator evaluation errors are normalized consistently. Pre-stable breaking change; see `errors/README.md`.
- **resilience / grpc**: explicit retry verdicts and minimum server delays survive the wire. Generic external-service failures are non-retryable by default. gRPC retries require a configured policy and an idempotent call marker; per-call retry selection preserves shared breaker and limiter state.
- **sse**: redesigned the module into scoped, resumable live events. A single `Bus` owns `<epoch>:<sequence>` replay, route matching, bounded connection admission, and per-subscriber live queues, replacing the former monolithic `Hub`. Application messages are proto-JSON keyed by their full protobuf message name; `connected`/`reset`/`failure` controls use a separate slot and never carry an `id:`. Slow clients get a priority `overflow` reset and disconnect instead of silently losing events; replay readers overtaken by eviction get `replayExpired`. `Authorizer` returns a verified `Access{Principal, Route, Lifetime}` with no implicit public fallback. Failure frames serialize `errors.Failure` (not raw `AppError`), with minimum retry delays and preserved explicit-retry verdicts. Concerns are split into focused files (`bus`, `subscription`, `limits`, `metrics`), the worker→SSE bridge moves to `sse/worker.Forward` as an explicit typed proto mapper with no invented JSON envelope, and renewable write deadlines bound slow writes. Pre-stable breaking change; see `sse/README.md`.
- **database**: redesigned backend selection around a `Dialect` seam so core stays driver-agnostic. A `Dialect` (`Name`/`Open`) is registered in a `DialectRegistry` and selected via `Component.WithDialect`/`WithDialectFromRegistry`; the opt-in `StructuredDialect` adds `DSN(ConnParams)` so a dialect can build its own connection string. `Config` now carries an opaque `DSN` **or** structured, driver-agnostic `ConnParams` (`Host`/`Port`/`User`/`Password`/`Database` plus an `Options` map for backend-specific knobs like `sslmode`/`tls`); when `DSN` is empty the selected dialect builds it. Removes the postgres-coupled `Config` fields (`Host`/`Port`/`DBName`/`User`/`Password`/`SSLMode`), the unused `Resolve` and `Driver` fields, `BuildDSN()`, and the old `DriverFunc`/`DriverRegistry`/`WithDriver`/`WithDriverFromRegistry` names. `Validate` now requires a `DSN` or non-empty `Params` when enabled. Structured DSN construction lives entirely in each adapter (SQLite stays DSN-only; PostgreSQL implements `StructuredDialect`), so core never imports a driver SDK. Pre-stable breaking change.
- **auth/jwt / bootstrap**: standard JWT keys are structurally validated and privately snapshotted, including typed-nil inputs and nil ECDSA coordinates or scalars; constructor errors retain causes and issuance enforces the verifier's 8 KiB bound. RunPortTask rejects nil app/port/task before mutation.

### Removed
- **auth/apikey**: the standalone `Middleware`/`FromContext` path. API keys authenticate through `auth.NewChain` like every other credential. Pre-stable breaking change.
- **database**: `ScopeToTenant` and `SetSessionVariable`. Use `repository.NewScopedSpec` for partition scoping and `postgres.SetLocal` for transaction-local settings; session-level settings leaked across pooled connections. Pre-stable breaking change.
- **database**: the untimed `DB.Ping()` method. Callers must use the context-aware `DB.PingContext(ctx)` so every liveness check is cancellable — and timeout-bound when the caller passes a context carrying a deadline. Pre-stable breaking change.

### Fixed
- **connect/client**: failure mapping preserves validated peer retry verdicts and minimum delays, fails closed on malformed RPC details, and retains caller causes. Locally constructed validation and policy errors no longer change peer availability.
- **bootstrap**: readiness rechecks draining after component health collection, preventing a stale ready response during shutdown.
- **bootstrap/testutil**: the loopback listener supports h2c and follows the production root/fallback contract, verified by a shared conformance suite and real Connect unary/streaming clients.
- **server**: unencodable Gin error details produce a valid Internal problem before headers are committed instead of an empty response under the original status.
- **connect/client**: first-message watchdogs start before stream construction, and `CloseResponse` preserves an expired first-message error while still releasing the response. Clean `io.EOF` is normal completion in availability and `MapCallFailure` classification; marked transport failures wrapping EOF remain outages.
- **bootstrap**: admin tests probe wildcard listeners through loopback, so ambient HTTP proxies cannot intercept local health checks.
- **sse**: the handler clears its write deadline after each flushed frame. A deadline left armed between frames made Go's HTTP/2 server reset quiet streams before their next heartbeat.
- **database**: `gorm.Open` builds the `*sql.DB` pool lazily and succeeds even when the server is unreachable, so a failed attempt surfaced only at ping time left the pool open; each retry then abandoned another pool. GORM's automatic (context-free) ping is now disabled so `connectOnce` owns the sole cancellable `PingContext` and closes the pool on any post-open failure. `Component.Start` likewise closes the pool it opened when a follow-on step (auto-migration) fails, since the component registry only calls `Stop` for `Start` failures caused by a context error. Connection attempts now run through a `resilience.Policy` (canonical retry/backoff/timeout owner) instead of a bespoke loop, injectable via `WithConnectPolicy`, with a per-attempt `Config.ConnectTimeout` (default `30s`) bounding each attempt; `Component.Start` validates config before connecting.

## [0.3.0-alpha.1] - 2026-08-25

### Added — L5 transport rskit parity
- **discovery**: a canonical `Balancer` set (`RoundRobin`/`Random`/`Weighted`/`LeastConnections`) selected by `NewBalancer(strategy)`; the high-level `Client` now delegates every strategy to it (per-`service:protocol` round-robin state, shared random/weighted/least-connections) instead of carrying its own inline selection. `LeastConnectionsBalancer` tracks in-flight load via `Client.Acquire`/`Release`. Add a `DiscoveryServer` component wrapper that starts the inner component then registers, rolls back (stops inner) on registration failure, and deregisters before stop. `ServiceResolver` rejects `least_conn`, which a stateless resolve-to-URL cannot honor.
- **grpc**: the lossless `AppErrorToStatus`/`StatusToAppError` mapping pair — the RFC 9457 ProblemDetail is embedded in the status details so a round trip preserves code, message, and extension members — alongside the `ErrorCodeToGRPCCode`/`GRPCCodeToErrorCode` code maps.
- **grpc/testutil**: an in-process gRPC `Server` test harness backed by an in-memory bufconn listener (implements `component.Component`/`testutil.TestComponent`); a configurable unary handler drives success, any gRPC status, deadlines, and cancellation so the client stack — dialing, interceptors, mapping — can be proven end to end without a real socket.
- **sse**: a bounded, typed `Bus[T]` with monotonic event IDs and `Last-Event-ID` replay; live fan-out and the replay buffer are both capacity-bounded and a slow subscriber drops newer events rather than blocking the bus.

### Added — core patterns/crosscutting rskit parity
- **component**: `RegistryConfig` with `NewRegistryWithConfig` — bounded-concurrency `StartAllConcurrent` plus per-component start/stop timeouts applied when the caller's context has no deadline. Add the `LazyComponent` factory wrapper that defers construction to first `Start` (rejecting a nil factory or nil-producing factory with a typed error), and `Healthy`/`Degraded`/`Unhealthy` `Health` constructors.
- **resilience**: `RetryPreset` presets (`RetryFast`/`RetryStandard`/`RetryExternalService`), an elapsed-time retry budget (`RetryConfig.MaxElapsedTime`) enforced at each attempt boundary, and `Validate()` guards (rejecting non-finite floats) for the retry, circuit-breaker, bulkhead, and rate-limiter configs.
- **observability**: OTLP wire-protocol selection (`OTLPProtocol`, `ParseOTLPProtocol`, `OTLPProtocol.Validate`) so tracer/meter exporters can target HTTP or gRPC collectors; unknown protocol strings and enum values are rejected instead of silently defaulting to HTTP.
- **security**/**util**: canonical `BasicAuthScheme`/`BearerAuthScheme` header-scheme names (referenced by server auth middleware) and `util.ConstantTimeEqual` for constant-time byte comparison.

### Changed
- **component**: `NewRegistry`'s default configuration now bounds each `StartAll`/`StartAllConcurrent` component `Start` with a 30s `StartTimeout` when the caller's context carries no deadline (previously startup was governed solely by the caller's context). A component that legitimately needs longer must pass a context with an explicit deadline or raise `RegistryConfig.StartTimeout`. Pre-stable behavioral change.

### Security — Go 1.26.6 toolchain, CC0-1.0 allow-list
- Bump the pinned `toolchain` to `go1.26.6` across every module and `go.work`, clearing the govulncheck stdlib findings (GO-2026-5942, 5972, 6088, 6089, 6090, 6091, 6218 and the idna advisory), all fixed in 1.26.6.
- Allow the `CC0-1.0` public-domain dedication in the license gate so `github.com/zeebo/blake3` (the core BLAKE3 content hasher) passes; the sign-off is recorded in `docs/dependencies.md`.

### Fixed
- **workload/docker**: `Manager.Wait` now returns `context.Canceled`/`DeadlineExceeded` deterministically when the context is already done, instead of racing the container-wait result; removes a flaky test under the Go 1.26.6 toolchain.
- **release**: the `Release (publish)` workflow no longer reports success when a release actually fails — both `make … | tee` steps now run under `set -euo pipefail` (`shell: bash`), so a failed `toven release publish` fails the job instead of being masked by `tee`'s zero exit code. Pin the release toolchain to the repo's declared Go via `actions/setup-go` (`go-version-file: go.mod`) on the preview and publish jobs so publishing no longer inherits the runner's incidental Go and cannot dirty the tree before Toven's clean-tree guard, add a pre-publish clean-tree assertion that prints `git status`/`diff --stat` when the tree is not pristine, and bump the pinned Toven to `v0.1.0-alpha.10` (whose clean-tree guard now names the offending paths) across `release.yml`, `ci.yml`, and `toven-canary.yml`.

### Changed — MCP go-sdk 1.7.0 / SEP-2322 multi round-trip
- **mcp**: bump `modelcontextprotocol/go-sdk` to 1.7.0. Its default
  2026-07-28 protocol forbids standalone server-initiated sampling,
  elicitation, and roots requests (SEP-2322). Add first-class hardened
  interactive (multi round-trip) tools via `Server.AddInteractiveTool`,
  which return these as `InputRequests` from a `tools/call` handler and
  read the responses back through the size-limited, audited
  `SampleResponse`/`ElicitResponse`/`RootsResponse` helpers. The standalone
  `Sample`/`Elicit`/`ListRoots` helpers are reframed as pre-2026-07-28-only
  and fail closed against modern clients.

## [0.2.0-alpha.1] - 2026-07-19

### Added — Generic dataset collection kit
- **dataset** (NEW module, light mirror of rskit-dataset): a streaming,
  generics-first dataset-collection toolkit built on one item-generic `collect.Collector[T]` engine.
  - **collect**: the generic engine — a bounded worker pool with `StreamBuffer` backpressure,
    per-source timeout/cancellation,
    and a single-owner main loop that folds worker events into the manifest, result,
    and progress without shared mutexes.
    Fails closed (a source, validation, or target error aborts the run and publishes no failed source),
    records real/AI stats, and always saves the manifest for resume.
  - **stage**: the generic streaming stages (`Source`/`Transform`/`Target`) over `stream` pipelines,
    plus opt-in item capabilities (`Labeled`, `Offsetted`)
    and source capabilities (`Keyed`, `Bounded`, `Resumable`) and a pluggable `Validator[T]`.
  - **record**: the tabular `record.Record` family — CSV/JSON-array/JSON-lines readers and writers,
    stream filters, and a file source/target that accumulates records across publishes.
  - **sample**: the blob item family — a labeled,
    offset-carrying `sample.Item` over a bounded payload, with slice/directory sources
    and a real/AI-splitting local target confined through `fs` path safety.
  - **schema**: fail-closed JSON Schema validation adapted into a `stage.Validator[record.Record]`.
  - **manifest**: a bounded,
    atomically persisted cache with one canonical `CacheStatusFor` that lets a run skip
    or resume sources.
  - **payload**: bounded in-memory or file-backed byte payloads with resource `Limits`.

### Changed (Breaking API Changes) — Typed AI/LLM/tool APIs
- **ai / llm**: `ai.ToolUseBlock.Input`
  and `llm.CompletionRequest.Extra` no longer expose `map[string]any`;
  they now carry raw JSON (`llm.RawJSON`) as an opaque,
  untrusted-by-default trust boundary that is validated at the edge instead of eagerly decoded.
- **tool**: tool-input schema is the documented opaque `schema.JSON` exception
  and per-tool resilience policy is a typed `*resilience.Policy` (via `Registry.WithToolPolicy` / `Registry.PolicyFor`);
  `Registry.Call` fails closed —
  raw input is JSON-Schema validated (`ErrInvalidToolInput`) before authorization
  or any side effect, and destructive tools (`SafetyDestructive`) are always human-gated
  and default to deny until an approver is wired.
- **llm**: `CompleteStructured[T]` is now generic, decoding model output into a concrete `T`
  and returning the zero value (never a partial one) on decode failure.

### Changed (Breaking API Changes) — Hardened MCP protocol server
- **mcp**: reshaped from a flat tool-bridge into a protocol-shaped,
  hardened server split by concern (`security`, `convert`, `handlers`, thin composition root).
  `NewServer` now returns a typed `*mcp.Server` (was the raw SDK server);
  transports are exposed as `Server.ServeStdio` and `Server.StreamableHTTPHandler`,
  and only `stdio` + Streamable HTTP are supported (the obsolete standalone SSE transport is dropped).
- **mcp**: every `tools/call` runs a fail-closed hardening chain —
  capability allow-list → input-size limit → JSON-Schema validation → authorization (`authz`) → registry human-in-the- loop destructive gate → result-size limit → output-schema validation → audit (`observability`).
  Untrusted client/model payloads stay typed as `json.RawMessage`;
  documented JSON-Schema is `schema.JSON`.

### Added — MCP protocol surface & hardening
- **mcp**: protocol coverage for prompts, resources + templates (with subscribe), roots, sampling,
  elicitation, progress, and logging;
  server→client sampling/elicitation size-limit untrusted model/elicited content and fail closed.
- **mcp**: Streamable HTTP hardening —
  Origin validation/normalization preloaded into `http.CrossOriginProtection` (rejects paths/queries/fragments/credentials/opaque/non-http),
  localhost protection on by default, and optional constant-time bearer-token auth (header only).

### Added — Typed AI/LLM/tool APIs & Inference Streaming
- **ai**:
  `NormalizeToolInput` normalizes absent/empty tool arguments to `{}` without lossy coercion.
- **llm**: `RawJSON` request-extension carrier round-trips through both JSON and YAML
  and merges fail closed (a non-object extension is rejected rather than silently corrupting the request).
- **llm**:
  streamed tool-call arguments (untrusted) are bounded at `streamwire.MaxToolArgsBytes` (1 MiB)
  so a server cannot exhaust memory with unbounded deltas.
- **inference**: TGI
  and vLLM adapters implement `PredictStream` over a shared OpenAI-compatible `/v1/completions` SSE helper (`OAICompatPredictStream`) with proper context cancellation
  and terminal error events.

### Added — Terminal-UX cli kit
- **cli** (NEW package, light mirror of rskit-cli):
  a parser-agnostic terminal-UX toolkit that writes to injected `io.Writer`s
  and confines `fmt.Print*`/stdout to this package.
  - **theme**: semantic `Palette` colors and `Glyphs`, resolving `NO_COLOR`, TTY,
    and UTF-8 locale capability with byte-clean ASCII fallbacks.
  - **render**: `OutputTable`, `OutputKV`, `StatusReporter`, `OutputFormat`,
    and an `ErrorRenderer`/`ExitCode` mapping RFC 9457 `AppError`s onto a CLI exit-code convention.
  - **progress**: determinate `Bar` and indeterminate `Spinner`, caller-driven (no background timer)
    so they render deterministically without a clock.
  - **prompt**: a `Prompter` over a `Terminal` seam with a cooked-stdio `LineTerminal`,
    a deterministic `ScriptedTerminal` test double, validators, and a non-interactive fallback.
  - **signal**:
    graceful-shutdown helper mapping SIGINT/SIGTERM onto `context.Context` cancellation via `signal.NotifyContext`.
  - **live**: a bounded multi-region console for concurrent streaming output.
- Raw-mode rich TUI widgets are intentionally rskit-only.

### Added — Foundational Parity (codec, fs)
- **codec** (NEW module):
  generics-first `Codec` with `Encode[T]`/`Decode[T]` over a documented opaque `Value` tree;
  `JSONCodec` (pretty/compact), `TOMLCodec`, extension-based `CodecForName`/`CodecForPath`,
  `value` deep-merge with per-key array strategies,
  and bounded length-delimited `framing` (`WriteFrame`/`ReadFrame`, generic `WriteValue`/`ReadValue`).
  Promotes `pelletier/go-toml/v2` to a direct dependency.
- **fs** (NEW module, light mirror of rskit-fs):
  safe path helpers (`ValidateRelativePath`, `SafeJoin`, `NormalizeRelativePath`, `Canonicalize`, `ConfinePath`/`ConfineExistingPath` with symlink-escape rejection),
  temp files/dirs, atomic writes (`WriteAtomic`/`WriteAtomicReplace`), permissions, and metadata.
  `watch` is intentionally rskit-only.
- Fuzz tests for the codecs, frame reader, and path-safety validation.

### Changed (Breaking API Changes) — Foundational Parity
- Renamed package `github.com/kbukum/gokit/logger` → `.../logging`
  and `github.com/kbukum/gokit/pipeline` → `.../stream` (canonical cross-kit names); all imports,
  `doc.go`, `domains.toml`, `MODULE-INDEX.md`, and `parity-matrix.md` updated.
- **logging**: dropped the mutable package-level registry, reassignable global singleton,
  and `init()` side effects in favor of an injected `Registry`
  and an install-once `Default()` backed by `sync.OnceValue`.
- **version**: immutable build-info via `sync.OnceValue`/`compute(source)`;
  no mutable exported vars.
- **errors**: `FormatResourceError[T]` is now generic;
  `Details map[string]any` is kept as a documented RFC 9457 extension-member opaque exception.
- **schema**: added `limits.go` (`ValidationLimits`/`DefaultLimits`/`LimitError`)
  and `validate.go` (`CompiledSchema`/`Compile`/`CompileWithLimits`).

### Added — Documentation & Project Hygiene
- README: sibling-projects callout and `Project Documentation` index linking every governance doc.

### Added
- **bench**:
  per-package `Benchmark*` coverage for the hot paths flagged by the OSS-review perf gap (#50, F-020).
  Package count grew from 5 → 15; benchmark count from 5 → 42. New benchmarks live in:
  - `registry` — Register/Get/Lookup/Names/Each
  - `di` — Container Register, Resolve (interface + generic + Must variants), Provide, ResolveKey
  - `validation` — fluent validator chains, struct validator, UUID, pattern
  - `chain` — Executor.Execute (1/4/16/64 ops), Builder.Build
  - `dag` — BuildLevels, ExecuteBatch (chain + fan, 4/16/64 nodes)
  - `tool` — Registry Register/Get/Call
  - `workload` / `storage` / `discovery` / `llm` — factory/dialect Register & Get
  - `auth/oidc` — JWKS getKey hit/miss, RSA publicKey decode, RS256 verifyRSA
- **ci**:
  `.github/workflows/bench.yml` extended to iterate over every gokit module that has benchmarks (was root-only);
  benchstat still runs head-vs-base and remains advisory until the baseline stabilises.

### Added
- **registry** (NEW package):
  generic `Registry[T any]` consolidating the previously ad-hoc registries in `auth`, `discovery`,
  `storage`, `tool`, `workload`, and `llm`. `Register` returns an error on empty name, nil value,
  or duplicate name; `Names()` returns sorted; `Each` iterates deterministically. (#45)
- **di**: typed-key DI surface layered on top of `UnifiedContainer`:
  - `Key[T any]`, `NameKey[T](name)` — opaque, type-parameterised keys.
    The full key embeds `reflect.Type` of `T`,
    so two `Key[T]` of different concrete types with the same `name` cannot collide.
  - `Provide[T](c, key, ctor)` / `ProvideSingleton[T](c, key, value)` — generic registration;
    constructor signature is validated up front (must return `T` or `(T, error)`).
  - `ResolveKey[T](c, key)` / `MustResolveKey[T](c, key)` — generic resolution,
    no type assertions in caller code. (#43)

### Changed (Breaking API Changes)
- **auth**:
  `Registry.Register` now returns `error` on duplicate registration instead of silently overwriting.
  `Registry.MustGet` removed — use `Get`.
- **tool**: `Registry.MustRegister` removed — use `Register` (which returns `error`). (#46)
- **workload**: `FactoryRegistry.MustRegister` removed — use `Register`. (#46)
- **llm**: `DialectRegistry.MustRegister` removed — use `Register`. (#46)
- **storage**: `FactoryRegistry.Register` now returns `error` (was panic on duplicate).
  Provider `Register` functions (`local.Register`, `s3.Register`, `supabase.Register`) likewise return `error`.
- **discovery**: `ProviderRegistry.Register` now returns `error` (was panic).
  `NewComponent(registry, cfg, log, opts...)` now returns `(*Component, error)` —
  previously panicked.
  Provider `Register` functions (`static.Register`, `consul.Register`) return `error`.
- **di**: `UnifiedContainer.MustResolve` method removed.
  The free function `di.MustResolve[T](container, key)` is **kept** (issue #46 explicitly allows `Must*` for `init()` / test / CLI scope, where this helper is idiomatic).
- **auth/authctx**: `MustGet[T]` removed — use `Get[T]`. (#46)
- **server/middleware**: `MustTenantFromContext` removed — use `TenantFromContext`. (#46)
- **agent**: `MustPromptTemplate` and `PromptBuilder.MustBuild` removed —
  use `NewPromptTemplate` / `Build`. (#46)

### Internal
- All 6 first-party registries are now thin wrappers around `provider/namedregistry.Registry[T]`.
  Subsequent explicit adapter/provider registries should use the lightweight named registry package directly when the registered values are not provider implementations.
- **security**: documented,
  time-boxed govulncheck suppression for `GO-2026-5932` (deprecated, unfixable `golang.org/x/crypto/openpgp`; not imported or reachable in any module).
  Removed the two stale `moby/moby` suppressions now that `workload` links `moby/moby/client`.

### Release engineering & supply chain
- **Coverage gates** (`codecov.yml`):
  security-load-bearing modules (`errors`, `auth`, `authz`, `security`, `resilience`, `encryption`) enforced at ≥85%;
  a project no-drop gate and an 85% patch gate fail CI on any regression;
  an advisory ≥80%-per-package floor tracks the remaining backfill.
- **Fuzzing**: CI discovers
  and smoke-runs every `Fuzz*` target (`codec`, `schema`, `validation`, `auth/jwt`, …);
  each seeds valid + malformed input and fails closed.
- **CI hardening**: dual Go matrix (`1.26.0` floor + `stable`); Conventional-Commits PR-title check;
  a dependency **license allow-list** gate (`scripts/check-licenses.sh`) across all modules;
  all actions SHA-pinned with per-job least-privilege permissions.
- **Release pipeline**: GoReleaser produces a source archive, `checksums.txt`,
  and a CycloneDX SBOM (fixed the SBOM working-directory bug); cosign keyless **sign + verify**;
  SLSA build provenance via `actions/attest-build-provenance`. Added `make release-dry`.
  New third-party deps are justified in [`docs/dependencies.md`](docs/dependencies.md).

## [0.2.0] - 2026-04-25

> Historical development baseline retained for changelog context. The old `v0.2.0`
> tags were removed before the prerelease line was started.

### Changed (Breaking API Changes)
- **workload**: `RegisterFactory()` global and `New(cfg, providerCfg, log)` removed.
  `New` now requires an explicit `*FactoryRegistry` as its first argument:
  `New(registry, cfg, providerCfg, log)`.
  Provider packages (`docker`, `kubernetes`) no longer register themselves via `init()`;
  call their `Register(registry)` function from your composition root.
  `NewComponent` likewise now takes the registry as its first argument.
- **llm**: `RegisterDialect()`, `GetDialect()`, and `Dialects()` package-level functions removed.
  `New(cfg)` is replaced by `New(registry, cfg)` taking an explicit `*DialectRegistry`.
  Provider packages (`anthropic`, `gemini`, `openai`) no longer register via `init()`;
  call their `Register(registry)` function instead.
- **di**: `MustResolve(name string) interface{}` removed from the `Container` interface.
  Use the generic free function `di.MustResolve[T](container, key)` instead —
  it provides type safety and works with any `Container` implementation.
- **config**:
  `WarningFunc` signature changed from `func(msg string, args ...any)` (printf-style) to `func(msg string, attrs ...slog.Attr)` (structured).
  Update custom warning loggers to emit structured attributes instead of formatted strings;
  this aligns config warnings with the rest of gokit's structured logging.
- **bootstrap**: `Summary.DisplaySummary` no longer writes directly to `os.Stdout`.
  Output now goes to the writer configured via `bootstrap.WithWriter(io.Writer)` (default still `os.Stdout`).
  `NewSummaryWithOptions` and `(*Summary).SetWriter` allow injecting a custom writer for testing
  or redirection.
- **storage**: `DefaultFactoryRegistry` global
  and `RegisterFactory()` / `New(cfg, providerCfg, log)` shims removed.
  `New` now requires an explicit `*FactoryRegistry` as its first argument:
  `New(registry, cfg, providerCfg, log)`.
  Provider packages (`local`, `s3`, `supabase`) no longer register themselves via `init()`;
  call their `Register(registry)` function from your composition root.
- **discovery**: `DefaultProviderRegistry` global, `RegisterProviderFactory()`,
  and `GetProviderFactory()` shims removed.
  `NewComponent` now requires an explicit `*ProviderRegistry` as its first argument:
  `NewComponent(registry, cfg, log, opts...)`.
  Provider packages (`static`, `consul`) no longer register via `init()`;
  call their `Register(registry)` function instead. `WithProviderRegistry` option removed.
- **server/middleware**: `Auth()`
  and `OptionalAuth()` now return `(gin.HandlerFunc, error)` instead of panicking on misconfiguration.
  All call sites must handle the returned error.
- **server/middleware**: `OptionalAuth` rejects invalid tokens by default (secure-by-default).
  Use `WithAllowInvalidTokens(true)` to opt in to the previous lax behavior.
  `WithRejectInvalidTokens` option removed.
- **di**: `ResolveOrError` removed (was an alias of `Resolve`). Use `Resolve` directly.
- **server/middleware**: `TenantFromContextOrError` and `ErrNoTenantID` removed.
  Use `TenantFromContext` (returns `(string, bool)`) or `MustTenantFromContext`.
- **config**: `Warning` struct and `[]Warning` return value removed from `loadFromResolvedFiles`.
  Non-fatal warnings are surfaced exclusively through the `WarningFunc` callback.

### Added
- **bootstrap**: `WithWriter(io.Writer)` option
  and `(*Summary).SetWriter` method for redirecting summary output (testing, in-memory capture, file logging).
- **workload**: `FactoryRegistry` type with `Register`, `MustRegister`, `Get`, and `Names`.
  Mirrors the `storage` package's explicit-registry pattern.
- **llm**: `DialectRegistry` type with `Register`, `MustRegister`, `Get`, and `Names`.
- **CI/governance**: `.editorconfig`, `.gitattributes`, `.github/dependabot.yml`,
  committed `go.work`, `GOVERNANCE.md`, `MAINTAINERS.md`.
  Expanded `SECURITY.md` with a private vulnerability reporting flow and a supply-chain section.
- **CI**: pinned `golangci-lint` to a specific tag, added `govulncheck` per module,
  multi-OS test matrix on representative modules, `-shuffle=on` and race detection by default,
  fuzz smoke job, and Go-version-consistency check across all `go.mod` files.
- **lint**: `errorlint`, `nilerr`, `copyloopvar`, `wastedassign`, `sqlclosecheck`, `rowserrcheck`,
  and govet `shadow` are now enforced.
- **examples**: `Example*` tests added for `config`, `errors`, `logger`, `pipeline`, `provider`,
  and `di` for godoc discoverability.
- **docs**:
  Added `doc.go` to packages that previously lacked package-level documentation (`database/repository`, `discovery/{consul,static}`, `grpc/{client,interceptor}`, `messaging/kafka/{consumer,producer}`, `server/{endpoint,middleware}`, `storage/{local,s3,supabase}`, `workload/{docker,kubernetes}`).
- **benchmarks**: All benchmarks now call `b.ReportAllocs()` for allocation visibility.

### Security
- **gosec**: Removed the global `G402` exclude.
  TLS configuration sites that intentionally allow `InsecureSkipVerify` now carry a per-site `//nolint:gosec` directive with a justifying comment.

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

- **messaging**: `ManagedConsumer` — wraps any `Consumer` with lifecycle (Start/Stop/IsRunning)
  and handler dispatch
- **messaging**: `ConsumerRunner` interface and `AsRunner()` adapter for managed consumption loops
- **messaging**:
  `MetricsCollector` interface with `RecordPublish()`/`RecordConsume()` for broker-agnostic metrics
- **messaging**: `ErrorTranslator` interface for converting raw errors to `*AppError`
- **messaging**: `ErrorClassifier` interface with `IsConnectionError()`/`IsRetryableError()` helpers
- **messaging**:
  `BrokerComponent` interface extending `component.Component` with `Producer()`/`Consumer()` accessors
- **messaging**:
  `MessageHandler` type + `HandlerMiddleware` + `ChainHandlers()` for composable handler pipelines
- **messaging**: `MessageRouter` — topic-based message routing with exact match
  and wildcard (`*`) pattern support
- **messaging**: `BatchProducer` — buffered producer with size, time (MaxWait),
  and byte (MaxBytes) flush triggers
- **messaging/bridge**: `ProducerAsSink()` — adapts `Producer` to `provider.Sink[Message]`
- **messaging/bridge**: `EventProducerAsSink()` — adapts `EventProducer` to `provider.Sink[Event]`
- **messaging/bridge**: `ConsumerAsStream()` —
  adapts `Consumer` to `provider.Stream[struct{}, Message]`
- **messaging/middleware**: `DedupHandler` — deduplication middleware with LRU cache, TTL,
  and bounded window
- **messaging/middleware**: `CircuitBreakerHandler` —
  fail-fast middleware wrapping `resilience.CircuitBreaker`
- **messaging/memory**: Enhanced `InMemoryBroker` with message history, topic management,
  and reset capability
- **messaging/memory**: Test assertions — `AssertPublished()`, `AssertPublishedN()`,
  `WaitForMessage()`, `AssertNoMessages()`

### Added
- **bench**: New sub-module —
  pluggable evaluation framework for benchmarking providers against labeled datasets
  - Core types: `Sample[L]`, `Prediction[L]`, `ScoredSample[L]`, `LabelMapper[L]`
  - `DatasetLoader[L]`: manifest-based dataset loading with filtering and pipeline integration
  - `Evaluator[L]`: provider adapter interface with `EvaluatorFunc` and `FromProvider` helpers
  - `BenchRunner[L]`: orchestrates evaluation runs with multi-branch support and concurrency
  - `FileStorage`: JSON file-based run result persistence with listing, filtering, and Latest()
  - `RunComparator`: compares two runs with metric diffs, regression detection,
    and sample-level tracking
  - Result types: `RunResult`, `MetricResult`, `BranchResult`, `SampleResult`, `RunSummary`
- **bench/metric**: Pluggable metric implementations for evaluation scoring
  - Classification: `BinaryClassification`, `MultiClassClassification`, `ConfusionMatrix`,
    `ThresholdSweep`
  - Probability: `AUCROC`, `BrierScore`, `LogLoss`, `Calibration`
  - Ranking: `NDCG`, `MAP`, `PrecisionAtK`, `RecallAtK`
  - Regression: `MAE`, `MSE`, `RMSE`, `RSquared`
  - Matching: `ExactMatch`, `FuzzyMatch` (Levenshtein-based)
  - Composite: `Weighted` for combining metrics with weights
  - `Suite[L]` for batch metric evaluation; `AsRunMetric`/`AsRunMetrics` adapters
- **bench/report**: Formatted output generation from benchmark results
  - `Reporter` interface with `JSON`, `Markdown`, `Table`, `CSV`, `JUnit`, `VegaLite`,
    `HTML` implementations
- **bench/viz**: SVG visualization generation from run results
  - `RenderAll` generates applicable charts; individual renderers for ROC, calibration,
    confusion matrix, distribution, branch comparison
- **bench/storage**:
  `ProviderStorage` adapter bridging `bench.RunStorage` with `gokit/storage.Storage` backends
- **tests**: Comprehensive test suite for bench module — types, dataset loading, evaluator adapters,
  runner, file storage, comparator, classification metrics, probability metrics, regression metrics,
  matching metrics, JSON reporter
- **docs**: Package-level documentation for bench/metric, bench/report sub-packages

- **provider**: Sink combinator primitives for composable push-based data flow
  - `NewSinkFunc[I]`: wraps a plain `func(ctx, I) error` as a `Sink[I]` (like `http.HandlerFunc`)
  - `FanOutSink[I]`: dispatches input to multiple sinks in parallel, joins errors
  - `AdaptSink[I, BI]`:
    transforms input types before sending (mirrors `Adapt` for `RequestResponse`)
  - `TapSink[I]`: adds a side-effect observer before forwarding to the inner sink
  - `SinkMiddleware[I]` + `ChainSink[I]`:
    composable wrapping for sinks (mirrors `Middleware` + `Chain`)
- **tests**: 11 sink combinator tests — SinkFunc,
  FanOutSink (parallel, errors, passthrough, availability), AdaptSink (mapping, errors), TapSink,
  ChainSink (ordering)
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
- **provider**: `Streamable[I, O, C]` interface for providers supporting both request-response
  and streaming modes
- **httpclient**: `MultipartBody` and `FileField` types for multipart/form-data requests
  - `encodeBody()` auto-handles `*MultipartBody` — no more manual `mime/multipart` construction
  - Supports custom content-type per file, streaming upload via `io.Reader`
- **httpclient/rest**: `Client` now implements `provider.Provider` (Name, IsAvailable, Close)
- **httpclient/rest**:
  Error helper re-exports (`IsNotFound`, `IsAuth`, `IsRateLimit`, `IsServerError`, `IsRetryable`, `IsTimeout`)
- **tests**: 27 LLM adapter tests (81.7% coverage) — adapter, dialect registry, streaming, helpers,
  types
- **tests**: 5 multipart encoding tests — fields, files, custom content-type, reader,
  full adapter integration
- **tests**: 3 REST provider interface tests — Name/IsAvailable/Close delegation,
  error classification
- **docs**: layered adapter composition guide

## [0.1.4] - 2026-03-01

### Added
- **github**: CODEOWNERS file for automatic code review assignment
- **github**: Issue templates for bug reports and feature requests (YAML forms)
- **github**: Pull request template with comprehensive checklist
- **docs**: CODE_OF_CONDUCT.md based on Contributor Covenant 2.1
- **docs**: SECURITY.md with responsible disclosure policy
- **docs**: adapter-guide.md documenting adapter pattern across all modules
- **docs**: adapter framework guide
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
- **tests**:
  Comprehensive test suite for encryption (ChaCha20 encryption/decryption, error handling)
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
- **redis/testutil**: Removed — exposed raw `*goredis.Client` instead of gokit's `*redis.Client`,
  making it unusable for testing gokit redis operations.

### Changed
- **ci**: Rewritten CI pipeline with dynamic module discovery — no hardcoded module list,
  per-module parallel jobs, tidy verification gate.

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
- **various**: Updated multiple functions to accept configuration by pointer to improve performance
  and satisfy linters.
