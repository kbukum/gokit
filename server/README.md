# server

Gin-based HTTP server with h2c support, built-in middleware, health/info endpoints, and component lifecycle.

## SPA, diagnostics, and shutdown

`MountSPA` serves a trusted embedded or root-confined `fs.FS`; keep an `os.OpenRoot` handle open until shutdown. HTML navigation falls back to `index.html`, while API namespaces, asset namespaces, and missing files return 404. Use `ReservedPrefixes` for application exclusions.

`Fallback` installs an unmatched-route handler behind middleware and rejects nil handlers with `ErrInvalidRoute`. Module fallbacks also reserve the module's API prefixes.

List fingerprinted files from the build manifest in `ImmutableAssets` to cache them for one year. Other assets revalidate. The index is limited to 1 MiB and uses `no-cache, no-store`. Put `{nonce}` in script/style nonce attributes; each index response gets a fresh cryptographic nonce and a matching strict CSP. `spa.Config.CSP` can replace the policy template, but cannot enable unsafe scripts or weaken its base/object/frame restrictions.

Diagnostics are never registered by `RegisterDefaultEndpoints`. Metrics, pprof and the lifecycle-aware `/readyz` belong to the App-owned admin listener: see `bootstrap.WithAdmin`.

Call `Instrument(meter)` before `ApplyDefaults` to record HTTP metrics through an injected OpenTelemetry meter. Route labels use registered Gin/RPC patterns; unknown routes and methods use fixed labels. `http_requests_active` tracks in-flight requests. Register `sse/metrics` against the same application meter for active streams and queue gauges, then unregister it during resource cleanup before telemetry stops.

