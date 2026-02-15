# observability

OpenTelemetry-based tracing, metrics, and health checks for service observability.

## Install

```bash
go get github.com/skillsenselab/gokit
```

## Quick Start

```go
package main

import (
    "context"
    "github.com/skillsenselab/gokit/observability"
)

func main() {
    ctx := context.Background()

    // Initialize tracer
    tp, _ := observability.InitTracer(ctx, observability.DefaultTracerConfig("my-service"))
    defer tp.Shutdown(ctx)

    // Initialize metrics
    mp, _ := observability.InitMeter(ctx, observability.DefaultMeterConfig("my-service"))
    defer mp.Shutdown(ctx)

    // Start a span
    ctx, span := observability.StartSpan(ctx, "process-request")
    observability.SetSpanAttribute(ctx, "user.id", "abc-123")
    defer span.End()

    // Health checks
    health := observability.NewServiceHealth("my-service", "1.0.0")
    health.AddComponent(observability.ComponentHealth{
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
| `SetSpanAttribute()` / `SetSpanError()` | Span enrichment |
| `Metrics` | Pre-built metric instruments for requests, operations, errors |
| `OperationContext` | Combines tracing + metrics for tracked operations |
| `ServiceHealth` / `HealthChecker` | Service health aggregation |

---

[⬅ Back to main README](../README.md)
