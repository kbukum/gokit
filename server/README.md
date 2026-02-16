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
| `Config` | Host, Port, timeouts, max body size, CORS |
| `ServerComponent` | Managed lifecycle — `Start`, `Stop`, `Health` |
| `New(cfg, log)` | Create a server instance |
| `(*Server) GinEngine()` | Access the underlying `*gin.Engine` |
| `RespondOK(c, data)` | JSON 200 response |

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
