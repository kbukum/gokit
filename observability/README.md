# observability

OpenTelemetry-based tracing, metrics, and health checks for service observability.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start

```go
package main

import (
    "context"
    "github.com/kbukum/gokit/observability"
)

func main() {
    ctx := context.Background()

    // Initialize tracer
    tracerCfg := observability.DefaultTracerConfig("my-service")
    tp, _ := observability.InitTracer(ctx, &tracerCfg)
    defer tp.Shutdown(ctx)

    // Initialize metrics
    meterCfg := observability.DefaultMeterConfig("my-service")
    mp, _ := observability.InitMeter(ctx, &meterCfg)
    defer mp.Shutdown(ctx)

    // Start a span
    ctx, span := observability.StartSpan(ctx, "process-request")
    observability.SetSpanAttributes(ctx, observability.StringAttribute("user.id", "abc-123"))
    defer span.End()

    // Health checks
    health := observability.NewServiceHealth("my-service", "1.0.0")
    health.AddComponent(observability.Health{
        Name:   "database",
        Status: "healthy",
    })
}
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `InitTracer()` / `TracerConfig` | OpenTelemetry tracer setup |
| `InitMeter()` / `MeterConfig` | OpenTelemetry metrics setup |
| `StartSpan()` / `SpanFromContext()` | Distributed tracing helpers |
| `SetSpanAttributes()` / `SetSpanError()` | Span enrichment |
| `Metrics` | Pre-built metric instruments for requests, operations, errors |
| `OperationContext` | Combines tracing + metrics for tracked operations |
| `ServiceHealth` / `HealthChecker` | Service health aggregation |

---

[⬅ Back to main README](../README.md)
