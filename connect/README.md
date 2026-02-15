# connect

Connect-Go integration with gokit server — interceptors, error mapping, and service mounting.

## Install

```bash
go get github.com/skillsenselab/gokit/connect@latest
```

## Quick Start

```go
import (
    "github.com/skillsenselab/gokit/connect"
    "github.com/skillsenselab/gokit/server"
    "github.com/skillsenselab/gokit/logger"
    "connectrpc.com/connect"
)

log := logger.New()
srv := server.New(server.Config{Port: 8080}, log)

// Create Connect service handler
path, handler := userv1connect.NewUserServiceHandler(svc,
    connectrpc.WithInterceptors(goconnect.LoggingInterceptor(log), goconnect.ErrorInterceptor()),
)

// Mount on gokit server
connect.Mount(srv, path, handler)

// Or use the Service abstraction
services := []connect.Service{
    connect.NewService(path, handler),
}
connect.MountServices(srv, services...)
```

## Key Types & Functions

| Symbol | Description |
|---|---|
| `Config` | SendMaxBytes, ReadMaxBytes, Enabled |
| `Service` | Interface — `Path() string`, `Handler() http.Handler` |
| `NewService(path, handler)` | Create a Service from path and handler |
| `Mount(srv, path, handler)` | Mount a single Connect handler on gokit server |
| `MountServices(srv, ...Service)` | Mount multiple services at once |
| `LoggingInterceptor(log)` | Log RPC calls with duration and status |
| `ErrorInterceptor()` | Convert `*AppError` to Connect errors |
| `AuthInterceptor(validateToken)` | Bearer token validation interceptor |
| `ToConnectError(appErr)` | `*AppError` → `*connect.Error` |
| `FromConnectError(err)` | `*connect.Error` → `*AppError` |

---

[← Back to main gokit README](../README.md)
