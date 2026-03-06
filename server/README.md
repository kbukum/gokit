# server

Gin-based HTTP server with h2c support, built-in middleware, health/info endpoints, and component lifecycle.

## Install

```bash
go get github.com/kbukum/gokit/server@latest
```

## Quick Start

```go
import (
    "github.com/kbukum/gokit/server"
    "github.com/kbukum/gokit/logger"
)

log := logger.New()
srv := server.New(server.Config{Host: "0.0.0.0", Port: 8080}, log)
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
| `Config` | Host, Port, timeouts, max body size, CORS, Docs |
| `DocsConfig` | Controls API documentation serving (`docs.enabled`) |
| `ServerComponent` | Managed lifecycle — `Start`, `Stop`, `Health` |
| `New(cfg, log)` | Create a server instance |
| `(*Server) GinEngine()` | Access the underlying `*gin.Engine` |
| `RespondOK(c, data)` | JSON 200 response |
| `MountDocs(engine, ...APIDoc)` | Mount interactive API docs (powered by [Scalar](https://github.com/scalar/scalar)) |
| `APIDoc` | Spec definition: Title, Spec, UIPath, Host, BasePath, HideAI, Theme |

### API Documentation

`MountDocs` serves interactive API reference pages powered by Scalar. Each `APIDoc` registers two routes: a raw spec endpoint and a rendered docs page.

```go
//go:embed swagger.json
var specJSON []byte

if srv.Config().Docs.Enabled {
    server.MountDocs(srv.GinEngine(), server.APIDoc{
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
| `DarkMode` | Dark theme | `true` |
| `HideAI` | Hide Scalar's AI assistant | `false` |
| `Theme` | Scalar theme (`"moon"`, `"purple"`, `"deepSpace"`, etc.) | *(default)* |
| `CustomCSS` | Additional CSS | *(none)* |

Multiple specs can be mounted for services with multiple APIs:

```go
server.MountDocs(engine,
    server.APIDoc{Title: "Users API", SpecPath: "/api-specs/users.yaml", Spec: usersSpec, UIPath: "/docs", ContentType: "application/yaml"},
    server.APIDoc{Title: "Admin API", SpecPath: "/api-specs/admin.yaml", Spec: adminSpec, UIPath: "/docs/admin", ContentType: "application/yaml"},
)
```

### `server/middleware`

| Function | Description |
|---|---|
| `Auth(AuthConfig)` | Token-based authentication |
| `CORS(CORSConfig)` | Cross-origin resource sharing |
| `Recovery()` | Panic recovery |

### `server/endpoint`

| Function | Description |
|---|---|
| `Health(name, checker)` | `/healthz` endpoint |
| `Info(name)` | `/info` build version endpoint |

---

[← Back to main gokit README](../README.md)
