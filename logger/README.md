# logger

Structured logging built on zerolog with context propagation, component tagging, and field helpers.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start

```go
package main

import "github.com/kbukum/gokit/logger"

func main() {
    log := logger.NewDefault("my-service")
    log.Info("server started", logger.Fields("port", 8080))

    // Component-scoped logger
    dbLog := log.WithComponent("database")
    dbLog.Error("connection failed", logger.ErrorFields("connect", err))

    // Global logger
    logger.SetGlobalLogger(log)
    logger.Info("using global logger")
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `Logger` | Structured logger wrapping zerolog |
| `Config` | Logger configuration (level, format, output, rotation) |
| `New()` / `NewDefault()` / `NewFromEnv()` | Logger constructors |
| `WithContext()` / `WithComponent()` / `WithFields()` | Scoped logger builders |
| `Debug()` / `Info()` / `Warn()` / `Error()` / `Fatal()` | Log methods (instance + global) |
| `Fields()` / `ErrorFields()` / `DurationFields()` | Structured field helpers |
| `SetGlobalLogger()` / `GetGlobalLogger()` | Global logger management |
| `ComponentRegistry` | Tracks registered infrastructure and service components |

---

[⬅ Back to main README](../README.md)