Call `Trace(middleware.TracingConfig)` before `ApplyDefaults` to trace every route with an injected tracer. The span wraps request logging, so request log lines and problem responses carry its trace ID, and `X-Request-Id` is replaced by that trace ID. Supply `Route` and, only for authenticated peers, `TrustParent` as described under [Middleware ordering](#middleware-ordering).

Register `NewComponent(srv)` for coordinated shutdown; give multiple servers distinct `WithName` values. Shutdown quiesces requests and SSE, drains listeners concurrently, then drains workers before closing dependencies and telemetry.

HTTP reserves one fifth of its drain time for forced close and handler cleanup. Idle pre-request connections close with the listeners. Forced close reports the deadline error; handlers and workers must honor cancellation.

REST request timeouts buffer at most 10 MiB of response body. Exceeding that limit returns a 503 and cancels the handler; writes after cancellation fail. Actual REST handler cleanup remains tracked after the timeout response, so dependency teardown waits for it. Mount streaming endpoints outside this buffering middleware.

## Install

```bash
go get github.com/kbukum/gokit/server@latest
```

## Quick Start

```go
import (
    "github.com/kbukum/gokit/server"
    "github.com/kbukum/gokit/logging"
)

log := logging.NewDefault("my-service")
srv := server.New(&server.Config{Host: "0.0.0.0", Port: 8080}, log)
srv.ApplyDefaults("my-service", healthChecker)

srv.GinEngine().GET("/hello", func(c *gin.Context) {
    server.RespondOK(c, map[string]string{"msg": "hello"})
})

comp := server.NewComponent(srv)
comp.Start(ctx)
defer comp.Stop(ctx)
```

## Key Types & Functions

| Symbol | Description |
|---|---|
| `Server` | Wraps Gin engine + `http.ServeMux` with h2c |
| `Config` | Host, Port, timeouts, max body bytes, h2c toggle, CORS, Docs |
| `DocsConfig` | Controls API documentation serving (`docs.enabled`) |
| `Component` | Managed lifecycle — `Start`, `Stop`, `Health` |
| `New(cfg, log)` | Create a server instance |
| `(*Server) GinEngine()` | Access the underlying `*gin.Engine` |
| `RespondOK(c, data)` | JSON 200 response |
| `MountDocs(engine, log, ...APIDoc)` | Mount interactive API docs (powered by [Scalar](https://github.com/scalar/scalar)) |
| `APIDoc` | Spec definition: Title, Spec, UIPath, Host, BasePath, HideAI, Theme |

### API Documentation

`MountDocs` serves interactive API reference pages powered by Scalar.
Each `APIDoc` registers two routes: a raw spec endpoint and a rendered docs page.

```go
//go:embed swagger.json
var specJSON []byte

if srv.Config().Docs.Enabled {
    server.MountDocs(srv.GinEngine(), srv.Logger(), server.APIDoc{
        Title:    "My Service API",
        SpecPath: "/api-spec.json",
        Spec:     specJSON,
        UIPath:   "/docs",
        Host:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
        BasePath: "/api/v1",
        HideAI:   true,
    })
}
```

**Config** (`config.yml`):
```yaml
server:   # or http:
  docs:
    enabled: true
```

**`APIDoc` options:**

| Field | Description | Default |
|---|---|---|
| `Title` | Browser tab title | `"API Reference"` |
| `SpecPath` | Route for raw spec | *(required)* |
| `Spec` | Embedded spec bytes (JSON or YAML) | *(required)* |
| `UIPath` | Route for docs page | *(required)* |
| `ContentType` | Spec MIME type | `"application/json"` |
| `Host` | Override spec `host` field | *(none)* |
| `BasePath` | Override spec `basePath` field | *(none)* |
| `DarkMode` | Dark theme (`*bool`; nil ⇒ enabled) | `true` |
| `HideModels` | Hide the "Models" sidebar section | `false` |
| `HideAI` | Hide Scalar's AI assistant | `false` |
| `Theme` | Scalar theme (`"moon"`, `"purple"`, `"deepSpace"`, etc.) | *(default)* |
| `CustomCSS` | Additional CSS | *(none)* |

Multiple specs can be mounted for services with multiple APIs:

```go
server.MountDocs(engine, log,
    server.APIDoc{Title: "Users API", SpecPath: "/api-specs/users.yaml", Spec: usersSpec, UIPath: "/docs", ContentType: "application/yaml"},
    server.APIDoc{Title: "Admin API", SpecPath: "/api-specs/admin.yaml", Spec: adminSpec, UIPath: "/docs/admin", ContentType: "application/yaml"},
)
```

### `server/middleware`

| Function | Description |
|---|---|
| `HTTPAuth[T](authenticator, setClaims, ...HTTPAuthOption)` | Authenticate the original HTTP request and propagate its typed identity before HTTP, Connect, or SSE dispatch |
| `WithAuthErrorWriter(writer)` | Inject protocol-aware rejection encoding; the default is safe RFC 9457 problem+json |
| `WriteProblemDetails(w, r, err)` | Write a normalized RFC 9457 problem+json response with `Cache-Control: no-store` and `Retry-After` for retryable failures that carry a positive retry delay; the default `HTTPAuth` writer |
| `WithMissingPolicy(policy)` | `RejectMissing` (default) or `AcceptMissing`, which dispatches credential-less requests without an identity |
| `Auth[T](validator, setClaims)` | Require a header-only bearer credential in Gin; validation receives the request context |
| `OptionalAuth[T](validator, setClaims)` | Accept a missing header, never malformed or invalid presented credentials |
| `CORS(CORSConfig)` | Cross-origin resource sharing |
| `Recovery()` | Panic recovery |
| `Tracing(TracingConfig)` | Inject a tracer and optional propagator, registered-route classifier and authenticated-parent policy; returns middleware and a configuration error |
| `RequestLogger(log)` | Log registered routes and context-derived identifiers without raw URLs or request-header IDs |

### Middleware ordering

When composing transport concerns, keep the shared order explicit:

1. tracing
2. recovery
3. logging
4. auth
5. validation
6. handler

Recovery inside tracing handles downstream panics with the active context before the span ends, writes a canonical safe 500 problem response and logs only safe failure metadata. An additional outer recovery boundary protects setup and middleware before tracing. `ApplyMiddleware` installs both boundaries when tracing is enabled. Never log raw panic values or stack dumps.

Public tracing starts a fresh root and ignores incoming sampling, baggage and forwarded scheme headers. For internal parent propagation, install tracing inside peer authentication and supply `TrustParent`; never decide trust from trace headers. An authentication rejection needs fresh local tracing at its rejection boundary, not trusted extraction. `Route` returns only a registered template/procedure; absent classification uses `http.request`. Place request logging inside tracing to share its context. Traces omit request bodies, URLs and raw errors; HTTP problem responses include the active trace ID.

Use `HTTPAuth` for cookie/API-key chains. The injected authenticator sees the original request and owns ambiguity, CSRF, expiry, and revocation checks. Pair its setter with the downstream typed getter and wrap protected handlers only.

Missing credentials fail unless `AcceptMissing` is explicit; that policy adds no identity, so required guards still reject the request. Invalid credentials and provider/store failures never become anonymous success.

`WithAuthErrorWriter` supplies protocol-specific rejection encoding. Normalize private diagnostics and preserve the protocol's status and envelope. Every rejection sets `Cache-Control: no-store`; the default uses safe RFC 9457 responses.

The Gin bearer middleware uses a context-aware `TokenValidator[T]` and accepts exactly one bounded `Authorization: Bearer …` credential. Empty, duplicate, oversized, or invalid headers are rejected even by `OptionalAuth`. URL query tokens and custom header/scheme fallbacks are not supported.

### TLS policy

`server` itself is transport-focused and commonly runs behind TLS termination or alongside a gRPC listener. For TLS settings shared across gokit transports, use `security.TLSConfig` with the locked policy:

- minimum supported floor: TLS 1.2
- default negotiation outcome: TLS 1.3 whenever peers support it
- explicit floors below TLS 1.2 are rejected
- any TLS setting on `server.Config.TLS` turns TLS on and requires both `cert_file` and `key_file`; `Config.Validate` rejects it otherwise

### `server/endpoint`

| Function | Description |
|---|---|
| `Health(name, checker)` | `/healthz` endpoint |
| `Info(name)` | `/info` build version endpoint |

---

[← Back to main gokit README](../README.md)
