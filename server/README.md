# server

Gin-based HTTP server with h2c support, built-in middleware, health/info endpoints, and component lifecycle.

## SPA, diagnostics, and shutdown

`MountSPA(files, spa.Config{...})` serves a trusted `fs.FS` behind registered API routes. Use an embedded filesystem in an image or `os.OpenRoot(...).FS()` for confined on-disk assets; keep the root open until the server stops. HTML navigation falls back to `index.html`, but API namespaces, asset namespaces, and missing file paths return 404. Add application-specific API prefixes through `ReservedPrefixes`.

List fingerprinted files from the build manifest in `ImmutableAssets` to cache them for one year. Other assets revalidate. The index is limited to 1 MiB and uses `no-cache, no-store`. Put `{nonce}` in script/style nonce attributes; each index response gets a fresh cryptographic nonce and a matching strict CSP. `spa.Config.CSP` can replace the policy template, but cannot enable unsafe scripts or weaken its base/object/frame restrictions.

Diagnostics are never registered by `RegisterDefaultEndpoints`. Set `Config.Admin` with `Enabled: true` to bind a separate listener; its default host is loopback and port zero selects an ephemeral port. `/metrics` exposes instance-owned Go/process metrics. `AdminConfig.Metrics` accepts an application-owned exporter handler, and `Pprof: true` enables profiling on that listener only. Restrict the admin listener to a trusted network. `AdminAddr()` reports its actual address.

Call `Instrument(meter)` before `ApplyDefaults` to record HTTP metrics through an injected OpenTelemetry meter. Route labels use registered Gin/RPC patterns; unknown routes and methods use fixed labels. `http_requests_active` tracks in-flight requests. Register `sse/metrics` against the same application meter for active streams and queue gauges, then unregister it during resource cleanup before telemetry stops.

Register `NewComponent(srv)` in the component registry for coordinated shutdown. Its name is `http-server`; when one registry hosts several servers, give each a distinct name with `NewComponent(srv, server.WithName("internal"))`. The kit rejects new requests first, cancels SSE subscriptions, drains HTTP, then drains workers before closing clients and telemetry. The admin listener stops last. Each operation receives a share of the remaining total shutdown budget; HTTP reserves one fifth of its share for force-close and handler cleanup. A forced close returns the deadline error. Handlers and workers must honor cancellation: Go cannot terminate arbitrary application code.

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
| `WriteProblem(w, r, err)` | Write a normalized RFC 9457 problem+json response with `Cache-Control: no-store` and `Retry-After` for retryable failures that carry a positive retry delay; the default `HTTPAuth` writer |
| `WithMissingPolicy(policy)` | `RejectMissing` (default) or `AcceptMissing`, which dispatches credential-less requests without an identity |
| `Auth[T](validator, setClaims)` | Require a header-only bearer credential in Gin; validation receives the request context |
| `OptionalAuth[T](validator, setClaims)` | Accept a missing header, never malformed or invalid presented credentials |
| `CORS(CORSConfig)` | Cross-origin resource sharing |
| `Recovery()` | Panic recovery |

### Middleware ordering

When composing transport concerns, keep the shared order explicit:

1. tracing
2. logging
3. auth
4. validation
5. handler
6. metrics

Apply recovery outside that chain so panics from any layer are captured consistently.

Use `HTTPAuth` for a cookie/API-key authenticator chain: the injected `Authenticate(*http.Request) (T, bool, error)` implementation sees the original method, repeated headers, and cookies and owns credential ambiguity, CSRF, expiry, and revocation checks. Pair its setter with the same typed context getter in downstream handlers. Apply the middleware to protected handlers only. Authentication errors default to safe RFC 9457 responses; unknown provider/store failures never become anonymous success. The boolean reports whether any credential was presented: missing credentials are rejected unless `WithMissingPolicy(AcceptMissing)` is set, in which case the request continues with no identity in context, so required guards such as Connect `RequireAuth` still reject it. `WithAuthErrorWriter(func(http.ResponseWriter, *http.Request, error))` injects transport-specific encoding for a shared HTTP boundary. The writer receives the original request and error; it must normalize private diagnostics before serialization and preserve the target protocol's status and error envelope. Rejections set `Cache-Control: no-store` before invoking either writer.

The Gin bearer middleware uses a context-aware `TokenValidator[T]` and accepts exactly one bounded `Authorization: Bearer …` credential. Empty, duplicate, oversized, or invalid headers are rejected even by `OptionalAuth`. URL query tokens and custom header/scheme fallbacks are not supported.

### TLS policy

`server` itself is transport-focused and commonly runs behind TLS termination or alongside a gRPC listener. For TLS settings shared across gokit transports, use `security.TLSConfig` with the locked policy:

- minimum supported floor: TLS 1.2
- default negotiation outcome: TLS 1.3 whenever peers support it
- explicit floors below TLS 1.2 are rejected

### `server/endpoint`

| Function | Description |
|---|---|
| `Health(name, checker)` | `/healthz` endpoint |
| `Info(name)` | `/info` build version endpoint |

---

[← Back to main gokit README](../README.md)
